package handlers

import (
	"context"
	"net/netip"

	"github.com/danielgtaylor/huma/v2"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/password"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
	"github.com/jorgealonsodev/vecingest/internal/http/apperr"
	"github.com/jorgealonsodev/vecingest/internal/http/dto"
)

// Login implements POST /v1/auth/login (auth-credentials;
// auth-session-tokens: JWT Access and Refresh Issuance). It refuses a
// superadmin account outright: that account has no path through this
// endpoint to satisfy auth-mfa-totp's Mandatory TOTP requirement, so
// allowing it to fully authenticate here would silently bypass the
// separate, TOTP-gated /v1/auth/superadmin/login route.
func (d *Deps) Login(ctx context.Context, in *dto.LoginInput) (*dto.LoginOutput, error) {
	ip := clientIP(ctx)

	locked, err := d.Lockout.IsLocked(ctx, in.Body.Email, ip)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if locked {
		return nil, apperr.New(429, apperr.CodeTooManyAttempts, "too many attempts, try again later", nil)
	}

	// public-form-protection: Turnstile Required After The Third Login
	// Failure. Checked BEFORE any credential lookup or RecordFailure
	// call, so a captcha-rejected attempt never itself advances the
	// lockout counters and never runs any protected-form logic.
	failures, err := d.Lockout.FailureCount(ctx, in.Body.Email, ip)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if failures >= captchaAfterFailures {
		ok, cerr := d.verifyCaptcha(ctx, in.Body.TurnstileToken, ip)
		if cerr != nil {
			// Login stays FAIL-CLOSED on an unreachable verifier, the
			// opposite of forgot-password's deliberate degradation
			// (review lineage review-e72754dc7521b57a): a caller
			// already past captchaAfterFailures failures is the exact
			// credential-stuffing shape this check exists for, and
			// they have a remedy a locked-out password-recovery user
			// does not -- waiting out the window. The failure is still
			// recorded, so forgot-password can see the outage.
			d.recordCaptchaOutage(ctx)
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		if !ok {
			return nil, captchaRequired()
		}
	}

	q := db.New(d.DB.Write)
	user, err := q.GetUserByEmail(ctx, in.Body.Email)
	if err != nil {
		if !isNoRows(err) {
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		password.VerifyAgainstDummy(ctx, in.Body.Password)
		_, _ = d.Lockout.RecordFailure(ctx, in.Body.Email, ip, false)
		return nil, invalidCredentials()
	}

	ok, needsRehash := password.Verify(ctx, user.PasswordHash, in.Body.Password)
	if !ok {
		_, _ = d.Lockout.RecordFailure(ctx, in.Body.Email, ip, true)
		return nil, invalidCredentials()
	}
	if user.IsSuperadmin {
		// Enumeration-safe: identical error to a wrong password, never
		// revealing that the account exists and is a superadmin.
		_, _ = d.Lockout.RecordFailure(ctx, in.Body.Email, ip, true)
		return nil, invalidCredentials()
	}

	// The second-factor challenge runs BEFORE RecordSuccess, exactly as
	// SuperadminLogin's does: a password that is correct but unaccompanied
	// by the code this account requires has not completed a login, so it
	// must not clear the lockout counters either.
	mfaAuthenticated, err := d.challengeTOTP(ctx, q, user, in.Body.TOTPCode)
	if err != nil {
		return nil, err
	}

	if err := d.Lockout.RecordSuccess(ctx, in.Body.Email); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if needsRehash {
		if newHash, herr := password.Hash(in.Body.Password); herr == nil {
			_ = q.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{ID: user.ID, PasswordHash: newHash})
		}
	}

	return d.issueSession(ctx, q, user.ID, false, mfaAuthenticated, in.Body.DeviceName, in.Body.Platform, ip)
}

// challengeTOTP runs the second-factor leg of a non-superadmin login
// (auth-mfa-totp; review lineage review-0e1833930adf141a,
// R1-mandatory-totp-gate-is-only-an-enrollment-flag) and reports whether
// the session about to be issued is second-factor authenticated.
//
// An account with NO active factor is untouched: it returns false with
// no error, and the caller gets an ordinary password-only session. That
// is deliberate on two counts -- TOTP stays optional for owners and
// tenants (PRD §10.1 mandates it for admins only), and an admin who has
// not enrolled yet must still be able to log in, or there would be no
// path to /v1/me/mfa/enroll and the account would be unreachable.
//
// An account WITH an active factor must present a valid code. The
// verification is the SAME throttled path SuperadminLogin uses --
// mfa.DecryptSecret, then mfa.ThrottledVerify wrapping mfa.VerifyTOTP
// with its replay protection -- not a second implementation of it.
func (d *Deps) challengeTOTP(ctx context.Context, q *db.Queries, user db.User, code string) (bool, error) {
	mfaRow, err := q.GetUserMFA(ctx, user.ID)
	if err != nil {
		if isNoRows(err) {
			return false, nil
		}
		return false, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if !mfaRow.EnabledAt.Valid {
		// Enrollment started but never confirmed: there is nothing to
		// verify a code against, so this is still a password-only
		// account.
		return false, nil
	}

	if code == "" {
		// Distinct from invalid credentials on purpose: the password
		// WAS right, and the client needs to know to prompt for a code
		// rather than tell the user their password is wrong. This
		// discloses nothing an attacker could not already infer by
		// trying a code, and it is reached only after a correct
		// password.
		return false, apperr.New(403, apperr.CodeMFARequired, "TOTP code required for this account", nil)
	}

	secret, err := mfa.DecryptSecret(d.MFAKey, mfaRow.TotpSecretEncrypted)
	if err != nil {
		return false, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	accepted, throttled, err := mfa.ThrottledVerify(ctx, d.MFACounter, user.ID, func(ctx context.Context) (bool, error) {
		outcome, verr := mfa.VerifyTOTP(ctx, d.DB.Write, d.clock(), user.ID, secret, code)
		if verr != nil {
			return false, verr
		}
		return outcome == mfa.OutcomeAccepted, nil
	})
	if err != nil {
		return false, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	if throttled {
		return false, apperr.New(429, apperr.CodeTOTPThrottled, "too many TOTP attempts, try again later", nil)
	}
	if !accepted {
		return false, apperr.New(401, apperr.CodeTOTPInvalid, "invalid TOTP code", nil)
	}
	return true, nil
}

// issueSession creates a brand-new session (a fresh family_id -- every
// login starts a new family; only rotation shares one) and renders the
// LoginResponse/LoginOutput shape shared by login, superadmin login and
// refresh.
//
// mfaAuthenticated lands in BOTH the session row (sessions.mfa_at) and
// the access token (the mfa claim). The row is not redundant with the
// claim: an access token lives 15 minutes, and refresh has no other way
// to learn how the original login authenticated, so without the
// persisted fact every elevated session would quietly decay into a
// password-only one at its first rotation.
func (d *Deps) issueSession(ctx context.Context, q *db.Queries, userID uuid.UUID, isSuperadmin, mfaAuthenticated bool, deviceName, platform, ip string) (*dto.LoginOutput, error) {
	familyID := uuid.New()
	sessionID := uuid.New()
	rawRefresh, refreshHash, err := token.GenerateRefresh()
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}
	now := d.clock().Now()
	expiresAt := now.Add(token.RefreshLifetime(isSuperadmin))

	var deviceNameCol pgtype.Text
	if deviceName != "" {
		deviceNameCol = pgtype.Text{String: deviceName, Valid: true}
	}
	var ipAddr *netip.Addr
	if parsed, perr := netip.ParseAddr(ip); perr == nil {
		ipAddr = &parsed
	}

	if _, err := q.InsertSession(ctx, db.InsertSessionParams{
		ID:               sessionID,
		UserID:           userID,
		RefreshTokenHash: refreshHash,
		FamilyID:         familyID,
		DeviceName:       deviceNameCol,
		Platform:         platform,
		Ip:               ipAddr,
		ExpiresAt:        expiresAt,
		MfaAt:            pgtype.Timestamptz{Time: now, Valid: mfaAuthenticated},
	}); err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	access, err := d.AccessIssuer.IssueAccess(userID, familyID, isSuperadmin, mfaAuthenticated)
	if err != nil {
		return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
	}

	body := dto.LoginResponse{
		AccessToken: access,
		ExpiresIn:   int64(token.AccessTTL.Seconds()),
	}
	out := &dto.LoginOutput{Body: body}

	if platform == "web" {
		out.SetCookie = RefreshCookie(d.CookieDomain, rawRefresh, expiresAt)
		csrf, cerr := token.MintCSRF(d.CSRFKey, familyID, sessionID, expiresAt)
		if cerr != nil {
			return nil, apperr.New(500, apperr.CodeInternal, "internal error", nil)
		}
		out.Body.CSRFToken = csrf
	} else {
		out.Body.RefreshToken = rawRefresh
	}

	return out, nil
}

func invalidCredentials() error {
	return apperr.New(401, apperr.CodeInvalidCredentials, "invalid email or password", nil)
}

// clientIP reads the RESOLVED client IP set by router step 2
// (middleware.ClientIPFromXFF), never a raw header (D-H).
func clientIP(ctx context.Context) string {
	return chimiddleware.GetClientIP(ctx)
}

// RegisterLogin wires POST /v1/auth/login into api.
func RegisterLogin(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "login",
		Method:      "POST",
		Path:        "/v1/auth/login",
		Summary:     "Password login",
		Tags:        []string{"auth"},
	}, d.Login)
}
