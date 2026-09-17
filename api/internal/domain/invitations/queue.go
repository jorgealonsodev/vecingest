// Package invitations holds the M1 invitation domain ports: the
// background-job seam touched for the first time by a real producer
// (design's Interfaces/Contracts table, "Queue -- yes -- first
// producers in the project") and the short-code generator (design D-6).
// Keeping these here, not in internal/http/handlers or
// internal/platform/queue, is what lets handlers.Deps depend on an
// interface without importing river directly, and lets
// internal/platform/queue provide the concrete phase-A implementation
// without importing the HTTP layer (design: "No M1 handler or domain
// service imports ... river. Each depends on the interface through a
// field on handlers.Deps, and the concrete types are named in exactly
// one place -- buildServeDeps").
package invitations

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// EmailArgs is the invitation-email background job's payload (design
// D-6: "river.InsertTx for the invitation email job inside the creation
// transaction"). ShortCode is the plaintext, one-time-visible secret:
// it lives only long enough to render and dispatch the email -- it is
// never written back to the invitations table (design D-6: "MUST
// persist only token_hash/short_code_hash").
//
// This struct is an IN-MEMORY port value and never reaches a durable
// row as it stands: internal/platform/queue seals the short code under
// ENCRYPTION_KEY before river.InsertTx and the worker opens it at send
// time. An earlier revision let the plaintext through into river_job,
// where every pending and retained-completed row held a directly usable
// credential for anyone with SELECT on that table, a backup or a read
// replica -- exactly the guarantee short_code_hash exists to provide
// (review lineage review-e72754dc7521b57a). Do not reintroduce a
// producer that persists this field verbatim.
type EmailArgs struct {
	InvitationID uuid.UUID
	Email        string
	ShortCode    string
}

// Queue is the phase-A/phase-B background-job seam (design's
// Interfaces/Contracts table). Phase A's concrete implementation
// (internal/platform/queue) wraps river.Client.InsertTx so the job is
// enqueued in the SAME transaction that creates the invitation row --
// if the transaction rolls back, the job is never worked (River's own
// snapshot-visibility guarantee). Phase B swaps the driver with no
// change to this interface or any caller.
type Queue interface {
	EnqueueInvitationEmail(ctx context.Context, tx pgx.Tx, args EmailArgs) error
}
