package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/audit"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
	"github.com/jorgealonsodev/vecingest/internal/http/dto"
)

// EnrollMFA implements POST /v1/me/mfa/enroll (auth-mfa-totp delta:
// Non-Superadmin TOTP HTTP Endpoints). It wires M0's existing
// internal/domain/auth/mfa.Enroll -- no new domain logic -- for ANY
// authenticated caller: superadmin already has its own mandatory-TOTP
// path at /v1/auth/superadmin/login; this is the first HTTP surface for
// everyone else, and the ONLY way an admin/admin_staff without TOTP can
// ever satisfy the mandatory-TOTP gate (authz.resolve.go), since this
// route is NOT under a scoped prefix and therefore never itself gated.
//
// It stays ungated ON PURPOSE. A password-only session can still reach
// this route -- an admin with no factor has no other way to acquire one
// -- and enrolling opens no gate by itself: the gate reads the SESSION's
// own second-factor fact, and the session doing the enrolling was minted
// without it (requireMFAForAdminRoles; review lineage
// review-0e1833930adf141a).
//
// What a password alone can no longer do is ACTIVATE the factor. With no
// disable or recovery path, the first active factor is permanent, so an
// attacker binding their own authenticator to a factor-less owner's
// account locked the owner out for good. Enrollment therefore also
// issues a 6-digit code to the STORED users.email, in the same
// transaction as the pending secret, and confirmation requires it
// (auth-mfa-totp: Email-Confirmed Enrollment). Re-enrolling supersedes
// the previous code along with the previous secret. Because each code
// carries its own attempt budget, issuance itself is capped
// (mfa.EnrollEmailIssueLimit per mfa.EnrollEmailIssueWindow, 429 past
// it): otherwise re-enrolling would mint unlimited guesses.
//
// Re-enrolling an ALREADY-ACTIVE factor is refused (409): silently
// replacing a live secret would strand the caller's current
// authenticator app with no warning.
func (d *Deps) EnrollMFA(ctx context.Context, in *dto.MFAEnrollInput) (*dto.MFAEnrollOutput, error) {
	claims, err := d.authenticate(ctx, in.Authorization)
	if err != nil {
		return nil, unauthorized(err)
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, apperr.New(401, apperr.CodeUnauthorized, "invalid token subject", nil)
	}

	if d.MFAEnrollQueue == nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	tx, err := d.DB.Write.Begin(ctx)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock order, shared with confirmMFAEnrollment: users row, then
	// otp_challenges, then user_mfa. The user lock also makes the
	// already-active check below and the issuance cap race-free: a
	// concurrent confirmation cannot activate the factor between the
	// check and the secret upsert, and concurrent enrollments cannot
	// all count below the cap.
	q := db.New(tx)
	email, err := q.LockUserForMFAEnrollment(ctx, userID)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	existing, err := q.GetUserMFA(ctx, userID)
	if err == nil && existing.EnabledAt.Valid {
		return nil, apperr.New(409, apperr.CodeConflict, "TOTP is already active on this account", nil)
	}
	if err != nil && !isNoRows(err) {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	challengeID, code, err := mfa.IssueEnrollEmailChallenge(ctx, tx, d.clock(), d.MFAKey, userID)
	if errors.Is(err, mfa.ErrEnrollEmailIssueLimited) {
		return nil, tooManyEnrollEmailCodes()
	}
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	result, err := mfa.Enroll(ctx, tx, d.MFAKey, userID)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if err := d.MFAEnrollQueue.EnqueueMFAEnrollEmail(ctx, tx, mfa.EnrollEmail{
		UserID: userID, ChallengeID: challengeID, Email: email, Code: code,
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.MFAEnrollOutput{Body: dto.MFAEnrollResponse{
		Secret:          mfa.Base32Secret(result.Secret),
		ProvisioningURI: mfa.ProvisioningURI(result.Secret, email),
	}}, nil
}

// tooManyEnrollEmailCodes is EnrollMFA's 429 once the caller reached
// mfa.EnrollEmailIssueLimit. Retry-After is the whole window, the same
// conservative upper bound tooManyInviteAttempts uses: the oldest code
// in the window may age out sooner, but this never understates the wait.
func tooManyEnrollEmailCodes() error {
	base := apperr.New(429, apperr.CodeTooManyAttempts, "too many enrollment codes requested, try again later", nil)
	return huma.ErrorWithHeaders(base, http.Header{
		"Retry-After": []string{strconv.Itoa(int(mfa.EnrollEmailIssueWindow.Seconds()))},
	})
}

// VerifyMFA implements POST /v1/me/mfa/verify (auth-mfa-totp delta:
// Non-Superadmin TOTP HTTP Endpoints, both scenarios). A caller with no
// active TOTP yet is completing enrollment (mfa.ConfirmEnrollment,
// generating and returning one-time recovery codes -- auth-mfa-totp:
// One-Time Recovery Codes); one already active is performing an
// ordinary throttled verification (mfa.ThrottledVerify + mfa.VerifyTOTP)
// -- both scenarios go through this SAME endpoint, exactly as the
// spec's own two scenarios exercise it ("verifies a valid code ... and
// becomes active" vs. "submits a valid TOTP code ... verification
// succeeds"). Activation is a sensitive action, audited with
// before/after (rules.apply.guidelines).
func (d *Deps) VerifyMFA(ctx context.Context, in *dto.MFAVerifyInput) (*dto.MFAVerifyOutput, error) {
	claims, err := d.authenticate(ctx, in.Authorization)
	if err != nil {
		return nil, unauthorized(err)
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, apperr.New(401, apperr.CodeUnauthorized, "invalid token subject", nil)
	}

	q := db.New(d.DB.Write)
	row, err := q.GetUserMFA(ctx, userID)
	if err != nil {
		if isNoRows(err) {
			return nil, apperr.New(400, apperr.CodeValidation, "call the enroll endpoint first", nil)
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	if !row.EnabledAt.Valid {
		return d.confirmMFAEnrollment(ctx, userID, in.Body.Code, in.Body.EmailCode)
	}
	return d.verifyActiveMFA(ctx, row, userID, in.Body.Code)
}

// confirmMFAEnrollment handles VerifyMFA's first-activation branch.
// The enabled_at write, recovery-code persistence and the mfa.enroll
// audit entry are ONE transaction (review lineage
// review-0e1833930adf141a): as three separate writes, a failure after
// activation left the account with a live second factor, an empty
// recovery-code set, and no way back -- the retry took the
// already-active branch, which never issues codes. Rolling activation
// back instead keeps the retry on this branch.
//
// Activation also requires the email code EnrollMFA issued
// (auth-mfa-totp: Email-Confirmed Enrollment), checked in this order:
//
//  1. One attempt is spent on the latest open challenge in its own
//     autocommitted statement, BEFORE anything is compared, and refused
//     when the challenge is expired or already has
//     mfa.EnrollEmailMaxAttempts. The increment must outlive the
//     rejection that may follow, and the rejection rolls back the
//     activation transaction -- so it cannot live in it.
//  2. Inside the activation transaction the challenge is re-read FOR
//     UPDATE and must still be open, so two concurrent correct
//     submissions cannot both spend it.
//  3. Both codes are checked; any failure is the same generic 401.
//  4. The challenge is marked verified in the activation transaction:
//     a rolled-back activation (see above) leaves it usable for the
//     retry, and a committed one leaves it spent.
func (d *Deps) confirmMFAEnrollment(ctx context.Context, userID uuid.UUID, code, emailCode string) (*dto.MFAVerifyOutput, error) {
	rejected := apperr.New(401, apperr.CodeMFAEnrollmentConfirmationInvalid,
		"invalid enrollment confirmation: check both the authenticator code and the emailed code, or enroll again for a new email code", nil)
	if emailCode == "" {
		return nil, rejected
	}

	now := d.clock().Now()
	challenge, err := db.New(d.DB.Write).ConsumeOTPChallengeAttempt(ctx, db.ConsumeOTPChallengeAttemptParams{
		UserID: userID, Purpose: mfa.EnrollEmailPurpose, Now: now, MaxAttempts: mfa.EnrollEmailMaxAttempts,
	})
	if err != nil {
		if isNoRows(err) {
			return nil, rejected
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	tx, err := d.DB.Write.Begin(ctx)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Same lock order as EnrollMFA (users row, otp_challenges, user_mfa):
	// a re-enrollment and this confirmation used to lock user_mfa and the
	// challenge in opposite orders and could deadlock.
	if _, err := db.New(tx).LockUserForMFAEnrollment(ctx, userID); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	locked, err := db.New(tx).LockOpenOTPChallenge(ctx, challenge.ID)
	if err != nil {
		if isNoRows(err) {
			return nil, rejected
		}
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	// Re-checked under the lock: a re-enrollment that committed between
	// step 1 and here expired this challenge along with its secret. The
	// clock is read again after the lock: that invalidation set expires_at
	// to its own clock reading, which is later than the step-1 now.
	if !locked.ExpiresAt.After(d.clock().Now()) || !mfa.EnrollEmailCodeMatches(d.MFAKey, emailCode, locked.CodeHash) {
		return nil, rejected
	}

	ok, err := mfa.ConfirmEnrollment(ctx, tx, d.clock(), d.MFAKey, userID, code)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if !ok {
		return nil, rejected
	}
	if err := db.New(tx).MarkOTPChallengeVerified(ctx, locked.ID); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	rawCodes, hashedCodes, err := d.recoveryCodes()
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if err := db.New(tx).SetUserMFARecoveryCodes(ctx, db.SetUserMFARecoveryCodesParams{
		UserID: userID, RecoveryCodesHashed: hashedCodes,
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	before, _ := json.Marshal(map[string]any{"enabled": false})
	after, _ := json.Marshal(map[string]any{"enabled": true})
	if _, err := audit.Append(ctx, tx, d.clock(), audit.Entry{
		UserID: &userID, Action: "mfa.enroll", Entity: "user_mfa", EntityID: &userID, Before: before, After: after,
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	return &dto.MFAVerifyOutput{Body: dto.MFAVerifyResponse{Active: true, RecoveryCodes: rawCodes}}, nil
}

// verifyActiveMFA handles VerifyMFA's steady-state branch: an
// already-active factor, verified through the identical
// mfa.ThrottledVerify wrapper SuperadminLogin uses.
func (d *Deps) verifyActiveMFA(ctx context.Context, row db.UserMfa, userID uuid.UUID, code string) (*dto.MFAVerifyOutput, error) {
	accepted, throttled, err := mfa.ThrottledVerify(ctx, d.MFACounter, userID, func(ctx context.Context) (bool, error) {
		secret, derr := mfa.DecryptSecret(d.MFAKey, row.TotpSecretEncrypted)
		if derr != nil {
			return false, derr
		}
		outcome, verr := mfa.VerifyTOTP(ctx, d.DB.Write, d.clock(), userID, secret, code)
		if verr != nil {
			return false, verr
		}
		return outcome == mfa.OutcomeAccepted, nil
	})
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if throttled {
		return nil, apperr.New(429, apperr.CodeTOTPThrottled, "too many TOTP attempts, try again later", nil)
	}
	if !accepted {
		return nil, apperr.New(401, apperr.CodeTOTPInvalid, "invalid TOTP code", nil)
	}

	return &dto.MFAVerifyOutput{Body: dto.MFAVerifyResponse{Active: true}}, nil
}

// RegisterMFA wires POST /v1/me/mfa/enroll and POST /v1/me/mfa/verify
// into api. Neither path sits under a scoped prefix (/v1/communities,
// /v1/units, /v1/offices), so neither needs -- or may use -- a
// scoped.* constructor (authz's A1 check only requires one for those
// prefixes): these are self-scoped operations on the caller's OWN
// account, exactly like GET /v1/me.
func RegisterMFA(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "enrollMFA",
		Method:      "POST",
		Path:        "/v1/me/mfa/enroll",
		Summary:     "Start TOTP enrollment for the caller's own account",
		Tags:        []string{"mfa"},
	}, d.EnrollMFA)

	huma.Register(api, huma.Operation{
		OperationID: "verifyMFA",
		Method:      "POST",
		Path:        "/v1/me/mfa/verify",
		Summary:     "Verify a TOTP code -- completes enrollment on the first call, an ordinary check afterward",
		Tags:        []string{"mfa"},
	}, d.VerifyMFA)
}
