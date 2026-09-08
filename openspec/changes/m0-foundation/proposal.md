---
change: m0-foundation
phase: propose
project: vecingest
date: 2026-09-04
authority: PRD_go.md §5.1, §6/§6.1, §7, §8/§8.1/§8.2, §9, §10/§10.1, §11, §12
binds: openspec/changes/m0-foundation/exploration.md, openspec/changes/m0-foundation/research.md (Engram sdd/m0-foundation/research, obs 5244)
---

# Proposal: M0 Foundation — monorepo, auth, and Portainer-deployable stack

## Intent

The repository has zero commits and no runnable toolchain. M0 (PRD §10) must turn it into a monorepo that produces a single Go binary (`serve`/`worker`/`migrate`/`seed`/`bootstrap-superadmin`), a base schema under `goose`+`sqlc`, working authentication with `/v1/me`, a generated TypeScript client, GHCR images, and a Portainer-deployable stack. Success is PRD §10's functional criterion — login from web and mobile against the Portainer-deployed server — plus every check in the §10.1 "M0 — Base" gate closed with archived evidence in `docs/security/gates/M0.md`.

M0 is the milestone where the security posture of the whole product is fixed: password hashing, token rotation, cookie/CSRF handling, the restricted database role, the trusted-proxy boundary, and log redaction are all cheaper to get right now than to retrofit.

## Scope

### In Scope

- Monorepo skeleton: `api/`, `app/`, `site/`, `packages/shared/`, `deploy/`, `docs/`, `.github/`, `Makefile`, pnpm workspace + Turborepo, `lefthook`, Renovate.
- Go module `api/`: chi + huma + pgx, `internal/config` startup validation, `internal/domain`, `internal/db` (sqlc), embedded `goose` migrations.
- Three PostgreSQL roles per **D6**: image superuser (one-shot role bootstrap), `vecingest_owner` (schema owner, runs goose), `app_rw` (runtime, owns nothing, no `UPDATE`/`DELETE`/`TRUNCATE` on append-only tables, not a member of the owner role).
- Auth-subset schema only: `users`, `sessions`, `password_reset_tokens`, `user_mfa`, `otp_challenges`, `audit_log`. River's schema provisioned.
- Endpoints: `/v1/auth/login`, `/refresh`, `/logout`, `/forgot-password`, `/reset-password`, superadmin login on a separate route, `GET /v1/me`, `/v1/health/live`, `/v1/health/ready`.
- Argon2id credential handling per **D5** (`m=19456 KiB, t=2, p=1`, 16-byte salt, 32-byte tag), password floor per **D3** (15 chars without 2FA, 12 with TOTP active; the error states which rule applies), HIBP k-anonymity rejection.
- JWT access (15 min) + refresh (30 d, rotating, reuse invalidates the family); refresh tokens, reset tokens and short codes stored SHA-256 hashed.
- CSRF protection on `/auth/refresh` per **D4**: token + `SameSite=Strict` + scoped `Path` + `Origin` validation.
- TOTP mandatory for `superadmin`; recovery codes.
- `vecingest bootstrap-superadmin` per **D1**: idempotent, `APP_ENV`-independent, credentials passed at invocation, distinct from `vecingest seed`.
- Rate limiting behind a `Limiter` interface with a correct trusted-proxy chain; `slog` PII redaction.
- Generation chain: Go structs → huma `openapi.yaml` → `openapi-typescript`/`openapi-fetch` client → `openapi-zod-client` Zod schemas in `packages/shared`, with a `make gen` dirty-diff CI gate.
- Expo app skeleton with the login screen in light **and dark** mode (both themes shipped from M0 because `docs/design/` defines a light and a dark system and the app reads the device theme; `docs/design/README.md` does NOT itself mandate dark mode — its non-negotiables are the single accent, no gradients or shadows, Spanish sentence case and the 11px/44px minimums), `expo-secure-store` token storage.
- Dockerfiles (`golang:1.2x` → `distroless/static`, nginx-unprivileged), `deploy/docker-compose.yml`, `.env.example`, GHCR publish, CI (lint, `go test -race`, coverage, gitleaks/govulncheck/gosec/Trivy/Semgrep/`pnpm audit`).
- `docs/security/threat-model.md` and `docs/security/gates/M0.md`.

### Out of Scope

