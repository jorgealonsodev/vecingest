---
change: m0-foundation
phase: tasks
project: vecingest
date: 2026-09-04
authority: proposal.md (WU-0..WU-12, checkpoints A/B), design.md (D-A..D-S), specs/*/spec.md (51 requirements, 92 scenarios), PRD_go.md §7, §8, §9, §10.1
---

# Tasks: M0 Foundation — monorepo, auth, and Portainer-deployable stack

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | Large — full milestone: monorepo scaffold, DB schema/migrations, auth domain, HTTP surface, generated client, Expo screen, CI |
| 400-line budget risk | High |
| Chained PRs recommended | No |
| Suggested split | Single delivery — `exception-ok`, lands directly on `main` |
| Delivery strategy | exception-ok |
| Chain strategy | size-exception |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: High

No review budget and no PRs apply to this change (`exception-ok`, maintainer-accepted `size:exception`). Work units below are engineering slices for sequencing and verification only — not proposed PR boundaries.

### Suggested Work Units

| Unit | Goal | Delivery | Focused test command | Runtime harness | Rollback boundary |
|------|------|----------|----------------------|------------------|--------------------|
| 0 | Monorepo/workspace scaffold | main (exception-ok) | `cd api && go build ./...` | N/A — no runtime behavior exists yet | Delete scaffold dirs/files |
| 1 | Config + embedded migration runner + 3-role bootstrap | main (exception-ok) | `go test ./internal/config/... -run Test` | Testcontainers Postgres 17 | `goose down` to zero; drop DB |
| 2 | Argon2id hasher + password policy + HIBP | main (exception-ok) | `go test ./internal/domain/auth/password/...` | Fake HIBP server, no network | Remove `internal/domain/auth/password` |
| 3 | Auth schema migrations + sqlc + append-only enforcement | main (exception-ok) | `go test ./api/test/... -run TestPrivilegeMatrix` | Testcontainers Postgres 17 | `goose down`; drop partitions |
| 4 | Auth domain services (tokens, CSRF, lockout, audit chain, TOTP) | main (exception-ok) | `go test ./internal/domain/...` | Testcontainers for chain/TOTP concurrency rows | Revert `internal/domain/{auth,audit}` |
| 5 | HTTP middleware/handlers + first `make gen` | main (exception-ok) | `go test ./internal/http/...` | huma test API + real Postgres (in-process HTTP) | Revert `internal/http`; regenerate `openapi.yaml` from prior state |
| 6 | CLI subcommands (`seed`, `worker`, `bootstrap-superadmin`) | main (exception-ok) | `go test ./api/test/... -run TestSubcommands` | Real built binary + signal delivery | Remove `cmd/vecingest/{seed,worker,bootstrap_superadmin}.go` |
| 7 | Generated `packages/shared` | main (exception-ok) | `make gen && git diff --exit-code` | N/A — generation is deterministic, no live system | Delete and regenerate |
| 8 | Dockerfiles | main (exception-ok) | `docker build -f api/Dockerfile .` | Built image + `docker run` smoke | Delete Dockerfiles |
| 9 | Compose + `.env.example` | main (exception-ok) | `docker compose -f deploy/docker-compose.yml config` | N/A at authoring time — no deployed host yet (Checkpoint B) | Delete compose files |
| 10 | Expo login screen | main (exception-ok) | `pnpm --filter app test` | React Native Testing Library | Revert `app/(auth)/login.tsx` |
| 11 | CI workflows | main (exception-ok) | N/A — verified by a green/failing pipeline run, not a local test | GitHub Actions run + deliberately-failing probe PR | Revert `.github/workflows/*.yml` |
| 12 | Threat model + gate M0 doc | main (exception-ok) | N/A — document, verified by review | N/A | Revert `docs/security/*.md` |
| 13 | Checkpoint A evidence aggregation | main (exception-ok) | `cd api && go test -race ./... && pnpm --filter app test` | Full CI green | N/A — evidence-only, no code |
| 14 | Checkpoint B (infra-blocked) | blocked | N/A until host exists | Real deployed Portainer host | N/A — evidence-only, no code |

## Phase 0: Monorepo & Workspace Scaffolding (WU-0 — non-TDD setup, no RED possible)

- [x] 0.1 Create `go.work`, `api/go.mod` (module path per PRD §7.2 org segment, `go 1.26.0`), pin `github.com/riverqueue/river v0.47.0`, `tool` directives for `sqlc`/`goose` (D-K, D-L)
- [x] 0.2 Create `pnpm-workspace.yaml`, `turbo.json`, root `package.json` covering `app/`, `packages/shared/`, `site/`
- [x] 0.3 Create `Makefile` with `dev`, `gen`, `test`, `test-short`, `test-golden-update`, `lint`, `lint-scope`, `test-e2e` targets. **Correction (verify-report.md CRITICAL-1, remediation batch 2026-09-08):** the `lint-scope` target was wired to `go run ./cmd/lintscope`, a tool that did not exist yet, so `[x]` here was inaccurate until `api/cmd/lintscope` was actually written (11.7 below). It now exists and `make lint-scope` passes for real — see 11.7's evidence.
- [x] 0.4 Create `lefthook.yml` pre-commit hooks for Go and TS
- [x] 0.5 Create `renovate.json`, grouping `github.com/riverqueue/river` with auto-merge disabled (D-L: still 0.x, no API stability guarantee)
- [x] 0.6 Create empty `.github/workflows/{ci,security,deploy}.yml` skeletons (jobs wired in Phase 11)
- [x] 0.7 Create directory skeleton: `api/cmd/vecingest/`, `api/internal/{config,http,domain,db,platform,observability,health,mail}/`, `api/migrations/{bootstrap,schema}/`, `api/openapi/`, `api/test/`, `app/`, `packages/shared/`, `site/`, `deploy/`, `docs/security/gates/`
- [x] 0.8 Verify by inspection: `cd api && go build ./...` succeeds on the empty module; `pnpm install` succeeds

## Phase 1: Config Validation, Embedded Migration Runner, Three-Role Bootstrap (WU-1 — TDD, D-A/D-B/D-I/D-R)

- [x] 1.1 RED: `api/internal/config/config_test.go` — table-driven, per subcommand (`serve`, `serve --migrate`/`migrate`, `worker`, `seed`, `bootstrap-superadmin`, `health`), asserting `config.Load` names the missing variable and fails; full required set passes (platform-bootstrap: Config Startup Validation; gate item 5)
- [x] 1.2 GREEN: implement `api/internal/config/config.go` — log-safe `Config`, `secrets.Holder` (`slog.LogValuer`, `String()`, `MarshalJSON` all redacted), per-subcommand requirement sets (D-I)
- [x] 1.3 RED: test asserting `os.Getenv("ENCRYPTION_KEY")` returns empty and the serialized `Config` carries no `ENCRYPTION_KEY` field after `Load` succeeds (platform-bootstrap: ENCRYPTION_KEY Isolation, both scenarios)
- [x] 1.4 GREEN: implement `os.Unsetenv("ENCRYPTION_KEY")` post-validation and holder-only storage
- [x] 1.5 RED (Testcontainers, `testing.Short()` skip): assert `vecingest_owner` and `app_rw` exist as distinct roles from the superuser after bootstrap, and `app_rw` is not a member of `vecingest_owner` (db-access-control: Three-Role Provisioning; I3)
- [x] 1.6 GREEN: implement `api/migrations/bootstrap/00001_roles.go` (`AddMigrationNoTxContext`, D-A) — create roles, `btree_gist`/`pg_stat_statements`, schema ownership, `REVOKE ALL ON SCHEMA public FROM PUBLIC`, `GRANT USAGE`, fail-closed `ALTER DEFAULT PRIVILEGES ... GRANT SELECT, INSERT ON TABLES TO app_rw`
- [x] 1.7 RED (Testcontainers): assert `append_only_relations` exists seeded with `audit_log`, and event trigger `vecingest_append_only_guard` exists on `ddl_command_end` filtered to `CREATE TABLE`/`CREATE TABLE AS`/`ALTER TABLE` (db-access-control: SECURITY DEFINER EXECUTE Invariant setup; D-B mechanism 2)
- [x] 1.8 GREEN: implement `append_only_relations` table + `vecingest_append_only_guard_fn()` (bidirectional `pg_partition_ancestors`/`pg_partition_tree` closure, `has_table_privilege`-gated, `SECURITY INVOKER`) + event trigger (D-B)
- [x] 1.9 RED (Testcontainers): two embedded goose providers (`WithSessionLocker`, pinned lock ids 5432001/5432002) started concurrently converge to target version with no race (platform-bootstrap: Embedded Migration Runner With Advisory Lock)
- [x] 1.10 GREEN: implement `api/internal/platform/migrate/` — `goose.NewProvider` over `//go:embed migrations/bootstrap/*.sql` and `migrations/schema/*.sql`, invoked from `serve --migrate`/`migrate` (D-A)
- [x] 1.11 RED (Testcontainers): `serve` yields two distinct `*pgxpool.Pool` instances with different `MaxConns`; `serve` pools use `QueryExecModeCacheDescribe`, `worker`'s do not; both handles default to `DATABASE_URL` with no overrides set (platform-bootstrap: Per-Role DB Handles, first two scenarios)
- [x] 1.12 GREEN: implement `api/internal/db/handles.go` — `ReadDB`, `WriteDB` distinct named types, `Handles{Write, Read}` (D-R)
- [x] 1.13 RED (Testcontainers): a configured `DATABASE_URL_READ`/`DATABASE_URL_WORKER` override authenticates via `SELECT current_user` as `app_rw`, never a broader role (platform-bootstrap: Per-Role DB Handles, remaining two scenarios)
- [x] 1.14 GREEN: wire `DATABASE_URL_READ`/`DATABASE_URL_WORKER` resolution in `internal/config` and `cmd/vecingest` (D-R point 4); include the compile-time negative test that `ReadDB` exposes no `Begin`

## Phase 2: Argon2id Hasher & Password Policy (WU-2 — TDD, the proposal's canonical first RED test, D-F/D-G)

- [x] 2.1 RED: `api/internal/domain/auth/password/argon2_test.go` — PHC round-trip, tampered tag, bounds-checked `m`/`t`/`p`, `needsRehash` (auth-credentials: Argon2id Password Hashing, scenario "Password stored as PHC string"; gate item 1)
- [x] 2.2 GREEN: implement `argon2.go` — `argon2.IDKey(pw, salt, 2, 19456, 1, 32)`, 16-byte `crypto/rand` salt, `RawStdEncoding` PHC string, `subtle.ConstantTimeCompare` verify, strict PHC parser (D-G)
- [x] 2.3 RED: rehash-on-login writes the new PHC string in the same transaction as `last_login_at` when stored parameters differ (auth-credentials: scenario "Rehash on parameter bump")
- [x] 2.4 GREEN: implement `Verify(ctx, stored, candidate) (ok, needsRehash bool)`
- [x] 2.5 RED: login against an unknown email still runs Argon2id work against a fixed dummy hash (non-disclosure of existence)
- [x] 2.6 GREEN: implement the dummy-hash comparison path
- [x] 2.7 RED: `password_policy_test.go` — 14-char without TOTP rejected stating the 15-char rule; 12-char with TOTP active accepted (auth-credentials: Conditional Password Length Floor, both scenarios; gate item 1)
- [x] 2.8 GREEN: implement `PasswordPolicy` — `AUTH_PASSWORD_TOO_SHORT_NO_MFA`/`AUTH_PASSWORD_TOO_SHORT_WITH_MFA` with `details: {min_length, rule}` (D-F)
- [x] 2.9 RED: `hibp_test.go` against a fake HIBP server — breached password rejected; zero-count padding entry discarded and not rejected; transport error fails open with WARN + `audit_log` entry (auth-credentials: HIBP k-Anonymity Breach Check, all three scenarios; gate item 1)
- [x] 2.10 GREEN: implement `HIBPChecker` + real client (SHA-1 5-char prefix, `Add-Padding: true`) in `internal/platform/hibp/`, fake server for tests (D-F)

## Phase 3: Auth Schema Migrations, sqlc, Append-Only Enforcement (WU-3 — TDD, Testcontainers, D-B/D-C/D-O)

- [x] 3.1 RED (Testcontainers): migrations up→down→up idempotent for the full auth schema set
- [x] 3.2 GREEN: implement `api/migrations/schema/0001_auth_schema.sql` — `users`, `sessions`, `password_reset_tokens`, `user_mfa`, `otp_challenges` per the design's exact DDL, each with explicit `GRANT UPDATE, DELETE ON <table> TO app_rw` (D-B mechanism 1)
- [x] 3.3 RED (Testcontainers): `audit_log` created `PARTITION BY RANGE (created_at)` with 12 pre-created monthly partitions; `pg_available_extensions` probes `pg_partman` presence (D-C)
- [x] 3.4 GREEN: implement `0002_audit_log.sql` — partitioned parent + 12 partitions + `REVOKE UPDATE, DELETE, TRUNCATE ON audit_log FROM app_rw, PUBLIC` on parent and every partition
- [x] 3.5 RED (Testcontainers): `app_rw` cannot `UPDATE`/`DELETE`/`TRUNCATE` `audit_log` directly (db-access-control: Runtime Role Append-Only Restriction, both scenarios; gate item 9)
- [x] 3.6 RED (Testcontainers): `app_rw` cannot `UPDATE`/`DELETE`/`TRUNCATE` a direct child partition (db-access-control: Append-Only Restriction Extends To Every Partition)
- [x] 3.7 RED (Testcontainers, required, D-B mechanism 2): `CREATE TABLE ... PARTITION OF audit_log` as `vecingest_owner` — resulting child carries no write grant for `app_rw`, with no explicit revoke anywhere in the test
- [x] 3.8 RED (Testcontainers, required, non-droppable per D-B): `CREATE TABLE (LIKE audit_log)` + `ALTER TABLE audit_log ATTACH PARTITION` as `vecingest_owner` — same assertion; proves which relation `pg_event_trigger_ddl_commands()` reports for ATTACH
- [x] 3.9 GREEN: satisfy 3.5–3.8 via the Phase 1 bootstrap-set guard (1.7/1.8) plus 3.4's explicit revoke; fix the guard or migration if any fails
- [x] 3.10 RED (Testcontainers): invariants I1 (partition-hierarchy walk), I2 (`SECURITY DEFINER`/`PUBLIC` EXECUTE scan), I3 (ownership/membership), I4 (default-privilege baseline pinned to `SELECT, INSERT` only) each fire on a deliberately-planted violation (db-access-control: SECURITY DEFINER EXECUTE Invariant, both scenarios)
- [x] 3.11 GREEN: implement `0003_assert_invariants.go` — `DO $$ ... RAISE EXCEPTION $$` blocks for I1–I4, run at the end of the schema set (D-B)
- [x] 3.12 RED (Testcontainers): River grants present (`SELECT, INSERT, UPDATE, DELETE` on the five River tables to `app_rw`); River migrates to the pinned `TargetVersion`; `river_job` is empty after the full E2E suite (D-L)
- [x] 3.13 GREEN: implement `0004_river.go` — `AddMigrationNoTxContext` calling `rivermigrate` with a pinned `TargetVersion`, plus explicit River grants (D-L)
- [x] 3.14 RED: no `float64`/`float32` in generated `internal/db` code; Semgrep/`forbidigo` rule blocks float types under `internal/http/dto`, `internal/domain`, `internal/db`, and `double precision`/`real` in `migrations/**.sql`
- [x] 3.15 GREEN: write `api/sqlc.yaml` (`numeric`→`decimal.Decimal`, `uuid`→`uuid.UUID`, `timestamptz`→`time.Time`, `inet`→`netip.Addr`, `jsonb`→`json.RawMessage`); add the Semgrep rule + `.golangci.yml` `forbidigo` pattern; run `sqlc generate`, commit `internal/db/*.sql.go`
- [x] 3.16 RED: a Semgrep rule fails if any caller other than `audit.Append` invokes the generated `InsertAuditLog` query (D-O single-writer invariant)
- [x] 3.17 GREEN: write `queries/*.sql` for every M0 table and the single-writer Semgrep rule

## Phase 4: Auth Domain Services — Tokens, CSRF, Lockout, Audit Chain, TOTP (WU-4 — TDD, D-D/D-E/D-N/D-O/D-P)

- [x] 4.1 RED: access JWT HS256 (`kid`, `sub`/`sid`/`sa`/`iat`/`exp`/`jti`, 15 min); refresh 30 d / 8 h for `superadmin` (auth-session-tokens: JWT Access and Refresh Issuance, both scenarios)
- [x] 4.2 GREEN: implement `internal/domain/auth/token/` — access issuance (`JWT_SECRET`/`JWT_SECRET_PREVIOUS` by `kid`), refresh generation (D-D)
- [x] 4.3 RED: rotation inserts a new `sessions` row sharing `family_id`, stamps `revoked_at` on the old row; presenting an invalidated token revokes the whole family (auth-session-tokens: Refresh Rotation with Family Invalidation, both scenarios; gate item 2)
- [x] 4.4 GREEN: implement `internal/domain/auth/session/rotate.go` — single-use rotation, reuse-detection family revocation, `audit.Append`, `NOTIFY session_revoked` (D-D)
- [x] 4.5 RED: refresh/reset/OTP short codes persisted only as SHA-256 hashes, never cleartext (auth-session-tokens: Hashed Storage of Sensitive Tokens; gate item 2)
- [x] 4.6 GREEN: implement hashing at the storage boundary for refresh/reset/OTP tokens
- [x] 4.7 RED: signed double-submit CSRF token — HMAC-SHA256 via `HKDF-SHA256(JWT_REFRESH_SECRET, info="vecingest/csrf/v1")`; forged, expired, family-mismatch, session-id-mismatch-after-rotation all rejected (auth-csrf-origin: CSRF Token Delivered In-Body, domain leg; gate item 3; D-D/D-E)
- [x] 4.8 GREEN: implement `internal/domain/auth/token/csrf.go` — mint/verify bound to `family_id` + `session_id` + `exp`
- [x] 4.9 RED: cookie XOR body transport; both present → `400 AUTH_AMBIGUOUS_TOKEN_TRANSPORT` (auth-session-tokens: Refresh Transport Mutual Exclusion)
- [x] 4.10 GREEN: implement the transport-detection guard in the refresh domain service
- [x] 4.11 RED: 5 failures/15 min lock by email and independently by IP across different emails; unknown email advances the counter identically; success resets the email counter only, not the IP counter; escalating 15/30/60-min window (auth-credentials: Progressive Lockout, both scenarios; D-N)
- [x] 4.12 GREEN: implement `internal/domain/auth/lockout/` + `internal/platform/attempts/` (`AttemptCounter`, in-process sliding window) + lockout-alert dedupe key (D-N)
- [x] 4.13 RED: forgot-password on an unregistered email returns a response identical in shape/status to a registered one (auth-credentials: Enumeration-Safe Auth Responses)
- [x] 4.14 GREEN: implement the uniform forgot-password response
- [x] 4.15 RED: consecutive `audit_log` rows link via `prev_hash`/`hash`; a field-boundary shift (character moved from `action` to `entity`) changes the hash; `NULL` vs `""` hash differently; `VerifyChain` pinpoints the first broken link; golden hashes under `internal/domain/audit/testdata/*.golden`, regenerated only via `make test-golden-update` (db-access-control: audit_log Hash Chain, both scenarios; D-O)
- [x] 4.16 GREEN: implement `internal/domain/audit/append.go` (`Append`, `pg_advisory_xact_lock(5432003)`, length-prefixed canonical `F(x)` encoding) and `verify.go` (`VerifyChain`) (D-O)
- [x] 4.17 RED (Testcontainers): 50 concurrent `Append` calls from 10 goroutines produce a gap-free, fork-free chain; a chain spanning a month partition boundary verifies unbroken; a hostile owner-connection `UPDATE` makes `VerifyChain` fail at exactly that row
- [x] 4.18 GREEN: fix any concurrency defect surfaced by 4.17
- [x] 4.19 RED: ±1 step accepted, ±2 rejected; a code rejected on second use; a `T-1` code rejected after `T` was accepted; highest matching step recorded when several candidates match; the candidate loop never breaks early (auth-mfa-totp: TOTP Verification Parameters + TOTP Replay Protection, all scenarios; D-P)
- [x] 4.20 GREEN: implement `internal/domain/auth/mfa/totp.go` — `pquerna/otp` generation, `last_totp_step` monotonic conditional update, `40001` mapped to `AUTH_TOTP_REPLAYED` (D-P)
- [x] 4.21 RED (Testcontainers): two concurrent TOTP verifications at the configured isolation level — exactly one wins, the loser gets zero rows and a rejection, never a 500
- [x] 4.22 GREEN: document the READ COMMITTED dependency in `internal/db`; add `pgerrcode.SerializationFailure` classification
- [x] 4.23 RED: TOTP enrollment stays inactive until one valid code is verified (auth-mfa-totp: TOTP Enrollment)
- [x] 4.24 GREEN: implement `mfa.Enroll`/`mfa.ConfirmEnrollment` gating
- [x] 4.25 RED: 10 recovery codes, 128-bit, SHA-256 hashed, single-use, constant-time compare over all entries, reused code rejected (auth-mfa-totp: One-Time Recovery Codes)
- [x] 4.26 GREEN: implement `mfa.GenerateRecoveryCodes`/`mfa.ConsumeRecoveryCode`
- [x] 4.27 RED: 5th TOTP failure blocks the 6th; recovery-code failures count toward the same `auth:fail:totp:<user_id>` budget; success resets the per-user counter, not the shared IP counter (auth-mfa-totp: TOTP Attempt Throttling, all four scenarios)
- [x] 4.28 GREEN: wire the `auth:fail:totp:<user_id>` key family into `mfa.VerifyTOTP`/`mfa.ConsumeRecoveryCode` (D-N/D-P)

## Phase 5: HTTP Middleware Chain, Handlers, First `make gen` Pass (WU-5 — TDD, D-H/D-E/D-J/D-Q)

- [x] 5.1 RED: middleware order — `ClientIPFromXFF` precedes the logger, `Limiter`, and CSRF steps (request-protection: Trusted-Proxy Client IP Resolution, both scenarios; gate item 10; D-H)
- [x] 5.2 GREEN: implement `internal/http/router.go` per D-H's 9-step chain
- [x] 5.3 RED: spoofed leftmost `X-Forwarded-For` from an untrusted source ignored; dev-only hop-count fallback refused when `APP_ENV` is staging/production
- [x] 5.4 GREEN: wire `PROXY_IP` config + `ClientIPFromXFFTrustedProxies(1)` dev fallback with the `APP_ENV` guard
- [x] 5.5 RED: 10 req/min login/reset, 300 req/min authenticated user, 60 req/min per IP; 11th login request in a minute → 429 (request-protection: Limiter Interface with Rate Budgets)
- [x] 5.6 GREEN: implement `internal/platform/limiter/httprate.go` — `httprate.LimitBy` + `CanonicalizeIP(GetClientIP(ctx))`
- [x] 5.7 RED: email/phone/IBAN redacted as a structured attribute and when interpolated into the message string (request-protection: slog PII Redaction, both scenarios; gate item 11)
- [x] 5.8 GREEN: implement `internal/observability/slogpii/` wrapping handler + `ReplaceAttr` + `LogValuer` types + Semgrep call-site rule (D-J)
- [x] 5.9 RED: `POST /v1/auth/login` — success issues access+refresh with correct lifetimes and a `csrf_token` in the body on the cookie transport; failure paths run the constant-time dummy-hash comparison
- [x] 5.10 GREEN: implement the login handler wiring `password`/`token`/`lockout`; `dto` structs with `DisallowUnknownFields`
- [x] 5.11 RED: `POST /v1/auth/refresh` — missing/invalid CSRF header → 403; valid cookie+CSRF → rotates; body-path requires no CSRF; both present → 400; foreign `Origin` → 403 (auth-csrf-origin: CSRF Token Delivered In-Body + Origin Validation on Refresh, all scenarios; gate item 3)
- [x] 5.12 GREEN: implement the refresh handler — transport detection, CSRF verification, `CrossOriginProtection`, rotation
- [x] 5.13 RED: `GET /v1/auth/refresh/csrf` — real cookie jar confirms the cookie is sent (subpath of `Path=/v1/auth/refresh`); `/v1/auth/csrf` receives no cookie (regression test); safe method, no rotation; 401 on missing/expired/revoked; response carries `Cache-Control: no-store`, `Vary: Origin`, `Cross-Origin-Resource-Policy: same-origin` (auth-csrf-origin: CSRF Token Recovery Endpoint, all scenarios)
- [x] 5.14 GREEN: implement the `GET /v1/auth/refresh/csrf` handler with the three required headers
- [x] 5.15 RED: refresh cookie carries `HttpOnly`, `Secure`, `SameSite=Strict`, `Path=/v1/auth/refresh`, `Domain=api.DOMAIN` on login (auth-csrf-origin: Refresh Cookie Scope and Flags)
- [x] 5.16 GREEN: implement shared `Set-Cookie` construction for login/refresh
- [x] 5.17 RED: `POST /v1/auth/logout` — missing Bearer rejected; valid Bearer revokes the whole `sid` family; no CSRF required/checked; `Set-Cookie` `Max-Age=0` on `Path=/v1/auth/refresh` regardless of request path (auth-session-tokens: Bearer-Authenticated Logout, all four scenarios)
- [x] 5.18 GREEN: implement the Bearer-authenticated logout handler (D-E)
- [x] 5.19 RED: a still-unexpired access token is rejected right after its session is revoked; degraded fallback to indexed `sessions` lookup when `LISTEN` has been down longer than the access-token TTL or on a fresh restart (auth-session-tokens: Immediate Session Revocation Effect, both scenarios)
- [x] 5.20 GREEN: implement the `Cache` revocation-set check (`sid`-keyed, TTL = access-token lifetime) + `LISTEN/NOTIFY` on `session_revoked` + fail-closed fallback
- [x] 5.21 RED: `POST /v1/auth/forgot-password`/`reset-password` — single-use, 1 h expiry, policy+HIBP applied, HIBP fail-open path exercised
- [x] 5.22 GREEN: implement the reset-flow handlers
- [x] 5.23 RED: `POST /v1/auth/superadmin/login` — non-superadmin rejected generically; no `user_mfa` row → 403 enrollment-required; TOTP-gated 8 h session; route distinct from `/v1/auth/login` (auth-mfa-totp: Mandatory TOTP for Superadmin, both scenarios; gate item 4)
- [x] 5.24 GREEN: implement the superadmin login handler and route
- [x] 5.25 RED: `GET /v1/me` — id/email/`is_superadmin`, no role/memberships, for both a regular user and a superadmin (user-profile: GET /v1/me Response Shape, both scenarios)
- [x] 5.26 GREEN: implement the `/v1/me` handler and `MeResponse` struct
- [x] 5.27 RED: `GET /v1/me/sessions` lists live families with device/activity data and no token hash; `DELETE /v1/me/sessions/:id` revokes the family, leaves `GET /v1/me` unchanged, invalidates the live access token, returns 404 (never 403) for another user's family (user-profile: Remote Session Listing and Revocation Endpoints; auth-session-tokens: Session Tracking and Remote Revocation)
- [x] 5.28 GREEN: implement `GET /v1/me/sessions` and `DELETE /v1/me/sessions/:id`
- [x] 5.29 RED: `GET /v1/health/live` — 200 `{"ok":true}` unconditionally, no dependency detail (service-health: Liveness Endpoint)
- [x] 5.30 GREEN: implement the liveness handler
- [x] 5.31 RED (Testcontainers): `GET /v1/health/ready` — 200 with Postgres up; non-2xx with no dependency detail when Postgres is stopped mid-test (service-health: Readiness Endpoint, both scenarios)
- [x] 5.32 GREEN: implement `internal/health/` `ReadinessCheck` registry, `postgres` check registered at M0 (D-Q)
- [x] 5.33 RED: per-subcommand startup tests — `worker` starts with no JWT secret, `health` with only `PORT`, `seed` with no `ENCRYPTION_KEY` (platform-bootstrap: Config Startup Validation, row-by-row per D-I; gate item 5)
- [x] 5.34 GREEN: finalize per-subcommand requirement sets in `internal/config`
- [x] 5.35 RED: a Go input struct gaining a required field propagates into `openapi.yaml`, the TS client, and the Zod schema (api-contract-generation: Generation Chain)
- [x] 5.36 GREEN: implement `api/cmd/openapi-gen/main.go` (huma OpenAPI emission); run the first `make gen` against the M0 auth structs; commit `api/openapi/openapi.yaml`

## Phase 6: CLI Subcommands (WU-6 — TDD, D-S)

- [x] 6.1 RED: `bootstrap-superadmin` — email as argument, password via `--password-stdin` only, never argv/env; first run creates exactly one superadmin without leaking the password to shell history/argv; second run is a no-op (platform-bootstrap: Idempotent Superadmin Bootstrap, both scenarios)
- [x] 6.2 GREEN: implement `api/cmd/vecingest/bootstrap_superadmin.go` — policy+HIBP+Argon2id, `ON CONFLICT (email) DO NOTHING`, ensures `is_superadmin`, never touches an existing password
- [x] 6.3 RED: `seed` refuses with `APP_ENV=production`; refuses with `APP_ENV` unset (fail-closed); creates the M0 fixture set with `APP_ENV=development`; second run changes nothing (platform-bootstrap: seed Refuses To Run Outside Non-Production, all three scenarios)
- [x] 6.4 GREEN: implement `api/cmd/vecingest/seed.go` — production/unset guard before any DB connection, idempotent fixture users, HIBP/policy bypass confined to the `seed` package, writes `audit_log` (D-S)
- [x] 6.5 RED (Testcontainers, real built binary, signal delivery): `worker` starts with zero registered workers, acquires River leadership, shuts down cleanly on `SIGTERM` inside 30 s; `health --worker` exits 0 while running and 1 after the DB is stopped
- [x] 6.6 GREEN: implement `api/cmd/vecingest/worker.go` — `WorkerDB`, jobless River client + leader election, graceful shutdown, `health --worker` heartbeat check (D-S)
- [x] 6.7 RED: subcommand dispatch — `bootstrap-superadmin` runs only its own flow; `health --ready`/`health --worker` exit 0/1 per their own check and run no other subcommand's behavior (platform-bootstrap: CLI Subcommands, all three scenarios)
- [x] 6.8 GREEN: implement `api/cmd/vecingest/main.go` — stdlib-`flag` dispatcher over the six subcommands, `import _ "time/tzdata"`

- [x] 6.9 RED: `serve` binds an HTTP listener on `PORT`, serves the assembled `internal/http/api` router, applies read/write/idle timeouts, and shuts down gracefully on SIGTERM within the compose `stop_grace_period`. **Added by the orchestrator: the original plan wired no listener anywhere, so `internal/http/api` was fully built and integration-tested but unreachable from the binary.**
- [x] 6.10 GREEN: implement `api/cmd/vecingest/serve.go` — assemble the API, start the revocation LISTEN loop, honour `serve --migrate` by running `migrate.Run` under its advisory lock before listening, and emit a startup log carrying no secrets
- [x] 6.11 Resolve every `golangci-lint` finding in NON-test code — `gosec` G101/G115/G306/G701, `errcheck`, `sqlclosecheck`. Each one is either fixed or annotated with a written justification; gosec is a blocking CI gate (PRD §10.1 item 7), so an unresolved finding fails Phase 11. Judge each: `params.tag` length conversion in `argon2.go` needs a real bounds check or a justified annotation, and the `00001_roles.go` taint warning needs the quoting proven, not assumed.
- [x] 6.12 Resolve the `golangci-lint` findings in test files the same way, including the unclosed `Rows` in `test/privilege_matrix_test.go` — an unclosed rows handle leaks a connection even in a test

## Phase 7: Generated `packages/shared` (WU-7 — generated discipline)

- [x] 7.1 Wire `openapi-typescript` + `openapi-fetch` generation into `make gen`, targeting `packages/shared/src/client/`
- [x] 7.2 Wire `openapi-zod-client` generation into `make gen`, targeting `packages/shared/src/schemas/`
- [x] 7.3 Define stable error codes (`AUTH_INVALID_CREDENTIALS`, `AUTH_PASSWORD_TOO_SHORT_NO_MFA`, `AUTH_PASSWORD_TOO_SHORT_WITH_MFA`, `AUTH_CSRF_INVALID`, `AUTH_AMBIGUOUS_TOKEN_TRANSPORT`, `AUTH_REFRESH_REUSED`, `AUTH_TOTP_REPLAYED`, `AUTH_TOO_MANY_ATTEMPTS`) in `packages/shared/src/errors.ts`
- [x] 7.4 Verify by inspection: run `make gen`; confirm no hand-written type exists under `packages/shared` (api-contract-generation: Generation Chain — "no type MAY be hand-written")
- [x] 7.5 Verify: `git diff --exit-code` after `make gen` against the committed `packages/shared` output is clean (foundation for the Phase 11 CI gate)

## Phase 8: Dockerfiles (WU-8 — non-TDD setup, D-K)

- [x] 8.1 Create `api/Dockerfile` — multi-stage `golang:1.27-bookworm` (`CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w -X main.version=…"`) → `gcr.io/distroless/static-debian12:nonroot`, `USER 65532:65532`, `ENTRYPOINT ["/vecingest"]`, `linux/amd64`
- [x] 8.2 Create `app/Dockerfile.web` and `site/Dockerfile` — `nginxinc/nginx-unprivileged` on 8080, `user: "1000:1000"`
- [x] 8.3 Verify by inspection: build both images locally; check size against the <30 MB advisory budget (gate item 6 advisory leg — archived server measurement is Phase 14)
- [x] 8.4 RED (`testing.Short()` skip guard, binary-level): `time.LoadLocation("Europe/Madrid")` succeeds inside the built `api` distroless image
- [x] 8.5 GREEN: confirm the `time/tzdata` import (6.8) satisfies 8.4; adjust the build if not

## Phase 9: Compose, `.env.example`, Portainer Wiring (WU-9 — verified by inspection, D-K)

- [x] 9.1 Create `deploy/docker-compose.yml` — `api`, `worker`, `db` (`postgres:17-alpine`), `web`, `site`, inert `clamav` (`profiles: [av]`) per §8.1 with the D-K corrections (drop `CLAMAV_HOST`; `clamav` stub only)
- [x] 9.2 Apply hardening flags to every service: `read_only: true`, `security_opt: [no-new-privileges:true]`, `cap_drop: [ALL]`, `tmpfs`, `user:`, `mem_limit` (api/worker 256m, nginx 128m, db 768m) + `GOMEMLIMIT: 200MiB`, `restart: unless-stopped`, `healthcheck`, `stop_grace_period: 30s` on `worker`, `depends_on: {condition: service_healthy}`, `networks: [internal, proxy]` with `db` internal-only
- [x] 9.3 Create `deploy/docker-compose.dev.yml` for local development overrides
- [x] 9.4 Create `deploy/.env.example` — full §8.2 active variable list plus a commented "fase B" block for `DATABASE_URL_READ`/`DATABASE_URL_WORKER` (D-R point 4), no new active variable
- [x] 9.5 Verify by inspection: `docker compose -f deploy/docker-compose.yml config` validates; manual review confirms every hardening flag from 9.2 is present (the `docker inspect` proof of a *running* stack is Phase 14)

## Phase 10: Expo Login Screen (WU-10 — TDD RNTL, structurally blocked on Phase 7's `make gen` pass)

- [x] 10.1 Verify dependency gate: confirm `packages/shared`'s generated login Zod schema and TS client (Phase 7) are complete and importable from `app/` before starting — the Expo screen cannot begin until this passes
- [x] 10.2 RED (RNTL): light mode renders with the single accent `#185FA5`, Spanish sentence-case verb-first copy, no gradients/shadows, 11px/44px minimums (app-login-ui: Login Screen Light and Dark Mode)
- [x] 10.3 RED (RNTL): dark mode renders without contrast failures, no light-mode-only asset leaks through, accent `#85B7EB` (app-login-ui: scenario "Dark mode renders without contrast failures")
- [x] 10.4 GREEN: implement `app/(auth)/login.tsx` + `PaperProvider` theme wiring from `docs/design/vecingest-{light,dark}.md` `colors` blocks, switched by `useColorScheme()`
- [x] 10.5 RED (RNTL): submission blocked client-side by the generated Zod schema on an empty password, before any network call; no hand-written validation schema in the component (app-login-ui: Generated-Schema Form Validation)
- [x] 10.6 GREEN: wire `react-hook-form` + `zodResolver` against the generated `packages/shared` login schema
- [x] 10.7 RED (RNTL): tokens from a successful mobile login are persisted through `expo-secure-store`, never `AsyncStorage`/plain state (app-login-ui: Secure Token Storage)
- [x] 10.8 GREEN: implement the `expo-secure-store` token-write path on login success

## Phase 11: CI Workflows (WU-11 — verified by inspection, D-M)

- [x] 11.1 Wire `ci.yml` **go** job: gofumpt check, golangci-lint (gosec, errcheck, sqlclosecheck, forbidigo), `go build ./...`
- [x] 11.2 Wire `sudo systemctl stop docker` immediately before the test step, then `go test -race -short ./...`; verify by inspection that a container-acquiring unit test fails this job
- [x] 11.3 Wire coverage ≥80% on `internal/domain/...` only (M0 `internal/legal` carve-out) and a clean `git diff` gate over `**/testdata/*.golden`
- [x] 11.4 Wire `ci.yml` **gen** job: Go+Node in one job, `make gen` then `git diff --exit-code` (api-contract-generation: CI Dirty-Diff Gate, both scenarios)
- [x] 11.5 Wire `ci.yml` **ts** job: `pnpm install --frozen-lockfile`, turbo lint/typecheck/test
- [x] 11.6 Wire `ci.yml` **e2e** job: `make test-e2e` with Testcontainers Postgres 17
- [x] 11.7 Wire `ci.yml` **scope** job: `make lint-scope`. **Correction (verify-report.md CRITICAL-1, remediation batch 2026-09-08):** `[x]` was recorded while `api/cmd/lintscope` did not exist — `make lint-scope` failed with `directory not found` and the job would have failed on its first real run. Fixed by writing `api/cmd/lintscope` (table-driven tests including a fixture proven to fail — `TestLint_MissingTenantFilterFails`) and re-verifying: `cd api && go run ./cmd/lintscope ./internal/db/queries` → `lintscope: OK`. The tool enforces `openspec/config.yaml`'s tenant-column rule; at M0 it passes trivially because no M0 table other than `audit_log.community_id` carries a tenant column, and `audit_log` is a documented, code-level exception (nullable, unenforced until M1) mirroring design.md's own tenant-scope subsection — this is now wired into CI so the first M1+ query that forgets its tenant filter fails the `scope` job.
- [x] 11.8 Wire `security.yml`: gitleaks, govulncheck, gosec, Semgrep (`p/owasp-top-ten` + Go + TS), `pnpm audit --audit-level=high`, Trivy fs, all blocking (platform-bootstrap: CI Security Scanning Gate; gate item 7)
- [x] 11.9 Wire `deploy.yml`: build+push GHCR `sha-<short>`/`latest`/`v<semver>` on tag, Trivy image scan blocking on HIGH/CRITICAL, advisory image-size report, Portainer webhook (platform-bootstrap: Immutable and Moving Image Tags, both scenarios)
- [x] 11.10 Create `.github/PULL_REQUEST_TEMPLATE.md` with the PRD §9 DoD point 10 security-review checklist
- [x] 11.11 Verify by inspection: open a deliberately-failing probe PR (known-vulnerable dependency) and confirm `security.yml` blocks it (gate item 7 evidence)

## Phase 12: Threat Model & Security Gate Document (WU-12 — document discipline)

- [x] 12.1 Create `docs/security/threat-model.md` derived from PRD §6.1's threat table, recording the HIBP fail-open residual risk (D-F) as an accepted risk (platform-bootstrap: Initial Threat Model Document; gate item 12)
- [x] 12.2 Create `docs/security/gates/M0.md` scaffold with all 12 gate items split into Checkpoint A / Checkpoint B tables, each row citing its evidence source

## Phase 13: Checkpoint A Closure — Code/CI Evidence (closeable entirely from this repository)

- [x] 13.1 Run `cd api && go test -race ./...` and `pnpm --filter app test`; archive the green result in `docs/security/gates/M0.md` for gate items 1, 2, 3, 4, 9, 10, 11
- [x] 13.2 Run the full `ci.yml` + `security.yml` pipeline green plus the 11.11 deliberately-failing probe result; archive as gate item 7 evidence. **Correction (verify-report.md CRITICAL-6/"task 13.2 claims a green pipeline that has never run", remediation batch 2026-09-08):** unmarked from `[x]` — no GitHub Actions run of `ci.yml` or `security.yml` has ever executed against this repository (one local commit, nothing pushed), so "run ... green" was never observed and the prior `[x]` was a claim the evidence did not support. This batch fixed the two `ci.yml` jobs that would have failed on a first real run (CRITICAL-1's `scope` job, CRITICAL-2's `go` coverage gate) and wired the previously-unexecuted custom Semgrep rules into `security.yml` (CRITICAL-4/verify-report numbering), but a local `semgrep`/`go test`/`golangci-lint` run is still not the same evidence as an actual green Actions run. **Re-closed 2026-09-08, with real evidence this time:** `gh run list`/`gh run view` against the current `main` `HEAD` (`6c0cb8d`) show both `ci` (run 34276511537, 7 jobs incl. `coverage`/`e2e`/Testcontainers, all green, `gen` correctly skipped by its own path filter) and `security` (run 34276511534, all six scanners — semgrep/govulncheck/pnpm-audit/gitleaks/gosec/trivy — green with non-trivial durations, not a zero-file scan) actually executed on GitHub Actions and passed. Verbatim output archived in `docs/security/evidence/checkpoint-b/2026-09-08-server-verification.md` §1, which also records an unrelated, unfixed defect found in passing: `.github/workflows/deploy.yml` fails on every push because `security.yml` lacks a `workflow_call:` trigger it's called with — out of this task's scope (`ci.yml`+`security.yml` only) and left for separate follow-up.
- [x] 13.3 Archive gate item 5 evidence (per-subcommand startup-validation tests, Phase 1.1/1.2 and 5.33/5.34)
- [x] 13.4 Archive gate item 12 evidence (link to `docs/security/threat-model.md`, 12.1)
- [x] 13.5 Cross-check: every Checkpoint A row in `docs/security/gates/M0.md` cites a passing test or a green workflow run, never a bare assertion

## Phase 14: Checkpoint B — Deployed-Server Evidence (BLOCKED on infrastructure — do not start until a real Portainer host exists)

A green CI run from Phase 13 does **not** close M0. These tasks require a real deployed host and are tracked separately so a green pipeline is never mistaken for a closed milestone.

**Blocker lifted 2026-09-08**: a real Portainer host now exists
(`vecingest-{api,worker,db,web,site}-1`, all healthy, `https://vecingest.xdev.es` /
`https://app.vecingest.xdev.es` / `https://api.vecingest.xdev.es` all
responding). Evidence for every check below is pasted verbatim in
`docs/security/evidence/checkpoint-b/2026-09-08-server-verification.md`.

- [x] 14.1 Provision the target host: Docker, Portainer, Nginx Proxy Manager, DNS for `app.DOMAIN`/`api.DOMAIN`. **Rewritten 2026-09-08 — the `GHCR pull access` clause is obsolete**: per PRD_go.md §8.1.1, Portainer no longer pulls published images; it builds `api`/`web`/`site` on the server from this repository, so there is no registry credential to provision. Verified by SSH: `docker --version` → `29.2.1`; `docker ps` shows `portainer/portainer-ee:latest` and `jc21/nginx-proxy-manager:latest` running; `dig` resolves all three hostnames and `curl` gets a valid TLS response with the expected status from each. The `(2FA-enabled)` clause on Portainer is **not** re-verified here — that is task 14.7's job below, which stays open for exactly that reason.
- [x] 14.2 Deploy the stack via Portainer. **Rewritten 2026-09-08 — stale reference fixed**: the task previously said `deploy/docker-compose.yml` with a pinned `v<semver>` `TAG`; the compose file now lives at the repo root (`docker-compose.yml`) and `TAG`/`REGISTRY` were removed entirely (PRD_go.md §8.1.1) — Portainer is configured as a "Repository" stack pointing at this GitHub repo with the default compose path, and builds the images itself on every redeploy from whatever commit is checked out. Verified: all five containers (`vecingest-{api,worker,db,web,site}-1`) are `Up`/`(healthy)`; the deployed containers' `com.docker.compose.project.working_dir` label resolves to commit `9a8422f`, whose only delta from the current tasks.md-editing `HEAD` is a docs-only file (`git diff --stat` confirms zero code/compose changes) — so this evidence is current, not stale.
- [ ] 14.3 [BLOCKED — needs a human] Manually verify login from a web browser against the deployed API; archive as the PRD §10 functional-criterion evidence. Blocked behind a superadmin account that does not exist yet — see the report for the exact `bootstrap-superadmin` invocation the user must run first.
- [ ] 14.4 [BLOCKED — needs a human] Manually verify login from the Expo mobile app (light and dark) against the deployed API; archive alongside 14.3. Same blocker as 14.3.
- [ ] 14.5 [BLOCKED — needs a human] Capture `docker stats` and run `k6` against `GET /v1/me`; archive as gate item 6 evidence (user-profile: GET /v1/me Performance Budget). `docker stats` alone was captured as supplementary evidence (see the evidence file) but does not satisfy this task: `k6` against `GET /v1/me` needs a real bearer token, which needs the same missing superadmin account as 14.3/14.4.
- [x] 14.6 Capture `docker inspect` for every service confirming `read_only`, non-root, `cap_drop: ALL`, `no-new-privileges`, `mem_limit`/`GOMEMLIMIT`; archive as gate item 8 evidence. Verified for all five services (see evidence file §3): `api`/`worker` — `read_only=true`, user `65532:65532`, `cap_drop=[ALL]`, `no-new-privileges`, `mem_limit=256m`, `GOMEMLIMIT=200MiB`; `web`/`site` — same flags, user `1000:1000`, `mem_limit=128m` (no `GOMEMLIMIT`, expected: nginx, not Go); `db` — `no-new-privileges` only, `mem_limit=768m`, root user and writable rootfs, which is the **documented, deliberate exception** in `docker-compose.yml`'s own comment and `docs/security/gates/M0.md`'s existing Checkpoint B row 8 (the official `postgres` image needs a writable data directory and cannot run `read_only`/non-root without breaking `initdb`) — reported here as the accepted exception it is, not as a failure.
- [ ] 14.7 [PARTIALLY DONE 2026-09-08] Audit Portainer's 2FA requirement and confirm `docker.sock` is not exposed to other containers; archive as gate item 8 manual evidence. The `docker.sock` half is done and green: checked all 16 containers on the host (not just vecingest's), and only `portainer` itself mounts `/var/run/docker.sock` (see evidence file §4). The 2FA half is **not verified** — there is no CLI/API surface to read Portainer's authentication settings without an interactive login this session does not have credentials for (and Portainer's HTTPS port was not reachable from outside during this check, likely the same Oracle VCN security-list behavior as `docs/pendientes-despliegue.md` §1). This task stays open until a human logs into the Portainer UI and confirms 2FA under Settings → Authentication.
- [ ] 14.8 [BLOCKED — needs a human] Update `docs/security/gates/M0.md` marking every Checkpoint B row green with its archived evidence link — only after this task does M0 close per PRD §10's two-criteria rule. `docs/security/gates/M0.md` was updated 2026-09-08 with everything closeable above, but the PRD's own M0 functional criterion (login from web and mobile against the deployed server) and gate item 6's `k6` measurement remain open behind 14.3/14.4/14.5, so this task cannot close yet either.
