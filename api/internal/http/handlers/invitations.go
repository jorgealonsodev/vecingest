package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jorgealonsodev/vecingest/internal/authz"
	"github.com/jorgealonsodev/vecingest/internal/authz/scoped"
	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/audit"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/password"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
	"github.com/jorgealonsodev/vecingest/internal/domain/invitations"
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
	"github.com/jorgealonsodev/vecingest/internal/http/dto"
)

// invitationManageRoles is invitations' write role set (invitations
// spec: "Invitation Creation ... MUST require admin/admin_staff
// membership"). List/resend/revoke share it too: none of these are
// resident-facing operations, and there is no requirement carving out a
// broader read role set the way community-management's read/write split
// does (a documented default, not a spec requirement -- see unit-
// management's identical reasoning for unitManageRoles).
var invitationManageRoles = []authz.Role{authz.RoleAdmin, authz.RoleAdminStaff}

// invitationExpiry is the fixed 14-day expiry (invitations spec:
// "Fourteen-Day Expiry And Single Use"). This is a PRODUCT value, not an
// LPH legal deadline, so it is a Go constant, never a legal_rules row
// (rules.apply.guidelines' legal-rules-are-data rule applies only to LPH
// deadlines/majorities/percentages).
const invitationExpiry = 14 * 24 * time.Hour

// inviteLockoutThreshold/Window implement the enumeration lockout
// (invitations spec: "Enumeration Lockout Is IP+Device Scoped, Not
// Per-Invitation"; design D-6: "Threshold 10 per 15 minutes").
const (
	inviteLockoutThreshold = 10
	inviteLockoutWindow    = 15 * time.Minute
)

var (
	errInvitationSecretMissing   = errors.New("invitations: neither token nor short_code provided")
	errInvitationSecretAmbiguous = errors.New("invitations: both token and short_code provided")
)