- Communities, units, offices, invitations, role portals (M1); incidents (M2); announcements/documents/directory (M3); store publication (M4); receipts/IBAN (M5); bookings (M6); meetings/voting/`legal_rules` content (M7); company tasks/invoices/`time_entries` (M8).
- NATS JetStream, Valkey, PgBouncer, Citus (phase B/C — configuration-only swaps behind the `Cache`/`Limiter`/`Queue`/`Search` interfaces).
- ClamAV (M2 gate; at most an inert compose profile).
- All periodic `worker` jobs — no M0 flow emits a domain event (§7.6 events are M1+).
- R2 storage, SMS provider, Turnstile, Sentry, OpenTelemetry wiring beyond configuration placeholders.
- Server hardening itself (runbook, §6.1) — M0 only archives evidence of it.

## Capabilities

### New Capabilities

- `auth-credentials`: password policy (D3), Argon2id hashing/verification (D5), HIBP k-anonymity check, progressive lockout.
- `auth-session-tokens`: JWT access/refresh issuance, refresh rotation with family invalidation, cookie vs body transport, `sessions` table.
- `auth-csrf-origin`: CSRF token issuance/validation plus `Origin`/`SameSite`/`Path` enforcement on `/auth/refresh` (D4).
- `auth-mfa-totp`: TOTP enrolment, verification, recovery codes, mandatory for `superadmin`, separate superadmin login route.
- `user-profile`: `GET /v1/me` returning the user plus `is_superadmin`.
- `platform-bootstrap`: `vecingest` subcommands, `internal/config` startup validation, embedded migration runner with an advisory session lock, `bootstrap-superadmin` (D1).
- `db-access-control`: three-role provisioning, grants/revokes, append-only enforcement including the `SECURITY DEFINER` invariant (D6).
- `request-protection`: `Limiter` interface, trusted-proxy client-IP resolution, `slog` PII redaction.
- `service-health`: `/v1/health/live` and `/v1/health/ready`.
- `api-contract-generation`: `make gen` chain and its CI dirty-diff gate.
- `app-login-ui`: Expo login screen (light + dark) on generated Zod schemas and secure token storage.

### Modified Capabilities

None — `openspec/specs/` is empty.

## Approach

### Research findings that bind this proposal

