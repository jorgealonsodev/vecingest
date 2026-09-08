package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jorgealonsodev/vecingest/internal/config"
	"github.com/jorgealonsodev/vecingest/internal/config/secrets"
	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/lockout"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/password"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/session"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/token"
	"github.com/jorgealonsodev/vecingest/internal/health"
	httpapi "github.com/jorgealonsodev/vecingest/internal/http/api"
	"github.com/jorgealonsodev/vecingest/internal/http/handlers"
	"github.com/jorgealonsodev/vecingest/internal/http/router"
	"github.com/jorgealonsodev/vecingest/internal/mail"
	"github.com/jorgealonsodev/vecingest/internal/platform/attempts"
	"github.com/jorgealonsodev/vecingest/internal/platform/cache"
	"github.com/jorgealonsodev/vecingest/internal/platform/hibp"
)

// mailCloseTimeout bounds how long serve's graceful shutdown waits for
// the mailer's bounded worker pool to drain (design D-N: "drained on
// graceful shutdown"). Kept well under serveShutdownGrace so a stuck
// mail drain can never eat the whole shutdown budget the HTTP server
// itself needs.
const mailCloseTimeout = 3 * time.Second

// serve's own hardening timeouts (design D-S task 6.9: "applies
// read/write/idle timeouts"). No spec scenario pins exact values; these
// are conservative, documented implementer defaults, not load-bearing
// PRD numbers.
const (
	serveReadTimeout  = 10 * time.Second
	serveWriteTimeout = 15 * time.Second
	serveIdleTimeout  = 60 * time.Second
	// serveShutdownGrace is comfortably under Docker Compose's own
	// default stop_grace_period (10s) when the api service does not
	// override it (only worker's compose entry pins an explicit 30s).
	serveShutdownGrace = 8 * time.Second
)

// runServe implements `vecingest serve` (platform-bootstrap: CLI
// Subcommands; design D-H/D-S). It assembles the exact same
// internal/http/api router cmd/openapi-gen generates its spec from,
// starts the revocation LISTEN loop, optionally runs migrations first
// under their advisory lock (`--migrate`), binds a real listener with
// read/write/idle timeouts, and shuts down gracefully when ctx is
// cancelled (main.go wires the real SIGTERM handler).
func runServe(ctx context.Context, args []string, stdout io.Writer, lookup config.LookupEnv) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	migrateFirst := fs.Bool("migrate", false, "run pending migrations under their advisory lock before listening")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cmd := config.CommandServe
	if *migrateFirst {
		cmd = config.CommandMigrate
	}
	cfg, holder, err := config.Load(ctx, lookup, cmd)
	if err != nil {
		return err
	}

	if *migrateFirst {
		if err := runMigrationsWithHolder(ctx, stdout, holder); err != nil {
			return fmt.Errorf("serve --migrate: %w", err)
		}
	}

	handlesDB, err := db.NewServeHandles(ctx, holder.DatabaseURL(), holder.DatabaseURLRead())
	if err != nil {
		return fmt.Errorf("serve: build database handles: %w", err)
	}
	defer handlesDB.Close()

	deps, realMailer, err := buildServeDeps(cfg, holder, handlesDB)
	if err != nil {
		return fmt.Errorf("serve: build handler dependencies: %w", err)
	}

	registry := health.NewRegistry(health.PostgresCheck{DB: handlesDB.Write})

	r, _, err := httpapi.New(httpapi.Config{
		Router: router.Config{
			AppEnv:      cfg.AppEnv,
			ProxyIP:     cfg.ProxyIP,
			CorsOrigins: cfg.CorsOrigins,
		},
		Deps:     deps,
		Registry: registry,
		Title:    "Vecingest API",
		Version:  "0.1.0",
	})
	if err != nil {
		return fmt.Errorf("serve: build API: %w", err)
	}

	listenCtx, stopListen := context.WithCancel(ctx)
	defer stopListen()
	go deps.RevocationCache.Listen(listenCtx, func(connectCtx context.Context) (*pgx.Conn, error) {
		return pgx.Connect(connectCtx, holder.DatabaseURL())
	}, slog.Default())

	srv := &http.Server{
		Handler:      r,
		ReadTimeout:  serveReadTimeout,
		WriteTimeout: serveWriteTimeout,
		IdleTimeout:  serveIdleTimeout,
	}

	ln, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		return fmt.Errorf("serve: listen on port %s: %w", cfg.Port, err)
	}

	serveErrCh := make(chan error, 1)
	go func() { serveErrCh <- srv.Serve(ln) }()

	// Startup log carries no secret: only the bound address, the
	// non-secret AppEnv, and the build-time version stamp (D-K, package
	// main.version) (platform-bootstrap: Config Startup Validation).
	_, _ = fmt.Fprintf(stdout, "serve: listening on %s (app_env=%s, version=%s)\n", ln.Addr().String(), cfg.AppEnv, version)
	slog.InfoContext(ctx, "serve: listening", "addr", ln.Addr().String(), "app_env", cfg.AppEnv, "version", version)

	select {
	case <-ctx.Done():
	case err := <-serveErrCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve: unexpected listener error: %w", err)
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), serveShutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("serve: graceful shutdown: %w", err)
	}

	// Drain the mailer's bounded worker pool (design D-N: "drained on
	// graceful shutdown"), on its own bounded timeout so a stuck SMTP
	// server can delay shutdown by at most mailCloseTimeout, never
	// indefinitely, and never fails the shutdown that already
	// succeeded for the HTTP server itself.
	mailCloseCtx, mailCancel := context.WithTimeout(context.Background(), mailCloseTimeout)
	defer mailCancel()
	if err := realMailer.Close(mailCloseCtx); err != nil {
		slog.WarnContext(ctx, "serve: mailer did not drain within the shutdown grace period", "error", err)
	}

	_, _ = fmt.Fprintln(stdout, "serve: stopped")
	return nil
}

