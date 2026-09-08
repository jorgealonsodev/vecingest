package handlers

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/lockout"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/password"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/session"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
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

	RevocationCache *cache.RevocationCache

	// CookieDomain is api.<DOMAIN> (D-E).
	CookieDomain string
	// AllowedOrigins is the exact allowlist CORS credentials and the
	// refresh-endpoint Origin check are ever satisfied by
	// (auth-csrf-origin: Origin Validation on Refresh).
	AllowedOrigins []string

	ResetRequester password.ResetRequester
	TokenIssuer    password.TokenIssuer
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