// CreateInvitation implements POST /v1/communities/:id/invitations
// (invitations: Invitation Creation With Hashed, One-Time-Visible
// Secrets). Registered via scoped.Community with invitationManageRoles:
// an owner/tenant caller never reaches this handler (403).
func (d *Deps) CreateInvitation(ctx context.Context, in *dto.CreateInvitationInput, membership authz.Membership) (*dto.CreateInvitationOutput, error) {
	unitID, err := uuid.Parse(in.Body.UnitID)
	if err != nil {
		return nil, apperr.New(400, apperr.CodeValidation, "invalid unit_id", nil)
	}

	tx, err := d.DB.Write.Begin(ctx)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	// unit_id is client-supplied, so it MUST be re-checked against the
	// RESOLVED community, never trusted directly -- a unit id from
	// another community must never silently attach an invitation to a
	// foreign unit (design D-4: "Foreign resource → 404").
	unit, err := q.GetUnitByID(ctx, unitID)
	if err != nil {
		if isNoRows(err) {
			return nil, apperr.New(404, apperr.CodeNotFound, "unit not found", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if unit.CommunityID != membership.CommunityID() {
		return nil, apperr.New(404, apperr.CodeNotFound, "unit not found", nil)
	}

	rawToken, tokenHash, err := token.GenerateOpaqueToken()
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	rawShortCode, err := invitations.GenerateShortCode()
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	shortCodeHash := hashInviteSecret(rawShortCode)

	now := d.clock().Now()
	inv, err := q.InsertInvitation(ctx, db.InsertInvitationParams{
		ID:            uuid.New(),
		CommunityID:   membership.CommunityID(),
		UnitID:        pgtype.UUID{Bytes: unitID, Valid: true},
		Email:         pgtype.Text{String: in.Body.Email, Valid: true},
		Role:          in.Body.Role,
		TokenHash:     tokenHash,
		ShortCodeHash: shortCodeHash,
		ExpiresAt:     now.Add(invitationExpiry),
	})
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	// invitations spec: "MUST send an invitation email when email is
	// present" -- email is required at this milestone's application
	// layer (see dto.CreateInvitationRequest's doc comment), so this is
	// unconditional; the enqueue happens in the SAME transaction as the
	// invitation row (design D-6/D-7 interfaces table: river.InsertTx).
	if d.Queue != nil {
		if err := d.Queue.EnqueueInvitationEmail(ctx, tx, invitations.EmailArgs{
			InvitationID: inv.ID,
			Email:        in.Body.Email,
			ShortCode:    rawShortCode,
		}); err != nil {
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
	}

	callerID := membership.UserID()
	after, _ := json.Marshal(map[string]any{"invitation_id": inv.ID, "community_id": inv.CommunityID, "unit_id": unitID, "role": inv.Role})
	if _, err := audit.Append(ctx, tx, d.clock(), audit.Entry{
		UserID: &callerID, CommunityID: &inv.CommunityID, Action: "invitation.create", Entity: "invitation", EntityID: &inv.ID, After: after,
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.CreateInvitationOutput{Body: dto.CreateInvitationResponse{
		InvitationResponse: invitationResponse(inv, now),
		ShortCode:          rawShortCode,
		Token:              rawToken,
	}}, nil
}

// ListInvitations implements GET /v1/communities/:id/invitations
// (invitations: Cross-Tenant Isolation Proven By Test -- a foreign
// community's invitations are never present, enforced entirely by
// scoped.Community's resolver, never a caller-supplied filter).
func (d *Deps) ListInvitations(ctx context.Context, _ *dto.ListInvitationsInput, membership authz.Membership) (*dto.ListInvitationsOutput, error) {
	q := db.New(d.DB.Read)
	rows, err := q.ListInvitationsByCommunityID(ctx, membership.CommunityID())
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	now := d.clock().Now()
	out := make([]dto.InvitationResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, invitationResponse(r, now))
	}
	return &dto.ListInvitationsOutput{Body: dto.ListInvitationsResponse{Invitations: out}}, nil
}

// ResendInvitation implements POST /v1/invitations/:id/resend
// (invitations: Resend And Revoke). Registered via scoped.Invitation:
// community membership is resolved via the invitation's own
// community_id (design D-4), never a caller-supplied community id.
func (d *Deps) ResendInvitation(ctx context.Context, in *dto.ResendInvitationInput, membership authz.Membership) (*dto.ResendInvitationOutput, error) {
	invitationID, err := uuid.Parse(in.InvitationID)
	if err != nil {
		return nil, apperr.New(400, apperr.CodeValidation, "invalid id", nil)
	}

	tx, err := d.DB.Write.Begin(ctx)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	sentCount, err := q.IncrementInvitationSentCount(ctx, db.IncrementInvitationSentCountParams{
		ID: invitationID, CommunityID: membership.CommunityID(),
	})
	if err != nil {
		if isNoRows(err) {
			return nil, apperr.New(409, apperr.CodeConflict, "invitation is no longer pending", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	callerID := membership.UserID()
	communityID := membership.CommunityID()
	after, _ := json.Marshal(map[string]any{"invitation_id": invitationID, "sent_count": sentCount})
	if _, err := audit.Append(ctx, tx, d.clock(), audit.Entry{
		UserID: &callerID, CommunityID: &communityID, Action: "invitation.resend", Entity: "invitation", EntityID: &invitationID, After: after,
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.ResendInvitationOutput{Body: dto.ResendInvitationResponse{SentCount: sentCount}}, nil
}

// RevokeInvitation implements DELETE /v1/invitations/:id (invitations:
// Resend And Revoke; Cross-Tenant Isolation Proven By Test).
func (d *Deps) RevokeInvitation(ctx context.Context, in *dto.RevokeInvitationInput, membership authz.Membership) (*dto.RevokeInvitationOutput, error) {
	invitationID, err := uuid.Parse(in.InvitationID)
	if err != nil {
		return nil, apperr.New(400, apperr.CodeValidation, "invalid id", nil)
	}

	tx, err := d.DB.Write.Begin(ctx)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	communityID := membership.CommunityID()
	before, _ := json.Marshal(map[string]any{"invitation_id": invitationID, "status": "pending"})
	if _, err := q.RevokeInvitation(ctx, db.RevokeInvitationParams{ID: invitationID, CommunityID: communityID}); err != nil {
		if isNoRows(err) {
			return nil, apperr.New(409, apperr.CodeConflict, "invitation is no longer pending", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	callerID := membership.UserID()
	after, _ := json.Marshal(map[string]any{"invitation_id": invitationID, "status": "revoked"})
	if _, err := audit.Append(ctx, tx, d.clock(), audit.Entry{
		UserID: &callerID, CommunityID: &communityID, Action: "invitation.revoke", Entity: "invitation", EntityID: &invitationID, Before: before, After: after,
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.RevokeInvitationOutput{Body: dto.RevokeInvitationResponse{Status: "revoked"}}, nil
}

// PreviewInvitation implements POST /v1/invitations/preview (invitations:
// Preview Endpoint Is POST, Never GET With A Query Credential). It is
// registered via plain huma.Register (unauthenticated -- the caller has
// no account yet), never scoped.*, and never a GET route (enforced
// separately by TestInvitations_NoGetOperationAcceptsATokenOrShortCodeQueryParameter).
func (d *Deps) PreviewInvitation(ctx context.Context, in *dto.PreviewInvitationInput) (*dto.PreviewInvitationOutput, error) {
	ip := clientIP(ctx)
	key := inviteLockoutKey(ip)

	locked, err := d.inviteLocked(ctx, key)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if locked {
		return nil, tooManyInviteAttempts()
	}

	q := db.New(d.DB.Write)
	inv, err := d.resolveInvitationSecret(ctx, q, in.Body.Token, in.Body.ShortCode)
	if err != nil {
		if errors.Is(err, errInvitationSecretMissing) || errors.Is(err, errInvitationSecretAmbiguous) {
			return nil, apperr.New(400, apperr.CodeValidation, "exactly one of token or short_code is required", nil)
		}
		if isNoRows(err) {
			_, _ = d.InviteAttempts.Fail(ctx, key, inviteLockoutWindow)
			return nil, invitationNotFound()
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	_ = q.IncrementInvitationFailedAttempts(ctx, db.IncrementInvitationFailedAttemptsParams{ID: inv.ID, CommunityID: inv.CommunityID})

	if !invitationUsable(inv, d.clock().Now()) {
		// design D-6: "Unresolvable, expired, revoked and blocked all
		// return the same generic 404 body, so the endpoint is not an
		// existence oracle."
		return nil, invitationNotFound()
	}

	community, err := q.GetCommunityByID(ctx, inv.CommunityID)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.PreviewInvitationOutput{Body: dto.PreviewInvitationResponse{
		CommunityID:   inv.CommunityID,
		CommunityName: community.Name,
		UnitID:        pgtypeUUIDPtr(inv.UnitID),
		Role:          inv.Role,
		ExpiresAt:     inv.ExpiresAt,
	}}, nil
}

// AcceptInvitation implements POST /v1/auth/accept-invitation
// (invitations: Accept Creates Or Links An Account Without Revealing
// Prior Existence; Fourteen-Day Expiry And Single Use). It is
// unauthenticated, exactly like PreviewInvitation.
func (d *Deps) AcceptInvitation(ctx context.Context, in *dto.AcceptInvitationInput) (*dto.AcceptInvitationOutput, error) {
	ip := clientIP(ctx)
	key := inviteLockoutKey(ip)

	locked, err := d.inviteLocked(ctx, key)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if locked {
		return nil, tooManyInviteAttempts()
	}
	if !in.Body.Consent {
		return nil, apperr.New(400, apperr.CodeValidation, "consent is required", nil)
	}

	tx, err := d.DB.Write.Begin(ctx)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	inv, err := d.resolveInvitationSecret(ctx, q, in.Body.Token, in.Body.ShortCode)
	if err != nil {
		if errors.Is(err, errInvitationSecretMissing) || errors.Is(err, errInvitationSecretAmbiguous) {
			return nil, apperr.New(400, apperr.CodeValidation, "exactly one of token or short_code is required", nil)
		}
		if isNoRows(err) {
			_, _ = d.InviteAttempts.Fail(ctx, key, inviteLockoutWindow)
			return nil, invitationNotFound()
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if err := q.IncrementInvitationFailedAttempts(ctx, db.IncrementInvitationFailedAttemptsParams{ID: inv.ID, CommunityID: inv.CommunityID}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if perr := d.PasswordPolicy.Validate(ctx, in.Body.Password, false); perr != nil {
		var pe *password.PolicyError
		if errors.As(perr, &pe) {
			return nil, apperr.New(422, pe.Code, pe.Error(), pe.Details)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	unitID, ok := pgtypeUUIDValue(inv.UnitID)
	if !ok {
		return nil, apperr.New(500, apperr.CodeInternal, "invitation has no unit assigned", nil)
	}

	// design D-6: "Single use is enforced by the write, not by a prior
	// read" -- this conditional UPDATE is what actually rejects a
	// second accept or an accept past expiry; zero rows ⇒ 409.
	if _, err := q.AcceptInvitation(ctx, db.AcceptInvitationParams{ID: inv.ID, CommunityID: inv.CommunityID}); err != nil {
		if isNoRows(err) {
			return nil, apperr.New(409, apperr.CodeConflict, "invitation is no longer available", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	var userID uuid.UUID
	existing, err := q.GetUserByEmail(ctx, inv.Email.String)
	switch {
	case err == nil:
		// The invitation secret proves the CHANNEL, never the identity:
		// its creator chose the email freely and got the plaintext short
		// code back, so linking on the secret alone would let any
		// admin/admin_staff mint a session for an arbitrary existing
		// account (review lineage review-0e1833930adf141a). Linking an
		// existing account therefore requires that account's OWN
		// password, verified through the SAME primitive Login uses.
		// The whole accept rolls back on mismatch, so the invitation
		// stays usable for the genuine owner's next attempt.
		//
		// Because that rollback also leaves the short code replayable,
		// this branch is a credential-guessing surface and MUST be
		// throttled by the SAME account lockout POST /v1/auth/login
		// uses -- keyed on email+IP, with its escalating block window
		// and its victim alert -- not by the invitation enumeration
		// counter alone, which is per-address, coarser, silent, and
		// blind to which account is being guessed (review lineage
		// review-e72754dc7521b57a). Both counters run: they protect
		// different things (invitation-secret enumeration vs. this
		// account's credentials).
		accountLocked, lerr := d.Lockout.IsLocked(ctx, inv.Email.String, ip)
		if lerr != nil {
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		if accountLocked {
			return nil, apperr.New(429, apperr.CodeTooManyAttempts, "too many attempts, try again later", nil)
		}
		if ok, _ := password.Verify(ctx, existing.PasswordHash, in.Body.Password); !ok {
			_, _ = d.InviteAttempts.Fail(ctx, key, inviteLockoutWindow)
			// accountExists is unconditionally true here: this branch
			// is reached only because GetUserByEmail returned a row.
			_, _ = d.Lockout.RecordFailure(ctx, inv.Email.String, ip, true)
			return nil, invalidCredentials()
		}
		userID = existing.ID
	case isNoRows(err):
		hash, herr := password.Hash(in.Body.Password)
		if herr != nil {
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		newUser, uerr := q.InsertUser(ctx, db.InsertUserParams{
			ID:           uuid.New(),
			Email:        inv.Email.String,
			PasswordHash: hash,
			Name:         in.Body.Name,
			Phone:        optionalText(in.Body.Phone),
			Locale:       "es",
			IsSuperadmin: false,
		})
		if uerr != nil {
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		userID = newUser.ID
	default:
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	var consentAt pgtype.Timestamptz
	if in.Body.Consent {
		consentAt = pgtype.Timestamptz{Time: d.clock().Now(), Valid: true}
	}
	if _, err := q.InsertUnitMember(ctx, db.InsertUnitMemberParams{
		ID:                               uuid.New(),
		UnitID:                           unitID,
		CommunityID:                      inv.CommunityID,
		UserID:                           userID,
		Role:                             inv.Role,
		Tenure:                           "full_owner",
		ElectronicNotificationsConsentAt: consentAt,
	}); err != nil {
		if isUniqueViolation(err) {
			return nil, apperr.New(409, apperr.CodeConflict, "this account is already a member of this unit", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	after, _ := json.Marshal(map[string]any{"invitation_id": inv.ID, "unit_id": unitID, "user_id": userID})
	if _, err := audit.Append(ctx, tx, d.clock(), audit.Entry{
		UserID: &userID, CommunityID: &inv.CommunityID, Action: "invitation.accept", Entity: "invitation", EntityID: &inv.ID, After: after,
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	sessionOut, err := d.issueSession(ctx, db.New(d.DB.Write), userID, false, in.Body.DeviceName, in.Body.Platform, ip)
	if err != nil {
		return nil, err
	}
	return &dto.AcceptInvitationOutput{SetCookie: sessionOut.SetCookie, Body: sessionOut.Body}, nil
}

// inviteLocked reports whether key has already reached the enumeration
// lockout threshold, without recording a new attempt (design D-6:
// AttemptCounter.Count).
func (d *Deps) inviteLocked(ctx context.Context, key string) (bool, error) {
	count, err := d.InviteAttempts.Count(ctx, key, inviteLockoutWindow)
	if err != nil {
		return false, err
	}
	return count >= inviteLockoutThreshold, nil
}

// inviteLockoutKey builds the enumeration lockout key from the ADDRESS
// leg alone. D-6 originally appended a hash of the X-Platform +
// X-App-Version headers, but that same text calls the device leg
// spoofable and the address leg load-bearing: mixing a client-chosen
// value into the key let one caller mint a fresh zero counter per
// request simply by varying its app version, which defeated the
// threshold outright (review lineage review-0e1833930adf141a). Those
// headers stay on the request as evidence/telemetry; they never key
// the counter.
func inviteLockoutKey(ip string) string {
	return "invite:" + ip
}

// invitationsQuerier is the narrow subset of *db.Queries the invitation
// secret-resolution/response helpers below use, satisfied by both the
// write-pool Queries (preview/accept, which may run inside a
// transaction) and the read-pool Queries (list).
type invitationsQuerier interface {
	GetInvitationByTokenHash(ctx context.Context, tokenHash []byte) (db.Invitation, error)
	GetInvitationByShortCodeHash(ctx context.Context, shortCodeHash []byte) (db.Invitation, error)
}

// resolveInvitationSecret resolves EXACTLY one of token/shortCode to its
// invitation row. A caller providing both, or neither, is a validation
// error distinguishable from "not found" -- returning the SAME 404 for a
// malformed request as for an unresolvable secret would blur a client
// bug with an enumeration attempt.
func (d *Deps) resolveInvitationSecret(ctx context.Context, q invitationsQuerier, rawToken, rawShortCode string) (db.Invitation, error) {
	switch {
	case rawToken != "" && rawShortCode != "":
		return db.Invitation{}, errInvitationSecretAmbiguous
	case rawToken != "":
		return q.GetInvitationByTokenHash(ctx, hashInviteSecret(rawToken))
	case rawShortCode != "":
		return q.GetInvitationByShortCodeHash(ctx, hashInviteSecret(normalizeShortCode(rawShortCode)))
	default:
		return db.Invitation{}, errInvitationSecretMissing
	}
}

// normalizeShortCode uppercases and trims the caller-supplied short
// code so a hand-typed or read-aloud code (the "paper/voice" delivery
// path) still hashes to the same value invitations.GenerateShortCode's
// uppercase alphabet produced.
func normalizeShortCode(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// hashInviteSecret is the single hashing primitive for both the
// invitation token and short code (SHA-256, design D-6), reusing
// token.HashToken so there is exactly one hash implementation for every
// hashed secret in this codebase.
func hashInviteSecret(raw string) []byte {
	return token.HashToken(raw)
}

// invitationUsable reports whether inv can still be previewed/accepted:
// stored status must be pending AND not yet past expiry (design D-6: a
// pending invitation past expiry reads as expired even before the sweep
// job runs).
func invitationUsable(inv db.Invitation, now time.Time) bool {
	return inv.Status == "pending" && inv.ExpiresAt.After(now)
}

// invitationDisplayStatus is the READ-TIME DERIVED status (invitations:
// Explicit Status Column, "Pending invitation past expiry reads as
// expired").
func invitationDisplayStatus(inv db.Invitation, now time.Time) string {
	if inv.Status == "pending" && !inv.ExpiresAt.After(now) {
		return "expired"
	}
	return inv.Status
}

func invitationNotFound() error {
	return apperr.New(404, apperr.CodeNotFound, "invitation not found or no longer valid", nil)
}

// tooManyInviteAttempts is the enumeration lockout's 429 response,
// carrying a Retry-After header sized to the lockout window (design
// D-6: "Threshold 10 per 15 minutes → 429 with Retry-After"). The value
// is the fixed window itself, a conservative upper bound: neither
// AttemptCounter.Count nor .Fail exposes the exact remaining TTL for a
// key, so this deliberately never understates how long a caller should
// wait.
func tooManyInviteAttempts() error {
	base := apperr.New(429, apperr.CodeTooManyAttempts, "too many attempts, try again later", nil)
	return huma.ErrorWithHeaders(base, http.Header{
		"Retry-After": []string{strconv.Itoa(int(inviteLockoutWindow.Seconds()))},
	})
}

func invitationResponse(inv db.Invitation, now time.Time) dto.InvitationResponse {
	return dto.InvitationResponse{
		ID:          inv.ID,
		CommunityID: inv.CommunityID,
		UnitID:      pgtypeUUIDPtr(inv.UnitID),
		Email:       inv.Email.String,
		Role:        inv.Role,
		Status:      invitationDisplayStatus(inv, now),
		ExpiresAt:   inv.ExpiresAt,
		AcceptedAt:  timestamptzPtr(inv.AcceptedAt),
		SentCount:   inv.SentCount,
		CreatedAt:   inv.CreatedAt,
		UpdatedAt:   inv.UpdatedAt,
	}
}

func pgtypeUUIDPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := uuid.UUID(v.Bytes)
	return &id
}

func pgtypeUUIDValue(v pgtype.UUID) (uuid.UUID, bool) {
	if !v.Valid {
		return uuid.UUID{}, false
	}
	return uuid.UUID(v.Bytes), true
}

// RegisterInvitations wires the authenticated, scoped invitation
// operations (create/list/resend/revoke) into api. The caller MUST pass
// the Bearer-authenticated group (api.go's authGroup), never hapi
// directly -- see RegisterInvitationsPublic for preview/accept, which
// MUST NOT go through that group (design D-6).
func RegisterInvitations(api huma.API, d *Deps) {
	scoped.Community(api, huma.Operation{
		OperationID: "createInvitation",
		Method:      "POST",
		Path:        "/v1/communities/{id}/invitations",
		Summary:     "Invite a resident to a community (admin/admin_staff only)",
		Tags:        []string{"invitations"},
	}, invitationManageRoles, d.CreateInvitation)

	scoped.Community(api, huma.Operation{
		OperationID: "listInvitations",
		Method:      "GET",
		Path:        "/v1/communities/{id}/invitations",
		Summary:     "List a community's invitations (admin/admin_staff only)",
		Tags:        []string{"invitations"},
	}, invitationManageRoles, d.ListInvitations)

	scoped.Invitation(api, huma.Operation{
		OperationID: "resendInvitation",
		Method:      "POST",
		Path:        "/v1/invitations/{id}/resend",
		Summary:     "Resend an invitation (admin/admin_staff only)",
		Tags:        []string{"invitations"},
	}, invitationManageRoles, d.ResendInvitation)

	scoped.Invitation(api, huma.Operation{
		OperationID: "revokeInvitation",
		Method:      "DELETE",
		Path:        "/v1/invitations/{id}",
		Summary:     "Revoke an invitation (admin/admin_staff only)",
		Tags:        []string{"invitations"},
	}, invitationManageRoles, d.RevokeInvitation)
}

// RegisterInvitationsPublic wires preview/accept -- unauthenticated (the
// caller has no account yet), registered directly via plain
// huma.Register, and NEVER as a GET route (design D-6: "a credential in
// a query string leaks into access logs, proxies and Referer"). The
// caller MUST pass hapi directly (or an unauthenticated group), never
// the Bearer-authenticated authGroup: nesting a public route inside
// that group would apply its unrelated per-user rate limit whenever an
// already-authenticated caller happens to carry a bearer token, which
// this endpoint has no business depending on.
func RegisterInvitationsPublic(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "previewInvitation",
		Method:      "POST",
		Path:        "/v1/invitations/preview",
		Summary:     "Preview an invitation by token or short code (public, unauthenticated)",
		Tags:        []string{"invitations"},
	}, d.PreviewInvitation)

	huma.Register(api, huma.Operation{
		OperationID: "acceptInvitation",
		Method:      "POST",
		Path:        "/v1/auth/accept-invitation",
		Summary:     "Accept an invitation, creating or linking an account (public, unauthenticated)",
		Tags:        []string{"auth", "invitations"},
	}, d.AcceptInvitation)
}