| # | Finding | Consequence for M0 |
|---|---|---|
| 1 | `httprate` does **not** do trusted-proxy IP handling; its own IP helpers are deprecated as spoofable | Install chi v5.3.0 `middleware.ClientIPFromXFF` (NPM host as a `/32` trusted prefix) **ahead** of `httprate`; the key function reads `middleware.GetClientIP(ctx)` through `httprate.CanonicalizeIP`. Two packages, not one config knob. §10.1's "`trust proxy` acotado a `PROXY_IP`" is satisfied by this chain, not by an `httprate` option |
| 2 | `gorilla/csrf` is unusable: CVE GO-2025-3884 (`TrustedOrigins` trusts the HTTP variant of an HTTPS host → MITM CSRF) and the Gorilla toolkit is discontinued as of 2026-05-01 | Hand-rolled signed/HMAC double-submit token (OWASP's recommended variant). Stdlib `net/http.CrossOriginProtection` (Go 1.25+) reinforces the `Origin` leg and is transparent to native clients that send no `Origin`, but issues **no token**, so it cannot satisfy D4 alone. Toolchain implication: pin Go **1.25+** (current stable 1.27.1) — permitted by the PRD's "Go 1.23+" floor |
| 3 | "By default, goose does not lock the database during migrations" | Wire `WithSessionLocker` (Postgres **session-level** advisory lock) explicitly, or `api` and `worker` race on boot |
| 4 | `distroless/static` has no shell | Migrations run **embedded**: `//go:embed migrations/*.sql` + goose invoked from inside the binary at startup, not as a separate container command |
| 5 | Only a Go migration can read `APP_DB_PASSWORD` from the environment and open a second connection under the bootstrap role | The three-role bootstrap is a **Go** migration (`AddMigrationNoTxContext`), not SQL |
| 6 | `SECURITY DEFINER` is the real append-only enforcement boundary — a function owned by the table owner writes regardless of the revoke if `app_rw` or `PUBLIC` holds `EXECUTE` (and `PUBLIC` gets it by default) | Explicit invariant **plus a migration-time assertion**, not a comment |
| 7 | `ALTER DEFAULT PRIVILEGES` only affects future objects, keys off the creating role, and its per-schema form cannot revoke a schema-less grant | Every migration creating an append-only table carries its own explicit `REVOKE`; documented convention, never assumed automatic |
| 8 | `golang.org/x/crypto/argon2` is a bare KDF — no PHC encoding, no verification, no constant-time compare | All three are hand-written and tested (`crypto/rand` salt, `RawStdEncoding` PHC string, `crypto/subtle.ConstantTimeCompare`, rehash-on-param-bump) |
| 9 | `slog` redaction needs three legs — `ReplaceAttr` never sees the message string, `LogValuer` does not fire reliably on nested fields | Wrapping handler + `ReplaceAttr` on sensitive keys + `LogValuer` types on PII-bearing domain types, plus a call-site discipline rule against interpolating PII into messages |
| 10 | River's exact current release and minimum PostgreSQL version could **not** be verified (internally inconsistent fetches) | **OPEN — do not pin.** `sdd-design` resolves it against the live README and docs. Do not invent a version number |

### Where strict TDD actually starts

Strict TDD is enabled, but nothing is testable before `go.mod` exists and a Go package compiles. This proposal states plainly, and the tasks phase **must not pretend the whole milestone is test-driven**:

| Class | Work | Discipline |
|---|---|---|
| **Non-TDD setup** | WU-0 (module/workspace scaffolding), WU-8 (Dockerfiles), WU-9 (compose + `.env.example`) | Written first, then verified by the tests and CI that follow. No RED phase is possible |
| **Configuration verified by inspection** | CI workflow YAML, `lefthook`, Renovate, golangci-lint config, compose hardening flags | Verified by a green pipeline, a deliberately-failing probe PR, and `docker inspect` — not by unit tests |
| **Strict TDD (RED first, mandatory)** | Everything from WU-2 onward: the Argon2id hasher, `internal/config` validation, token services, CSRF token, TOTP, handlers, the `app_rw` privilege tests, the trusted-proxy test, the redaction test | Every task states its RED test first |

**The first genuine RED test in this repository is the Argon2id hasher or `internal/config` validation** (WU-2), immediately after the module compiles.

### The generation chain is a hard ordering constraint

```
Go input/output structs  →  huma  →  api/openapi/openapi.yaml
                                          ↓
                          openapi-typescript + openapi-fetch  →  packages/shared (TS client)
                                          ↓
                              openapi-zod-client  →  packages/shared (Zod schemas)
                                          ↓
                    Expo login screen (React Hook Form + zodResolver)
```

The Expo login screen is **structurally blocked** on a completed backend auth struct pass through `make gen`. This is a dependency, not a preference: no hand-written types exist on either side (PRD §7.2, §9). It also resolves the `make gen` chicken-and-egg — the dirty-diff CI check only becomes meaningful once WU-4's auth structs exist, so it is wired in WU-5 and enforced from WU-10 onward.

### Work breakdown (dependency order)

| WU | Work | Depends on | Discipline |
|---|---|---|---|
| 0 | Monorepo skeleton: `go.mod` (Go 1.25+), pnpm workspace, Turborepo, `Makefile`, `lefthook`, empty CI | — | setup |
| 1 | `internal/config` + embedded goose runner with `WithSessionLocker`; three-role Go migration (D6) | 0 | TDD |
| 2 | Argon2id hasher (PHC encode/decode, constant-time verify, rehash) and password policy (D3, D5) | 0 | **TDD — first RED test** |
| 3 | Auth schema migrations + sqlc queries: `users`, `sessions`, `password_reset_tokens`, `user_mfa`, `otp_challenges`, `audit_log`; append-only revokes + `SECURITY DEFINER` assertion; River schema | 1 | TDD (Testcontainers) |
| 4 | Auth domain services: login, refresh rotation with family invalidation, reset, TOTP, HIBP client (mockable), signed double-submit CSRF token (D4) | 2, 3 | TDD |
| 5 | huma handlers + chi middleware chain (`ClientIPFromXFF` → `httprate`), `slog` redaction handler, `/v1/me`, health endpoints; **first `make gen` pass** | 4 | TDD |
| 6 | CLI subcommands: `serve --migrate`, `worker`, `migrate`, `seed`, `bootstrap-superadmin` (D1) | 1, 4 | TDD |
| 7 | `packages/shared`: generated TS client + Zod schemas + stable error codes | 5 | generated |
| 8 | Dockerfiles: multi-stage → `distroless/static` (api/worker), nginx-unprivileged (web/site) | 6 | setup |
| 9 | `deploy/docker-compose.yml` + `.env.example` + Portainer wiring, hardening flags | 8 | inspection |
| 10 | Expo skeleton + login screen (light + dark) + `expo-secure-store` | 7 | TDD (RNTL) |
| 11 | CI: lint, `go test -race`, coverage, `make gen` dirty-diff, security scanners, GHCR publish | 5, 7 | inspection |
| 12 | `docs/security/threat-model.md` + `docs/security/gates/M0.md` | 11 | document |

### M0 closes in two checkpoints (D2)

Both are tracked in `docs/security/gates/M0.md`. **Work must never block waiting on a server that does not exist.** A green CI run alone does not close M0.

**Checkpoint A — code/CI** (closeable from this repository with CI and Testcontainers):

| §10.1 item | Evidence |
|---|---|
| Argon2id params + password floor (15/12) + HIBP rejection | test |
| Refresh/invitation/reset hashed; refresh reuse invalidates the family | e2e test |
| Cookie `Strict` + scoped `Path`; `Origin` validation and CSRF token on `/auth/refresh` | e2e: foreign origin → 403; valid cookie without CSRF token → 403 |
| TOTP mandatory for `superadmin`; separate superadmin login route | test |
| `internal/config` refuses to start with a missing variable | startup test |
| `gitleaks`, `govulncheck`, `gosec`, `pnpm audit --audit-level=high`, Trivy, Semgrep blocking in CI | green workflow + a deliberately-failing probe PR blocked |
| `app_rw` without `UPDATE`/`DELETE`/`TRUNCATE` on `audit_log`, not owner, not a member of the owner role | test attempting all three operations |
| Rate limiting with the trusted-proxy chain scoped to `PROXY_IP` | test: spoofed `X-Forwarded-For` from another IP → ignored |
| `slog` PII redaction (email, phone, IBAN) | test |
| Initial threat model (`docs/security/threat-model.md`) | document |

**Checkpoint B — infra** (requires a real deployed host; an operator archives the evidence):

| §10.1 item / criterion | Evidence |
|---|---|
| PRD §10 functional criterion: login from web and mobile against the Portainer-deployed server | manual verification archived |
| Resource budget "en el servidor": `api`/`worker` < 128 MB at rest, image < 30 MB, boot < 1 s, p95 `GET /v1/me` < 50 ms | `docker stats` + `k6` archived |
| Containers `read_only`, non-root distroless `nonroot`, `cap_drop: ALL`, `mem_limit` + `GOMEMLIMIT` | `docker inspect` archived |
| Portainer with 2FA and no `docker.sock` exposed to other containers | archived evidence |

Authoring the compose hardening flags belongs to Checkpoint A (WU-9); only the `docker inspect` proof of a running stack belongs to Checkpoint B. Image size may be measured advisorily in CI, but the archived §10.1 evidence is the server measurement.

## Assumptions

Carried from the orchestrator handoff and marked as assumptions, not PRD text. Each is a candidate for `sdd-design` to confirm or overturn.

1. **Prisma-free auth-subset schema only** — `users`, `sessions`, `password_reset_tokens`, `user_mfa`, `otp_challenges`, `audit_log`. No office/community tables in M0.
2. **`/v1/me` at M0 returns the user plus `is_superadmin`.** §7.4's "usuario + membresías" contract completes at M1, when office/community schema exists. The endpoint list in §7.4 is not milestone-tagged; this resolves that ambiguity.
3. **River's schema is provisioned in M0** even though no M0 event enqueues a job, because the `worker` binary and its healthcheck are M0 compose deliverables (§8) and first-run migration friction is worse later. River's migration set cannot all run in one transaction (its migration 6 depends on migration 4's enum change), which constrains how it folds into goose.
4. **The 80 % coverage threshold applies to `internal/domain` only at M0.** `internal/legal` has no M0 content — its first consumer is M7 voting — and must be excluded from the gate rather than failing CI on an empty package. `openspec/config.yaml` `coverage_scope` needs a corresponding M0 carve-out.
5. **ClamAV deferred to M2; NATS and Valkey are phase B.**
6. Go toolchain pinned to **1.25+** so `net/http.CrossOriginProtection` is available (finding 2). Within the PRD's "Go 1.23+" floor.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `api/` | New | Go module, `cmd/vecingest`, `internal/{config,http,domain,db,jobs}`, `migrations/`, `openapi/`, `test/`, `Dockerfile` |
| `app/` | New | Expo skeleton, `(auth)` routes, login screen light + dark |
| `packages/shared/` | New | Generated TS client, Zod schemas, stable error codes |
| `site/` | New | Astro placeholder + nginx Dockerfile |
| `deploy/` | New | `docker-compose.yml`, `docker-compose.dev.yml`, `.env.example` |
| `.github/workflows/` | New | `ci.yml`, `security.yml`, `deploy.yml`; PR template with the §9 DoD point 10 checklist |
| `docs/security/` | New | `threat-model.md`, `gates/M0.md` |
| `Makefile`, `lefthook.yml`, `renovate.json` | New | Single workspace entrypoint (`make dev/gen/test/lint`) |
| `openspec/config.yaml` | Modified | M0 coverage carve-out for `internal/legal` (assumption 4) |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| River version/minimum-PostgreSQL unresolved (finding 10) | High | `sdd-design` resolves against live docs before any pin; no version invented here |
| Two-connection role bootstrap is unspecified by the PRD and not a goose-endorsed pattern | Med | Go migration + Testcontainers test asserting the resulting privilege matrix, not the mechanism |
| `SECURITY DEFINER` silently defeats the append-only revoke | Med | Explicit invariant + migration-time assertion + a test that tries all three write paths |
| Hand-rolled CSRF token is security-critical code with no upstream | Med | Signed/HMAC double-submit per OWASP, layered with stdlib `CrossOriginProtection`; both 403 cases tested |
| Trusted-proxy chain silently reverts to spoofable behaviour if the chi middleware is omitted | Med | The spoofed-XFF test is a gate item; middleware order asserted in a test |
| `make gen` chicken-and-egg (no endpoints ⇒ empty `openapi.yaml`) | Med | Dirty-diff check wired at WU-5 after the first real auth structs, enforced from WU-11 |
| HIBP check needs an outbound call in tests | Med | Interface + fake HIBP server; no network in CI |
| TOTP tests are time-window sensitive | Med | Injectable clock, never `time.Now()` inside the domain |
| Coverage gate on the empty `internal/legal` package | High | Assumption 4 carve-out, applied in `openspec/config.yaml` |
| Single server, no HA (§11) collides with the functional criterion | High | D2's two-checkpoint split; Checkpoint A never waits on Checkpoint B |
| Two-language monorepo contract drift (§11) | Med | Generated-only `packages/shared` + CI dirty-diff |

## Rollback Plan

Required by `rules.proposal` — this change touches auth and personal data.

- **Code**: the repository has zero commits and no production deployment. Rollback is `git revert` of the WU commits on `main`, or resetting `main`; there is no prior release to preserve compatibility with.
- **Database**: no production data exists at M0. Rollback is `goose down` to zero or dropping and recreating the database. Every migration ships a `down`; the role-bootstrap Go migration's `down` drops `app_rw` and `vecingest_owner` after reassigning ownership.
- **Deployment**: Portainer pins `TAG`; rollback is re-pointing `TAG` at the previous GHCR image and redeploying. Images are immutable and version-tagged, never rolled back by mutating `latest`.
- **Superadmin**: `bootstrap-superadmin` is idempotent (D1), so re-running after a rollback neither duplicates nor errors.
- **Point of no return**: the first real user account. Until then M0 is fully disposable; after it, expand/contract applies (§9, never `DROP`/`RENAME` in the same release as the code that stops using the shape).

## Dependencies

- A real host with Portainer, DNS, Nginx Proxy Manager and GHCR pull access — required for Checkpoint B only.
- Go 1.25+ toolchain; PostgreSQL 17 (`btree_gist`, `pg_partman`, `pg_stat_statements`); Docker with Testcontainers support in CI.
- GitHub Actions with GHCR publish permission; accounts for Trivy/Semgrep/gitleaks (all OSS, no paid tier assumed).
- Expo SDK version pinned in M0 and frozen until M4 (§7.1).
- `docs/design/` design system as the binding UI contract: both the light and the dark system, and the README's non-negotiable rules.

## Success Criteria

- [ ] `make dev`, `make gen`, `make test`, `make lint` all run from a clean clone.
- [ ] `vecingest serve`, `worker`, `migrate`, `seed`, `bootstrap-superadmin` all work; `bootstrap-superadmin` run twice neither duplicates nor errors.
- [ ] Login, refresh with rotation, logout, password reset and superadmin TOTP login pass e2e against a real Postgres in Testcontainers.
- [ ] `GET /v1/me` returns the user plus `is_superadmin` for an authenticated caller.
- [ ] `make gen` produces no diff in CI; `packages/shared` is entirely generated.
- [ ] The Expo login screen authenticates against the deployed API in both light and dark mode.
- [ ] Every Checkpoint A item in `docs/security/gates/M0.md` is green with archived evidence.
- [ ] Every Checkpoint B item in `docs/security/gates/M0.md` is green with archived server evidence, and PRD §10's functional criterion is met.
