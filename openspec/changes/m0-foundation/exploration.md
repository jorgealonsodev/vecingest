---
change: m0-foundation
phase: explore
project: vecingest
source: Engram observation 5227
date: 2026-09-04
---

## Exploration: M0 Foundation — monorepo, auth, Portainer deployment

### Current State
Greenfield repo (0 commits, no go.mod/package.json). Only `PRD_go.md` (authority), `docs/design/` (Stitch-derived UI system), `openspec/` config exist. `openspec/config.yaml` correctly captures stack (Go 1.23+/chi/huma/pgx/sqlc+goose/River, Expo+TS, generated Zod). Nothing is runnable yet; `make test`/`make gen`/`go test` do not exist.

### Affected Areas (PRD sections read)
- `PRD_go.md` §3 (roles, board_role, permission matrix)
- §5.1 Autenticación (amended: password floor 15/12, Argon2id m=19456/t=2/p=1, CSRF+Origin+SameSite on `/auth/refresh`, `bootstrap-superadmin` subcommand)
- §6 / §6.1 (NFRs, SLOs measured from M0, threat table, secrets handling for Portainer no-bind-mount, hardening)
- §7.1–7.10 (stack, repo layout, data model conventions, API surface, implementation decisions incl. multi-tenant authz, River, sqlc-scope lint, caching, ClamAV memory budget, PgBouncer phasing, scale-phase prep points)
- §8 / §8.1 / §8.2 (Portainer stack reference compose, three Postgres roles, `.env.example`, CI/CD pipeline to GHCR)
- §9 (DoD 10 points, conventions)
- §10 / §10.1 (milestone table, M0 security gate checklist — amended items)
- §11 (open risks: single server, two-language monorepo drift, ClamAV budget, premature scaling)
- §12 (glossary — Spanish UI terms vs English code identifiers)
- `docs/design/README.md` (non-negotiable UI rules: single accent #185FA5, one primary button, Spanish sentence-case copy verb-first, min 11px font/44px touch target, **dark mode mandatory from M0**)

### Scope boundary
**In M0**: monorepo skeleton (api/, app/, site/, packages/shared/, deploy/, docs/, .github/); Go binary `vecingest` with `serve`/`worker`/`migrate`/`seed`/`bootstrap-superadmin` subcommands; GHCR image publish; Portainer-deployable compose stack; base schema via goose+sqlc sufficient for auth (users, sessions, password_reset_tokens, user_mfa, audit_log — likely otp_challenges too since OTP purposes include more than phone verify); three Postgres roles (superuser/vecingest_owner/app_rw); `/v1/auth/*`, `/v1/me`, `/v1/health/{live,ready}`; generated TS client + Zod schemas in packages/shared; Expo login screen (light+dark) built against generated types; CI (lint, test, make-gen dirty-diff, security scanners); `docs/security/gates/M0.md` + initial `docs/security/threat-model.md`.

**Deliberately excluded from M0** (do not leak in): communities/units/invitations/office schema and portals (M1); incidents (M2); announcements/documents/directory (M3); store publication (M4); receipts/IBAN (M5); bookings (M6); meetings/voting/legal_rules content (M7); company tasks/invoices/time_entries (M8); NATS JetStream + Valkey (phase B); ClamAV (M2 gate, though the compose profile stub could exist inert); nearly all `worker` periodic jobs in §7.8 (none of the M0 login flow emits a domain event per §7.6's table).

### Structural decomposition (dependency order)
0. Monorepo skeleton: go.mod, pnpm workspace, Makefile, lefthook, empty CI. **No RED test is possible before this exists** — TDD only becomes enforceable once a Go package compiles (first real unit: Argon2id hashing or `internal/config` env validation).
1. DB bootstrap: goose migrations creating `vecingest_owner`/`app_rw` roles + grants/revokes; sqlc config/queries.
2. Auth domain schema: users, sessions, password_reset_tokens, user_mfa, otp_challenges, audit_log.
3. huma handlers for `/v1/auth/*`, `/v1/me`, `/v1/health/*`, built on domain services (strict TDD from unit's service layer).
4. Generation chain fires: Go structs → huma → `openapi.yaml` → `openapi-typescript`+`openapi-fetch` (TS client) → `openapi-zod-client` (Zod). **This forces ordering**: backend auth structs must be finalized before `packages/shared` has real types, before the Expo login screen can consume RHForm+Zod against generated schemas — frontend work is structurally blocked on a first `make gen` pass.
5. CLI subcommands wired: `serve --migrate`, `worker`, `migrate`, `seed`, `bootstrap-superadmin`.
6. Docker images: multi-stage `golang:1.23`→`distroless/static` (api/worker), nginx-unprivileged (web/site).
7. `deploy/docker-compose.yml` + `.env.example`, Portainer wiring.
8. Frontend: Expo skeleton, login screen against design system (light+dark), token storage (`expo-secure-store`).
9. CI pipeline: golangci-lint, `go test -race`, gen dirty-diff check, ESLint/Prettier/tsc, gitleaks/govulncheck/gosec/Trivy/Semgrep, coverage 80% on `internal/domain`+`internal/legal`.
10. Gate evidence: `docs/security/gates/M0.md`, `docs/security/threat-model.md`.

### Architectural questions (PRD evidence + silence)
1. **Two-phase migrate with two DSNs**: `.env.example` defines both `BOOTSTRAP_DATABASE_URL` (superuser) and `MIGRATIONS_DATABASE_URL` (owner), and §8 says "vecingest_owner aplica las migraciones" — implying `vecingest migrate` must connect first as bootstrap superuser (idempotent role/grant creation), then reconnect as owner for schema migrations. PRD never specifies this two-connection flow explicitly — must design it. Standard `goose` CLI runs one dir against one DSN; likely needs Go-based goose migrations (`AddMigrationContext`) or a custom two-stage runner inside `vecingest migrate`.
2. **Role password provisioning**: `app_rw`'s password must equal `APP_DB_PASSWORD` env value at migration time — goose static SQL can't read env vars; likely needs a `.go` goose migration reading env via `os.Getenv` to `ALTER ROLE ... WITH PASSWORD`. Not specified in PRD.
3. **REVOKE scope on append-only tables**: M0 gate only requires `audit_log` (votes/time_entries "cuando existan"). No PRD mechanism for default privileges on future tables — each milestone's migration that creates a new append-only table must carry its own REVOKE. Should be a documented convention, not automatic.
4. **Role hierarchy**: gate requires `app_rw` not be a member of the owner role. `vecingest_owner` must itself be LOGIN-capable (it has its own DSN) but is never used at runtime by api/worker.
5. **distroless/static running migrations**: no shell in distroless; compose command is `serve --migrate` (not a separate `migrate` step) — this forces `goose` to run as an embedded Go library (`pressly/goose/v3`) reading migrations via `//go:embed migrations/*.sql`, plus a Postgres advisory lock (per §8) so `worker` doesn't race `api` on boot.
6. **River schema in M0**: no M0 event actually needs enqueuing (§7.6 events are all M1+), but §7.10 point 4 wants job infra interface-ready from M0, and the `worker` binary/healthcheck is an M0 deliverable per compose. Open question: provision River's Postgres schema now (cheap, avoids first-time migration friction in M1) vs. defer until the first real job. Recommend provisioning now.
7. **`/me` semantics with no community/office schema yet**: §7.4 says GET `/me` returns "usuario + membresías," but offices/communities are M1. PRD endpoint list is not milestone-tagged, so it's ambiguous whether M0 ships office/office_members schema too, or `/me` just returns an empty memberships array for M0. Flag for the proposal to resolve explicitly.
8. **CSRF token scheme unspecified**: §5.1 (amended) mandates a CSRF token on `/auth/refresh` beyond SameSite+Origin but doesn't pick double-submit-cookie vs synchronizer-token vs custom header. Open decision.
9. **`internal/legal` coverage gate in M0**: config.yaml pins 80% coverage on `internal/domain/...` AND `internal/legal/...`, but `internal/legal` (LegalRulesService) has no real M0 content (first real consumer is M7 voting). Needs a stub package or an explicit scope carve-out decision — PRD silent.
10. **`Limiter`/`Cache`/`Queue` interfaces built in M0** even with single (in-process/Postgres) implementations, per §7.7/§7.10 — must not hardcode `httprate` directly into handlers, to keep the phase-B swap config-only as the PRD requires.

### Risks specific to M0
- §11 "single server, no HA" collides with the M0 functional criterion itself: "login desde web y móvil contra el servidor desplegado por Portainer" requires a *real deployed host* — this cannot be satisfied by CI alone in a repo with no server.
- §11 "two-language monorepo drift" — the `make gen` dirty-diff CI check is itself an M0 deliverable with a chicken-egg bootstrap problem (no endpoints ⇒ empty openapi.yaml ⇒ nothing to generate) until the first auth structs land.
- HIBP k-anonymity check needs an outbound network call in tests; needs a fake/mockable HIBP server for deterministic CI, not specified in PRD.
- TOTP tests are time-window sensitive; risk of CI flakiness under Testcontainers.
- Coverage gate on empty `internal/legal` package risks false-negative CI failures (0% on essentially no code).
- ClamAV/EICAR is M2 scope; verify no M0 endpoint (login has no file upload) accidentally requires it.

### Gate items: CI-testable now vs infra-dependent
**CI-testable from day one** (Testcontainers Postgres + Go test suite, no deployed host): Argon2id params + password floor + HIBP rejection (mockable); refresh/invitation/reset hashing + reuse-invalidates-family; cookie Strict+Path + Origin + CSRF 403 tests; TOTP mandatory for superadmin + separate route; `internal/config` startup validation; gitleaks/govulncheck/gosec/pnpm-audit/Trivy/Semgrep wired and blocking; `app_rw` REVOKE UPDATE/DELETE/TRUNCATE test on `audit_log`; `httprate` trust-proxy test with spoofed XFF; slog PII redaction test; threat-model document.

**Needs a real deployed host, not achievable by CI alone**: resource budget evidence explicitly "en el servidor" (`docker stats`/`k6` p95 of `GET /v1/me` < 50ms, image < 30MB, boot < 1s) — PRD ties this evidence to the same deployment used for the functional criterion; container hardening evidence (`docker inspect`) and Portainer-with-2FA/no-exposed-docker.sock are inherently about the live Portainer instance; the functional criterion itself (login from web+mobile against the Portainer-deployed server) requires DNS, GHCR pull, NPM proxy, and a running host.

**Implication**: "M0 done" in this repo can only mean two separable things — (1) repo/CI-closable: all code + CI-verifiable checklist items green, deployable artifacts correct; (2) org-closable, outside repo scope: an operator manually deploys to a real Portainer host and archives the docker-stats/k6/docker-inspect evidence in `docs/security/gates/M0.md`. The proposal should not claim M0 is "done" from a green CI run alone.

### Recommendation
Sequence tasks exactly as the dependency order above; treat work unit 0 (scaffolding) as pre-TDD, everything from unit 2 onward as strict-TDD-enforced. Resolve the 10 architectural questions explicitly in the proposal (especially the two-DSN migrate flow and the `/me`-without-offices ambiguity) before writing tasks, since they affect migration file structure and the auth handler's shape respectively.

### Risks
- Real-host dependency for full gate closure (see above).
- `make gen` bootstrap chicken-egg.
- Two-phase migration DSN design is genuinely novel/unspecified by PRD.
- Coverage gate on an empty `internal/legal` package.
- HIBP/TOTP test determinism.

### Ready for Proposal
Yes — scope, decomposition, and open questions are clear enough to proceed to `sdd-propose`, provided the proposal explicitly resolves the 10 architectural questions (or defers them to `sdd-design` with named decisions to make) and states the CI-closable vs org-closable split for M0's gate.
