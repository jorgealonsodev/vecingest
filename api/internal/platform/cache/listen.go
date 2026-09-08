package cache

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// listenRetryDelay is the backoff between reconnect attempts of the
// LISTEN loop.
const listenRetryDelay = 2 * time.Second

// Listen subscribes to RevokedChannel using a dedicated connection
// (LISTEN/NOTIFY requires a persistent connection, never a pooled one)
// and marks c accordingly: healthy while connected, down (starting the
// fail-closed fallback window) on any error, with an automatic
// reconnect after listenRetryDelay. It blocks until ctx is cancelled, so
// callers run it in its own goroutine (cmd/vecingest serve, once that
// subcommand exists).
func (c *RevocationCache) Listen(ctx context.Context, connect func(ctx context.Context) (*pgx.Conn, error), logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	for {
		if ctx.Err() != nil {
			return
		}
		if err := c.listenOnce(ctx, connect, logger); err != nil {
			c.MarkListenDown()
			logger.WarnContext(ctx, "cache: LISTEN session_revoked connection lost, falling back to indexed lookup", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(listenRetryDelay):
		}
	}
}

func (c *RevocationCache) listenOnce(ctx context.Context, connect func(ctx context.Context) (*pgx.Conn, error), logger *slog.Logger) error {
	conn, err := connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, "LISTEN "+RevokedChannel); err != nil {
		return err
	}
	c.MarkListenHealthy()
	logger.InfoContext(ctx, "cache: LISTEN session_revoked established")

	for {
		notif, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		familyID, err := uuid.Parse(notif.Payload)
		if err != nil {
			logger.WarnContext(ctx, "cache: malformed session_revoked payload", "payload", notif.Payload)
			continue
		}
		c.MarkRevoked(familyID)
	}
}
