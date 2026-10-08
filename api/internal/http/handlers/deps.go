package handlers

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/captcha"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/lockout"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/password"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/session"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
	"github.com/jorgealonsodev/vecingest/internal/domain/invitations"
	"github.com/jorgealonsodev/vecingest/internal/platform/cache"
)

// Clock abstracts time.Now() across every handler. It structurally
// satisfies token.Clock, session.Clock, audit.Clock and mfa.Clock (all
// declare the identical Now() time.Time method), so one injected value
// threads through the whole request path with no adapter.
type Clock interface{ Now() time.Time }

// Deps carries every dependency an M0 HTTP handler needs. Handlers
// themselves contain no policy: every field here is a domain/platform
// type built and owned by internal/domain or internal/platform (PRD §9).
type Deps struct {
	DB db.Handles

	AccessIssuer token.Issuer
	CSRFKey      []byte
	Rotator      session.Rotator
	Clock        Clock

	Lockout        lockout.Service
	PasswordPolicy password.PasswordPolicy

	MFAKey     [32]byte
	MFACounter mfa.AttemptCounter
	// RecoveryCodes is the recovery-code generator TOTP activation
	// calls, defaulting to mfa.GenerateRecoveryCodes when nil (the
	// production wiring). It is a seam so a test can inject a failure
	// between the enabled_at write and recovery-code persistence and
	// prove those two are ATOMIC (auth-mfa-totp: One-Time Recovery
	// Codes) -- an active factor with no recovery path is unreachable
	// through any endpoint and would strand the account permanently.
	RecoveryCodes func() (raw []string, hashed []string, err error)

	RevocationCache *cache.RevocationCache

	// CookieDomain is api.<DOMAIN> (D-E).
	CookieDomain string
	// AllowedOrigins is the exact allowlist CORS credentials and the
	// refresh-endpoint Origin check are ever satisfied by
	// (auth-csrf-origin: Origin Validation on Refresh).
	AllowedOrigins []string

	ResetRequester password.ResetRequester
	TokenIssuer    password.TokenIssuer

	// InviteAttempts is the invitation enumeration-lockout counter
	// (design D-6: "existing AttemptCounter seam ... key
	// invite:{ip}:{deviceHash}"). Reuses mfa.AttemptCounter's identical
	// Fail/Count/Reset shape rather than declaring a fourth structurally
	// identical interface -- Go's structural typing means
	// internal/platform/attempts.Counter satisfies all of them with no
	// adapter, exactly as it already does for Lockout.Counter and
	// MFACounter.
	InviteAttempts mfa.AttemptCounter
	// Queue is the phase-A/B background-job seam invitations touches
	// for the first time as a real producer (design's Interfaces/
	// Contracts table). nil is a legitimate value only in a test that
	// does not exercise invitation creation; production wiring
	// (buildServeDeps) always sets it.
	Queue invitations.Queue
	// Captcha is the CaptchaVerifier seam (public-form-protection: new,
	// 6th seam) wired into login-after-third-failure and forgot-
	// password. nil is treated as "never passes" by verifyCaptcha
	// (captcha.go): a deployment that forgets to wire it fails CLOSED,
	// never open.
	Captcha captcha.Verifier
}

// recoveryCodes resolves d.RecoveryCodes against its production
// default, mirroring d.clock()'s nil-means-default pattern.
func (d *Deps) recoveryCodes() (raw []string, hashed []string, err error) {
	if d.RecoveryCodes != nil {
		return d.RecoveryCodes()
	}
	return mfa.GenerateRecoveryCodes()
}

func (d *Deps) clock() Clock {
	if d.Clock != nil {
		return d.Clock
	}
	return systemClock{}
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// allowedOrigin reports whether origin is in d.AllowedOrigins.
func (d *Deps) allowedOrigin(origin string) bool {
	for _, o := range d.AllowedOrigins {
		if o == origin {
			return true
		}
	}
	return false
}

// userLookup adapts db.Queries.GetUserByEmail to password.UserLookup.
type userLookup struct{ q *db.Queries }

func (u userLookup) LookupByEmail(ctx context.Context, email string) (uuid.UUID, bool, error) {
	row, err := u.q.GetUserByEmail(ctx, email)
	if err != nil {
		if isNoRows(err) {
			return uuid.UUID{}, false, nil
		}
		return uuid.UUID{}, false, err
	}
	return row.ID, true, nil
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505), mirroring
// internal/domain/auth/mfa.isSerializationFailure's classification
// pattern (an interface{ SQLState() string } check via errors.As,
// rather than importing pgconn directly) -- unit-management: Unit
// Uniqueness Per Community (task 4.4) maps this to a 409, never a raw
// 500.
func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == pgerrcode.UniqueViolation
	}
	return false
}
