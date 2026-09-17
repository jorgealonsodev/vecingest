package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
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
	shortCodeHash := d.hashInviteShortCode(rawShortCode)

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
//
// IT RE-ISSUES THE SHORT CODE. That is a contract change, and it is a design
// decision rather than an implementation shortcut, so here is the whole of
// it. Resend used to increment sent_count, write an audit row, return 200 and
// dispatch nothing (R3-resend-invitation-dispatches-nothing, review lineage
// review-f855997b550a986d) -- and it could not do otherwise: the invitation
// row holds only one-way digests, short_code_hash being an HMAC-SHA-256 under
// ENCRYPTION_KEY, so the plaintext the email template renders does not exist
// anywhere at resend time.
//
// There were exactly two ways out, and they are not close:
//
//   - Persist a recoverable copy of the code (sealed the way the job payload
//     is). REJECTED. hashInviteShortCode below spends fifty lines explaining
//     that the named adversary is the one holding this table, and that the
//     defence is that a table dump contains no usable credential. Adding a
//     decryptable copy of every pending code to that same table, permanently,
//     to save re-issuing one, gives that adversary back exactly what the
//     digest was chosen to deny them -- and unlike the job row, which is
//     transient and only ever holds codes in flight, invitations rows sit
//     there for fourteen days.
//   - Re-issue on resend, invalidating the previous code. CHOSEN. It keeps
//     "only one-way digests are persisted" literally true, it costs one
//     UPDATE, and it matches what every other credential redelivery in this
//     codebase does: a forgotten password does not re-send the old reset
//     token, it mints a new one.
//
// The cost is stated plainly: a neighbour who still has the first code, on
// paper, finds it stops working the moment an administrator resends. That is
// the honest reading of a resend -- the administrator is asserting the first
// delivery did not arrive -- and it is strictly better than the alternative
// this replaces, which was an endpoint reporting success while delivering
// nothing at all.
//
// What resend does NOT rotate is the opaque token: the email carries the
// short code (mail/templates/invitation.html), so the token is a separate
// delivery channel that this endpoint does not re-deliver and therefore has
// no reason to invalidate. Revoke, not resend, is how an invitation whose
// secrets leaked is killed.
func (d *Deps) ResendInvitation(ctx context.Context, in *dto.ResendInvitationInput, membership authz.Membership) (*dto.ResendInvitationOutput, error) {
	invitationID, err := uuid.Parse(in.InvitationID)
	if err != nil {
		return nil, apperr.New(400, apperr.CodeValidation, "invalid id", nil)
	}

	rawShortCode, err := invitations.GenerateShortCode()
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	tx, err := d.DB.Write.Begin(ctx)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	rotated, err := q.RotateInvitationShortCode(ctx, db.RotateInvitationShortCodeParams{
		ID: invitationID, CommunityID: membership.CommunityID(), ShortCodeHash: d.hashInviteShortCode(rawShortCode),
	})
	if err != nil {
		if isNoRows(err) {
			return nil, apperr.New(409, apperr.CodeConflict, "invitation is no longer pending", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if !rotated.Email.Valid {
		return nil, apperr.New(422, apperr.CodeValidation, "this invitation has no email address to resend to", nil)
	}

	// The enqueue shares the transaction with the rotation (river.InsertTx,
	// design D-6), so the row can never end up carrying a new code whose
	// email was never queued, nor a queued email for a rotation that rolled
	// back. This is the same coupling CreateInvitation has, and it is the
	// thing whose ABSENCE here was the finding.
	if d.Queue != nil {
		if err := d.Queue.EnqueueInvitationEmail(ctx, tx, invitations.EmailArgs{
			InvitationID: invitationID,
			Email:        rotated.Email.String,
			ShortCode:    rawShortCode,
		}); err != nil {
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
	}

	callerID := membership.UserID()
	communityID := membership.CommunityID()
	after, _ := json.Marshal(map[string]any{"invitation_id": invitationID, "sent_count": rotated.SentCount, "short_code_rotated": true})
	if _, err := audit.Append(ctx, tx, d.clock(), audit.Entry{
		UserID: &callerID, CommunityID: &communityID, Action: "invitation.resend", Entity: "invitation", EntityID: &invitationID, After: after,
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.ResendInvitationOutput{Body: dto.ResendInvitationResponse{
		SentCount: rotated.SentCount,
		ShortCode: rawShortCode,
	}}, nil
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
	inv, err := d.resolveUsableInvitation(ctx, q, key, in.Body.Token, in.Body.ShortCode)
	if err != nil {
		return nil, err
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
//
// ORDERING INVARIANT -- read this before moving anything in this function.
// Four review rounds have each reordered these steps and each broken a
// different property doing it, so the order is the design, not an accident of
// how it was written:
//
//  1. ESTABLISH THE INVITATION IS USABLE. Resolve the secret, then
//     invitationUsable: pending, unexpired, not revoked, not consumed.
//     NOTHING EXPENSIVE MAY PRECEDE THIS. Everything below it costs either a
//     network round trip (HIBP), a deliberately slow memory-hard hash
//     (Argon2id), or somebody else's lockout budget, on an endpoint any
//     caller can reach.
//  2. THROTTLE. The enumeration counter first (it keys on the address alone
//     and gates step 1), then this invitation's own account lockout, which
//     needs the email step 1 resolved.
//  3. ONLY THEN THE CREDENTIAL WORK: password policy, password.Verify or
//     password.Hash, and the second-factor challenge.
//  4. THEN THE TRANSACTION, holding the writes and nothing else.
//
// What each inversion cost, so the next reader does not have to rediscover
// it: credential work above step 1 made every revoked, expired or already-
// consumed invitation an unbounded Argon2id-plus-HIBP amplifier and let a
// DEAD invitation keep driving the invited account's lockout
// (R1-accept-invitation-runs-argon2id-before-any-validity-check,
// R3-accept-invitation-status-checked-only-after-credential-work); a
// transaction above step 3 pinned a primary write connection and a row lock
// across all of it (R3-accept-invitation-holds-write-tx-across-password-work).
//
// Single use still comes from the conditional UPDATE in step 4 and NOT from
// step 1's read: step 1 is a cheap gate that rejects the dead invitations
// early, the UPDATE is the authority that settles concurrent accepts.
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

	// Step 1. Nothing writes between here and the transaction below, so
	// there is nothing to roll back; d.DB.Write is used for the reads only
	// because this flow must read its own writes.
	readQ := db.New(d.DB.Write)
	inv, err := d.resolveUsableInvitation(ctx, readQ, key, in.Body.Token, in.Body.ShortCode)
	if err != nil {
		return nil, err
	}
	unitID, ok := pgtypeUUIDValue(inv.UnitID)
	if !ok {
		return nil, apperr.New(500, apperr.CodeInternal, "invitation has no unit assigned", nil)
	}

	// Step 2, second leg. The invited address is the invitation's, never the
	// caller's, and the linking branch below is a credential-guessing
	// surface, so it is throttled by the SAME account lockout
	// POST /v1/auth/login uses -- keyed on email+IP, with its escalating
	// block window and its victim alert -- alongside the invitation
	// enumeration counter. Both run: they protect different things
	// (invitation-secret enumeration vs. this account's credentials).
	// Checking it HERE rather than after the account lookup means a locked
	// account stops costing an HIBP round trip per attempt.
	accountLocked, lerr := d.Lockout.IsLocked(ctx, inv.Email.String, ip)
	if lerr != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if accountLocked {
		return nil, apperr.New(429, apperr.CodeTooManyAttempts, "too many attempts, try again later", nil)
	}

	// Step 3. The policy runs on BOTH branches, deliberately: running it
	// only where a new password is being set would make a policy-violating
	// password answer 422 for an unknown address and 401 for a known one,
	// which is precisely the prior-existence disclosure this endpoint's
	// requirement forbids.
	if perr := d.PasswordPolicy.Validate(ctx, in.Body.Password, false); perr != nil {
		var pe *password.PolicyError
		if errors.As(perr, &pe) {
			return nil, apperr.New(422, pe.Code, pe.Error(), pe.Details)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	// Resolve WHICH account this accept is for, and prove the caller may
	// use it. newPasswordHash is non-empty only on the create branch.
	var (
		userID           uuid.UUID
		newPasswordHash  string
		mfaAuthenticated bool
	)
	existing, err := readQ.GetUserByEmail(ctx, inv.Email.String)
	switch {
	case err == nil:
		// The invitation secret proves the CHANNEL, never the identity:
		// its creator chose the email freely and got the plaintext short
		// code back, so linking on the secret alone would let any
		// admin/admin_staff mint a session for an arbitrary existing
		// account (review lineage review-0e1833930adf141a). Linking an
		// existing account therefore requires that account's OWN
		// credentials, verified through the SAME primitives Login uses.
		// A mismatch returns before any write happens at all, so the
		// invitation stays usable for the genuine owner's next attempt.
		if ok, _ := password.Verify(ctx, existing.PasswordHash, in.Body.Password); !ok {
			_, _ = d.InviteAttempts.Fail(ctx, key, inviteLockoutWindow)
			// accountExists is unconditionally true here: this branch
			// is reached only because GetUserByEmail returned a row.
			_, _ = d.Lockout.RecordFailure(ctx, inv.Email.String, ip, true)
			return nil, invalidCredentials()
		}
		if existing.IsSuperadmin {
			// Enumeration-safe, and the same refusal Login makes for the
			// same reason: a superadmin has no path through this endpoint
			// to satisfy the separate, code-gated superadmin login, and
			// auth_refresh re-derives the superadmin claim from the users
			// row, so one rotation of a session minted here would hand
			// back a superadmin token that never passed that route.
			_, _ = d.Lockout.RecordFailure(ctx, inv.Email.String, ip, true)
			return nil, invalidCredentials()
		}
		// The second factor, through the SAME challengeTOTP that guards
		// POST /v1/auth/login. Accept issues a full session, so an account
		// with an ACTIVE factor must present its code here too -- otherwise
		// the two session-minting routes disagree and a caller holding a
		// stolen password gets through the door that does not ask
		// (R1-accept-invitation-mints-a-session-without-the-totp-challenge,
		// review lineage review-f855997b550a986d). An account with no
		// factor is untouched, exactly as on login.
		mfaAuthenticated, err = d.challengeTOTP(ctx, readQ, existing, in.Body.TOTPCode)
		if err != nil {
			return nil, err
		}
		userID = existing.ID
	case isNoRows(err):
		// Argon2id, deliberately slow -- which is exactly why it runs
		// here and not inside the transaction below, and why nothing
		// reaches it until the invitation has been proven usable.
		newPasswordHash, err = password.Hash(in.Body.Password)
		if err != nil {
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
	default:
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	// Step 4. From here on, and only from here on, a write transaction. It
	// covers the single-use claim, the account/membership writes and the
	// audit entry as one unit, and nothing slow runs inside it.
	tx, err := d.DB.Write.Begin(ctx)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	// design D-6: "Single use is enforced by the write, not by a prior
	// read" -- this conditional UPDATE is what actually rejects a second
	// accept or an accept past expiry; zero rows ⇒ 409. A concurrent accept
	// that passed step 1 against the same row still loses here, and
	// TestInvitation_ConcurrentAcceptsOfTheSameCodeYieldExactlyOneSuccess
	// proves it.
	if _, err := q.AcceptInvitation(ctx, db.AcceptInvitationParams{ID: inv.ID, CommunityID: inv.CommunityID}); err != nil {
		if isNoRows(err) {
			return nil, apperr.New(409, apperr.CodeConflict, "invitation is no longer available", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if newPasswordHash != "" {
		newUser, uerr := q.InsertUser(ctx, db.InsertUserParams{
			ID:           uuid.New(),
			Email:        inv.Email.String,
			PasswordHash: newPasswordHash,
			Name:         in.Body.Name,
			Phone:        optionalText(in.Body.Phone),
			Locale:       "es",
			IsSuperadmin: false,
		})
		if uerr != nil {
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		userID = newUser.ID
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

	// mfaAuthenticated is whatever the challenge above established: true
	// when this accept presented a valid code, false on the create branch
	// (a brand-new account has no factor yet) and for an existing account
	// that has none. It is never hardcoded, because the session this issues
	// is an ordinary session and the mandatory-TOTP gate reads exactly this
	// fact off it.
	sessionOut, err := d.issueSession(ctx, db.New(d.DB.Write), userID, false, mfaAuthenticated, in.Body.DeviceName, in.Body.Platform, ip)
	if err != nil {
		return nil, err
	}
	return &dto.AcceptInvitationOutput{SetCookie: sessionOut.SetCookie, Body: sessionOut.Body}, nil
}

// resolveUsableInvitation is step 1 of AcceptInvitation's ordering invariant,
// and PreviewInvitation's identical first leg: resolve exactly one of
// token/short_code to its row, record the resolution, and reject anything that
// is not still pending and unexpired.
//
// It exists as ONE function because the asymmetry between these two handlers
// is what the last two review rounds found: preview checked invitationUsable
// immediately after resolution and accept never called it at all, so the
// expensive endpoint was the unguarded one. A shared gate makes that
// divergence impossible to reintroduce by editing one handler.
//
// A resolved-but-dead invitation advances the enumeration counter, exactly
// like an unresolvable secret: both answer the same generic 404 (design D-6 --
// the endpoint is not an existence oracle), so both must cost the caller the
// same budget, or replaying one known-dead code is free forever.
func (d *Deps) resolveUsableInvitation(ctx context.Context, q *db.Queries, key, rawToken, rawShortCode string) (db.Invitation, error) {
	inv, err := d.resolveInvitationSecret(ctx, q, rawToken, rawShortCode)
	if err != nil {
		if errors.Is(err, errInvitationSecretMissing) || errors.Is(err, errInvitationSecretAmbiguous) {
			return db.Invitation{}, apperr.New(400, apperr.CodeValidation, "exactly one of token or short_code is required", nil)
		}
		if isNoRows(err) {
			_, _ = d.InviteAttempts.Fail(ctx, key, inviteLockoutWindow)
			return db.Invitation{}, invitationNotFound()
		}
		return db.Invitation{}, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	// Gate evidence, not the gate: the lockout itself is the IP-scoped
	// AttemptCounter above (design D-6, IncrementInvitationFailedAttempts'
	// own doc comment). Best-effort, so a bookkeeping failure never decides
	// whether a valid invitation may be used.
	_ = q.IncrementInvitationFailedAttempts(ctx, db.IncrementInvitationFailedAttemptsParams{ID: inv.ID, CommunityID: inv.CommunityID})

	if !invitationUsable(inv, d.clock().Now()) {
		_, _ = d.InviteAttempts.Fail(ctx, key, inviteLockoutWindow)
		return db.Invitation{}, invitationNotFound()
	}
	return inv, nil
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
		return q.GetInvitationByShortCodeHash(ctx, d.hashInviteShortCode(normalizeShortCode(rawShortCode)))
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

// hashInviteSecret is the hashing primitive for the invitation TOKEN
// (SHA-256, design D-6), reusing token.HashToken so there is exactly one
// implementation for every HIGH-ENTROPY hashed secret in this codebase.
// The opaque token is 256 bits of crypto/rand, so an unsalted, unkeyed
// digest is sound for it: there is no candidate list to enumerate.
func hashInviteSecret(raw string) []byte {
	return token.HashToken(raw)
}

// hashInviteShortCode is deliberately NOT that primitive
// (R1-invitation-short-code-hash-offline-recoverable, review lineage
// review-c4efc3f92d076299). The short code is eight symbols from a
// 32-symbol alphabet -- about 40 bits -- so the entire preimage space is
// 2^40 candidates, and a single-round unsalted SHA-256 under a UNIQUE
// index turned "only hashes are persisted" into a guarantee that holds
// on paper and not in fact: minutes of commodity GPU work recover every
// pending code from a table dump, a backup or a read replica.
//
// It is an HMAC-SHA-256 under ENCRYPTION_KEY, and each part of that was
// a choice against a real alternative:
//
//   - NOT a per-row salt. The lookup path resolves an invitation FROM
//     the code (GetInvitationByShortCodeHash) and short_code_hash is
//     UNIQUE. A per-row salt makes the digest unreproducible without
//     first finding the row, which is the row you are trying to find --
//     it would force a table scan plus one KDF per row, and the UNIQUE
//     constraint would have to go.
//
//   - NOT a slow KDF with a fixed pepper. It would work, and it would
//     put an Argon2id-class cost on every preview and accept: two
//     UNAUTHENTICATED endpoints. That is the identical primary-pool
//     denial-of-service lever this same round had to remove from
//     AcceptInvitation, reintroduced one layer down.
//
//   - NOT more entropy in the code. The code's length is a usability
//     constraint, not an implementation detail: it is read aloud and
//     copied from paper by a neighbour (design D-6's "paper/voice"
//     delivery path), and the alphabet already excludes O/0/I/1 to make
//     that survivable. Twelve or sixteen characters would buy the bits
//     by making the delivery path materially worse.
//
// A keyed MAC keeps the digest deterministic -- so the UNIQUE index and
// the single indexed lookup are untouched -- and is constant-cost, while
// moving the secret OUT of the database entirely. The named adversary
// holds the table; ENCRYPTION_KEY is in the process environment (config
// unsets it after load), which is exactly the trust boundary the round-2
// fix already relies on when it seals this same short code into the
// river_job row. Reusing that key means no new configuration variable
// and no new secret to rotate.
//
// What it does NOT defend against: an attacker who has the key as well
// as the table can still enumerate 2^40 candidates. That is the accepted
// residual, and it is the same one every keyed-digest scheme carries.
func (d *Deps) hashInviteShortCode(raw string) []byte {
	mac := hmac.New(sha256.New, d.MFAKey[:])
	mac.Write([]byte(raw))
	return mac.Sum(nil)
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
