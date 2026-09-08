-- InsertAuditLog is the audit_log hash chain's single writer (D-O): the
-- Semgrep rule single-writer-audit-log.yml (api/.semgrep/) fails the
-- build if any generated call site other than
-- internal/domain/audit.Append invokes it. Do not call this query
-- directly from anywhere else.
-- name: InsertAuditLog :one
INSERT INTO audit_log (id, user_id, community_id, action, entity, entity_id, before, after, ip, request_id, prev_hash, hash, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: GetAuditLogHead :one
SELECT hash, created_at, id FROM audit_log ORDER BY created_at DESC, id DESC LIMIT 1;

-- name: ListAuditLogRange :many
SELECT * FROM audit_log WHERE created_at >= $1 AND created_at <= $2 ORDER BY created_at ASC, id ASC;