// buildServeDeps assembles the real, production handlers.Deps: the same
// shape api_integration_test.go builds for tests, but wired to real
// secrets, a real HIBP client, and the real internal/mail.AsyncMailer
// SMTP sender -- SMTP_URL and MAIL_FROM are already `always`-required
// for the serve command (design D-I), so they must be used, not merely
// validated and then discarded. internal/platform/mail.LogMailer
// remains available for tests and for a deployment that deliberately
// has no SMTP, but nothing here falls back to it automatically: the
// only way to get one is to construct it explicitly in source, exactly
// as api_integration_test.go already does.
func buildServeDeps(cfg config.Config, holder *secrets.Holder, handlesDB db.Handles) (*handlers.Deps, *mail.AsyncMailer, error) {
	accessSecret := []byte(holder.JWTSecret())
	refreshSecret := []byte(holder.JWTRefreshSecret())
	previousSecret := []byte(holder.JWTSecretPrevious())

	csrfKey, err := token.DeriveCSRFKey(refreshSecret)
	if err != nil {
		return nil, nil, fmt.Errorf("derive CSRF key: %w", err)
	}

	var mfaKey [32]byte
	copy(mfaKey[:], holder.EncryptionKey())

	realMailer, err := mail.New(mail.Config{
		SMTPURL: holder.SMTPURL(),
		From:    cfg.MailFrom,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("build mailer: %w", err)
	}

	deps := &handlers.Deps{
		DB:           handlesDB,
		AccessIssuer: token.Issuer{Secret: accessSecret, PreviousSecret: previousSecret},
		CSRFKey:      csrfKey,
		Rotator:      session.Rotator{},
		Lockout: lockout.Service{
			Counter: attempts.NewCounter(nil),
			Mailer:  handlers.LockoutMailer{Sender: realMailer},
		},
		PasswordPolicy:  password.PasswordPolicy{HIBP: hibp.NewClient()},
		MFAKey:          mfaKey,
		MFACounter:      attempts.NewCounter(nil),
		RevocationCache: cache.New(cache.SessionsFamilyChecker{DB: handlesDB.Write}, token.AccessTTL, nil),
		// CookieDomain: api.DOMAIN (design D-E), never DOMAIN itself --
		// the refresh cookie must never be readable from app.DOMAIN.
		CookieDomain:   "api." + cfg.Domain,
		AllowedOrigins: cfg.CorsOrigins,
		ResetRequester: handlers.DBResetRequester{DB: handlesDB.Write, Sender: realMailer},
		TokenIssuer:    handlers.OpaqueTokenIssuer{},
	}
	return deps, realMailer, nil
}
