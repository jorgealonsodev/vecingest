-- invitations: tenant column community_id (design D-5).
-- name: InsertInvitation :one
INSERT INTO invitations (id, community_id, unit_id, email, role, token_hash, short_code_hash, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- GetInvitationByID is tenant-scoped by both id and community_id: the
-- caller's community membership was already resolved and role-checked
-- by scoped.Community/scoped.Invitation before this query ever runs,
-- but the explicit community_id predicate is the same defense-in-depth
-- unit_members.sql's three-bound queries already establish.
-- name: GetInvitationByID :one
SELECT * FROM invitations WHERE id = $1 AND community_id = $2;

-- Invitation resolver (design D-4): a {invitationId} route resolves
-- community membership via the invitation's own owning community_id,
-- never a caller-supplied value. Returns the community_id regardless of
-- status: a revoked/accepted/blocked invitation is still tied to a real
-- community for cross-tenant isolation purposes.
-- name: GetInvitationCommunityID :one
SELECT community_id FROM invitations WHERE id = $1;

-- name: ListInvitationsByCommunityID :many
SELECT * FROM invitations WHERE community_id = $1 ORDER BY created_at DESC;

-- GetInvitationByTokenHash and GetInvitationByShortCodeHash back the
-- unauthenticated preview/accept flow (invitations spec: "Preview
-- Endpoint Is POST"; "Accept Creates Or Links An Account"). Both are
-- global-uniqueness lookups by design (token_hash/short_code_hash are
-- each UNIQUE across every community), so neither takes a tenant
-- parameter -- the caller has no community context yet at this point in
-- the flow, which is exactly why the invitation row itself is what
-- supplies it (authz.ResolveCommunityViaInvitation, used by the
-- AUTHENTICATED resend/revoke routes, is a completely separate path).
-- name: GetInvitationByTokenHash :one
SELECT id, community_id, unit_id, email, role, token_hash, short_code_hash, status, expires_at,
    accepted_at, sent_count, failed_attempts, created_at, updated_at
FROM invitations WHERE token_hash = $1;

-- name: GetInvitationByShortCodeHash :one
SELECT id, community_id, unit_id, email, role, token_hash, short_code_hash, status, expires_at,
    accepted_at, sent_count, failed_attempts, created_at, updated_at
FROM invitations WHERE short_code_hash = $1;

-- AcceptInvitation implements design D-6's single-use write: the status
-- transition only succeeds when the invitation is still pending and
-- unexpired, so a concurrent or repeated accept observes zero rows
-- rather than racing a prior read.
-- name: AcceptInvitation :one
UPDATE invitations SET status = 'accepted', accepted_at = now(), updated_at = now()
WHERE id = $1 AND community_id = $2 AND status = 'pending' AND expires_at > now()
RETURNING id;

-- RevokeInvitation implements DELETE /v1/invitations/:id (invitations
-- spec: "Resend And Revoke"). Only a still-pending invitation can be
-- revoked; zero rows returned means it was already accepted/revoked/
-- blocked, which the handler maps to a 409 rather than silently
-- reporting success.
-- name: RevokeInvitation :one
UPDATE invitations SET status = 'revoked', updated_at = now()
WHERE id = $1 AND community_id = $2 AND status = 'pending'
RETURNING id;

-- RotateInvitationShortCode backs POST /v1/invitations/:id/resend. It
-- re-issues the short code and counts the send in ONE statement, because the
-- two must never diverge: sent_count is the operator-visible evidence that a
-- delivery happened, and it used to be incremented by a resend that dispatched
-- nothing at all (R3-resend-invitation-dispatches-nothing, review lineage
-- review-f855997b550a986d).
--
-- Re-issuing rather than redelivering is forced by the storage model and
-- chosen deliberately: only one-way digests of the code (an HMAC) and the
-- token are persisted, so the plaintext an invitation email renders is
-- unrecoverable here. See the handler for why a recoverable copy was rejected.
--
-- Pending-only and tenant-scoped, exactly like RevokeInvitation: zero rows
-- means the invitation is no longer pending, which the handler maps to 409
-- rather than silently reporting a send. email is returned because the job
-- payload needs it and the caller has only the invitation id in hand.
-- name: RotateInvitationShortCode :one
UPDATE invitations SET short_code_hash = $3, sent_count = sent_count + 1, updated_at = now()
WHERE id = $1 AND community_id = $2 AND status = 'pending'
RETURNING id, email, sent_count;

-- IncrementInvitationFailedAttempts records that a preview/accept call
-- RESOLVED to this real invitation row (design D-6: "invitations.
-- failed_attempts is still incremented, but only when the code resolved
-- to a real invitation, and it is gate evidence rather than the
-- mechanism"). It is called unconditionally on resolution, independent
-- of whether the call goes on to succeed -- the enumeration lockout
-- itself is enforced entirely by the IP+device AttemptCounter, never by
-- this column (invitations spec: "failed_attempts alone does not
-- enforce the lockout"). community_id is bound from the JUST-RESOLVED
-- row (the caller always has it in hand at this point), so this is a
-- genuine tenant filter, not merely a textual one.
-- name: IncrementInvitationFailedAttempts :exec
UPDATE invitations SET failed_attempts = failed_attempts + 1, updated_at = now() WHERE id = $1 AND community_id = $2;

-- SweepExpiredInvitations implements the daily invitations.expire job
-- (design D-6; PRD §7.4 job table; task 6.18/6.19). It transitions
-- past-expiry pending rows to 'blocked' -- the only CHECK-permitted
-- terminal value left once 'accepted' and 'revoked' are excluded, since
-- the invitations_status_check constraint (00008_invitations.sql) does
-- not include an 'expired' value: expiry is reported at READ time via
-- derivation (status='pending' AND expires_at<=now() reads as
-- "expired"), and this sweep is the bookkeeping step that makes the
-- stored column converge for reporting once the derivation window has
-- passed, matching design D-6's "expired is derived at read time and
-- never stored, and the daily invitations.expire job sweeps."
-- name: SweepExpiredInvitations :execrows
UPDATE invitations SET status = 'blocked', updated_at = now()
WHERE status = 'pending' AND expires_at <= now();
