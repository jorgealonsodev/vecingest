```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:d96a52aaadebba0bb1db42def68dfa2246e2bb76725f2bd0a02e0f2bcbcec0f7
verdict: fail
blockers: 6
critical_findings: 6
requirements: 41/51
scenarios: 77/92
test_command: cd api && go test -race -count=1 ./...
test_exit_code: 0
test_output_hash: sha256:06d43cd6ad4ae1d875d736fd44fa2e91779fcccacf3549e6982de9ce0008100e
build_command: cd api && go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: m0-foundation
**Version**: specs 11 capabilities / 51 requirements / 92 scenarios
**Mode**: Strict TDD (`openspec/config.yaml`: `strict_tdd: true`)
**Date**: 2026-09-08

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 174 |
| Tasks complete | 166 |
| Tasks incomplete | 8 (Phase 14, all `[BLOCKED on infra]`) |

Phase 14 is correctly untouched and correctly not marked green anywhere. That
is not a finding. The findings below concern tasks marked `[x]` whose claimed
outcome does not hold.

### Build & Tests Execution

**Build**: PASSED — `cd api && go build ./...`, exit 0, no output.

**Tests**: PASSED.

```text
$ cd api && go test -race -count=1 ./...
27/27 packages ok, exit 0
(cmd/vecingest 64.3s, internal/http/api 76.9s, internal/domain/auth/session 53.7s,
 internal/domain/auth/mfa 47.2s, migrations/schema 42.1s, test 58.6s, ...)

$ make test-e2e            -> ok  .../test  26.3s, exit 0
$ pnpm turbo run lint typecheck test --force
                           -> 9/9 tasks; app: 4 suites, 10 tests passed
$ cd api && gofumpt -l .   -> (empty)
$ cd api && ~/go/bin/golangci-lint run ./...   -> 0 issues
$ make lint-compose        -> OK
$ make gen                 -> byte-stable: all 31 generated files unchanged (sha256 verified)
```

`-count=1` was used throughout; no cached result was accepted.

**Docker-free `-short` contract**: VERIFIED GENUINE.

```text
$ cd api && DOCKER_HOST=unix:///nonexistent-docker.sock TESTCONTAINERS_RYUK_DISABLED=true \
    go test -race -short -count=1 ./...
27/27 packages ok, exit 0, ~1.0-1.7s per package
```

Every container-acquiring path is guarded. The guards sit in the shared
acquisition helpers (`internal/testhelpers.AppRWHandles` callers,
`test/helpers_test.go:36`, `internal/platform/migrate/testhelpers_test.go:25`,
`internal/db/handles_pg_test.go:25`, `migrations/schema/invariants_test.go:22`,
`internal/http/api/api_integration_test.go:52`) so the skip fires before
acquisition, plus per-test guards where no helper exists. The binary-exec tests
(`test/docker_tzcheck_test.go`, `cmd/vecingest/worker_test.go`,
`cmd/vecingest/serve_test.go`) are guarded too. This claim holds.

**Coverage**: BROKEN AS WIRED IN CI.

```text
$ cd api && go test -race -covermode=atomic -coverprofile=... ./internal/domain/...
total: 85.1%          <- above the 80% gate

$ cd api && go test -race -short -covermode=atomic -coverprofile=... ./internal/domain/...
total: 66.0%          <- BELOW the 80% gate; this is what ci.yml actually runs
  audit 56.3% | lockout 83.8% | mfa 56.1% | password 89.2%
  session 12.3% | token 90.9% | pii 100.0%
```

See CRITICAL-2.

### Spec Compliance Matrix

| Capability | Req | Scen | COMPLIANT | PARTIAL | UNTESTED | FAILING | DEFERRED (Phase 14) |
|---|---|---|---|---|---|---|---|
| api-contract-generation | 2 | 3 | 1 | 1 | 1 | 0 | 0 |
| app-login-ui | 3 | 3 | 3 | 0 | 0 | 0 | 0 |
| auth-credentials | 5 | 10 | 8 | 2 | 0 | 0 | 0 |
| auth-csrf-origin | 4 | 9 | 9 | 0 | 0 | 0 | 0 |
| auth-mfa-totp | 6 | 11 | 11 | 0 | 0 | 0 | 0 |
| auth-session-tokens | 7 | 13 | 11 | 2 | 0 | 0 | 0 |
| db-access-control | 5 | 8 | 8 | 0 | 0 | 0 | 0 |
| platform-bootstrap | 11 | 22 | 17 | 1 | 2 | 0 | 2 |
| request-protection | 3 | 5 | 4 | 0 | 0 | 1 | 0 |
| service-health | 2 | 3 | 2 | 1 | 0 | 0 | 0 |
| user-profile | 3 | 5 | 3 | 0 | 1 | 0 | 1 |
| **Total** | **51** | **92** | **77** | **7** | **4** | **1** | **3** |

**Compliance summary**: 77/92 scenarios COMPLIANT; 41/51 requirements fully
compliant.

Non-COMPLIANT scenarios, named:

| Requirement | Scenario | Evidence | Result |
|---|---|---|---|
| request-protection: Trusted-Proxy Client IP Resolution | Untrusted intermediary rejected as source | none; behaviour empirically contradicted | FAILING |
| api-contract-generation: CI Dirty-Diff Gate | Uncommitted generated diff fails CI | `ci.yml` `gen` job, never executed | UNTESTED |
| api-contract-generation: CI Dirty-Diff Gate | Clean generation passes CI | `make gen` byte-stability verified locally only | PARTIAL |
| auth-credentials: Progressive Lockout | Lockout by email (…has sent a lockout alert email) | `lockout_test.go` with `fakeMailer`; production mailer is a log-only stand-in | PARTIAL |
| auth-credentials: Progressive Lockout | Lockout by IP | `TestRecordFailure_LocksByIPAcrossDifferentEmails` passes, but the IP it keys on is attacker-controllable (CRITICAL-3) | PARTIAL |
| auth-session-tokens: Bearer-Authenticated Logout | Valid Bearer token revokes the whole family | `TestAuthFlow_LoginRefreshLogout` — family had one session | PARTIAL |
| auth-session-tokens: Bearer-Authenticated Logout | Refresh cookie cleared regardless of request path | `TestClearedRefreshCookie_MaxAgeZero` (builder unit test); the logout response's `Set-Cookie` is never asserted | PARTIAL |
| platform-bootstrap: CI Security Scanning Gate | Vulnerable dependency blocks merge | local red→green `govulncheck` run; `security.yml` has never executed | PARTIAL |
| platform-bootstrap: Immutable and Moving Image Tags | Version tag never overwritten | `deploy.yml` has no immutability guard | UNTESTED |
| platform-bootstrap: Immutable and Moving Image Tags | latest always moves | `deploy.yml`, never executed | UNTESTED |
| platform-bootstrap: Resource and Container Hardening Budget | Measured on the deployed server | Phase 14 | DEFERRED |
| platform-bootstrap: Resource and Container Hardening Budget | Portainer and docker.sock verified manually | Phase 14 | DEFERRED |
| service-health: Readiness Endpoint | Database unreachable | `TestPostgresCheck_RealContainer` proves the check errors; the handler's error→503 mapping is never asserted over HTTP | PARTIAL |
| user-profile: GET /v1/me Response Shape | Superadmin caller | no test asserts `is_superadmin: true` from `GET /v1/me` (task 5.25 claims both) | UNTESTED |
| user-profile: GET /v1/me Performance Budget | p95 measured on deployed server | Phase 14 | DEFERRED |

### Load-Bearing Security Mechanisms — verified in code, not just design

| Mechanism | Real? | Evidence |
|---|---|---|
| Narrowed default privileges | YES | `migrations/bootstrap/00001_roles.go:95` — `ALTER DEFAULT PRIVILEGES FOR ROLE vecingest_owner GRANT SELECT, INSERT ON TABLES TO app_rw` |
| `ddl_command_end` event trigger | YES | `00001_roles.go:106-108`, `WHEN TAG IN ('CREATE TABLE','CREATE TABLE AS','ALTER TABLE')` |
| Bidirectional partition closure | YES | `guard.go` — upward `pg_partition_ancestors`, downward `pg_partition_tree` from `pg_partition_root`, `SECURITY INVOKER` |
| Two non-droppable partition-guard RED tests | YES | `test/privilege_matrix_test.go` `PartitionOfGuardCoversNewChildWithNoExplicitRevoke` (+24 months) and `AttachPartitionGuardCoversNewChildWithNoExplicitRevoke` (+25 months) — both outside the 12 pre-revoked partitions, no explicit revoke in either test |
| Audit hash chain computed in Go | YES | `internal/domain/audit/hash.go` — SHA-256 over `prev ‖ F(field)…`, 4-byte BE length prefixes, distinct 0xFF null marker, canonical JSON; `TestComputeHash_FieldBoundaryShiftChangesHash`, `TestComputeHash_NullAndEmptyStringDifferently`, `TestAppend_HostileOwnerUpdateBreaksTheChainAtThatRow` |
| TOTP replay under READ COMMITTED | YES | `UpdateUserMFALastTOTPStep :execrows` with `last_totp_step IS NULL OR last_totp_step < $2`; `verify.go` maps 0 rows → `OutcomeReplayed` and classifies `pgerrcode.SerializationFailure`; `TestVerifyTOTP_ConcurrentVerificationsExactlyOneWins` |
| Trusted-proxy middleware ORDER | YES | `router.Steps` returns the named chain; `TestSteps_OrderMatchesD_H` asserts `client-ip` precedes `logger` and `limiter` |
| Trusted-proxy RESOLUTION | **NO** | see CRITICAL-3 |
| CSRF token in body, never cookie | YES | `dto.LoginResponse.CSRFToken` json body field; only cookie set anywhere is `vecingest_refresh`; `TestAuthFlow_MobileLoginGetsBodyRefreshToken` asserts native transport gets no `csrf_token` |
| `GET /v1/auth/refresh/csrf` under the cookie path | YES | registered at `/v1/auth/refresh/csrf`, a subpath of `RefreshCookiePath = /v1/auth/refresh`; safe method, no rotation, 401 on missing/expired/revoked; `Cache-Control: no-store`, `Vary: Origin`, `Cross-Origin-Resource-Policy: same-origin` all asserted at `api_integration_test.go:258-265` |

### Documented Deviations — judged

| Deviation | Verdict | Reasoning |
|---|---|---|
| `append_only_relations.relname` is `text`, not `regclass` | **SOUND** | The registry is seeded with `'audit_log'` by the bootstrap set, before the schema set creates that table; `regclass` would fail at seed time. `to_regclass()` resolves lazily at guard-invocation time and yields NULL (never matching) for a not-yet-existing relation. Documented in `guard.go:18-23`. |
| River runs a placeholder worker kind (v0.47.0 has no leadership-only mode) | **SOUND** | v0.47.0's `Config.validate` rejects `Queues` without `Workers`, and `Client.Start` refuses unless `willExecuteJobs()`. The meaningful invariant — zero producers — is asserted directly (`river_job` empty after the full migration run, `test/privilege_matrix_test.go`). Nothing calls `Insert`/`InsertMany` anywhere. |
| `health --worker` reads `river_leader.expires_at` | **SOUND** | v0.47.0 has no `river_client` table. With exactly one worker replica at M0, an unexpired leader lease *is* this worker's heartbeat. Proven end-to-end against the real built binary: exit 0 while running, non-zero after the DB stops (`worker_test.go:143,172`). Revisit when a second replica exists. |
| `vecingest_owner` NOLOGIN, schema migrations use `SET ROLE` | **SOUND** | A NOLOGIN owner cannot be connected as, so the superuser connection `SET ROLE vecingest_owner` is the only way to make the objects owned by the right role. Applied as the first statement of both schema migrations with a matching `RESET ROLE`. The spec's own scenario ("roles exist as distinct roles after bootstrap") is satisfied and tested. |
| Static Zod projection is `min(12)`, conditional 15/12 stays server-side | **SOUND** | A static schema cannot express a rule conditioned on server-side TOTP state. 12 is the *lower* of the two floors, so the client never rejects a legitimate 12-char-with-TOTP password and never silently accepts something the server would reject without an error — a 14-char no-TOTP password reaches the server and gets `AUTH_PASSWORD_TOO_SHORT_NO_MFA`. Fails safe in the right direction. |
| `internal/platform/mail` is a log-only stand-in | **GAP DRESSED AS A DECISION** | see CRITICAL-5 |
| `security.yml` gosec job uses `golangci-lint --enable-only=gosec` instead of the standalone binary | **SOUND** | Empirically necessary: the standalone binary does not honour `//nolint:gosec` and would re-flag four already-reviewed exceptions on every PR. Same SARIF, same blocking exit code. |

### Correctness (Static Evidence) — where the implementation is genuinely right

Stated plainly, because it is: the auth core of this change is correct and
well-tested. Argon2id (pinned params, PHC round-trip, constant-time verify,
`needsRehash`, dummy-hash path for unknown emails), the HIBP k-anonymity client
including `Add-Padding: true` and zero-count discard, the CSRF mint/verify bound
to `family_id`+`session_id`+`exp` with all five rejection cases, refresh rotation
with family invalidation on reuse, transport mutual exclusion, TOTP with the
never-break-early highest-matching-step loop, recovery codes with constant-time
comparison over all entries, the shared `auth:fail:totp:<user_id>` throttle
budget, `slog` PII redaction across attribute/message/nested-group/`LogValuer`,
the three-role provisioning with the `app_rw`-not-a-member assertion, the four
migration-time invariants I1-I4 each proven against a planted violation, and the
concurrent gap-free audit chain across a partition boundary — all of these are
real, all are covered by tests that passed at runtime in this verification.
`make gen` is byte-stable. The Expo dark-mode test computes real WCAG relative
luminance rather than asserting a colour constant.

### Coherence (Design)

| Decision | Followed? | Notes |
|---|---|---|
| D-A embedded goose + advisory lock | YES | pinned lock ids 5432001/5432002, `TestNewLockedProvider_ConcurrentCallersConverge` |
| D-B append-only, three mechanisms | YES | all three present and tested |
| D-D/D-E tokens, CSRF, cookie scope | YES | |
| D-F password policy + HIBP fail-open | YES | residual risk recorded in threat-model.md |
| D-G Argon2id | YES | |
| D-H middleware chain, step order | ORDER YES, STEP 2 NO | see CRITICAL-3 |
| D-I per-subcommand config sets | YES | |
| D-J slog redaction | HANDLER YES, CALL-SITE RULE NO | rule file exists but is never run (CRITICAL-4) |
| D-K Dockerfiles/compose | YES | `make lint-compose` OK |
| D-L River pinned + grants | YES (documented deviation) | |
| D-M ci.yml | INTERNALLY CONTRADICTORY | see CRITICAL-1, CRITICAL-2 |
| D-N lockout + `internal/mail` | LOCKOUT YES, MAIL NO | design.md file table line 473 mandates `api/internal/mail/` with a go-mail SMTP sender, bounded async dispatch pool and two `html/template` templates. None of it exists. |
| D-O audit chain single-writer | CHAIN YES, ENFORCEMENT NO | `single-writer-audit-log.yml` is never executed (CRITICAL-4) |
| D-P TOTP | YES | |
| D-Q readiness registry | YES | |
| D-R per-role handles | YES | |
| D-S CLI subcommands | YES | |
| design.md:646 "trusted-proxy resolution table incl. spoofed leftmost XFF **from an untrusted source**" | NO | that table row does not exist |

### TDD Compliance

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | PARTIAL | `sdd/m0-foundation/apply-progress` (revision 8) carries no "TDD Cycle Evidence" table; it declares Mode: Standard for Phases 11-13 (correct per tasks.md, which marks those "verified by inspection") and defers Phases 0-10's table to "the prior batch's content", which the current revision no longer contains. |
| All tasks have tests | YES for behavioural tasks | Every `RED:` task in Phases 1-6 and 10 maps to an existing test file. |
| RED confirmed (test files exist) | YES | 180 Go test functions across 47 test files + 4 jest suites (10 tests). |
| GREEN confirmed (tests pass now) | YES | full suite exit 0, re-run with `-count=1`. |
| Triangulation adequate | YES | e.g. password policy has 4 length cases, HIBP 5 cases, CSRF 6 rejection cases, TOTP 5 step cases, revocation cache 5 cases. |
| Safety net for modified files | NOT VERIFIABLE | no per-task record in the retrievable artifact. |

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit (Go) | ~140 | ~35 | stdlib `testing`, `-race` |
| Integration (Go, Testcontainers Postgres 17) | ~40 | 12 | testcontainers-go, real `app_rw` connections, real built binaries |
| Component (RN) | 10 | 4 | @testing-library/react-native, jest |
| E2E (browser/device) | 0 | 0 | not installed (Maestro is post-M0 per config.yaml) |
| **Total** | **~190** | **~51** | |

### Changed File Coverage (`internal/domain/...`, threshold 80%)

| Package | Full-suite | `-short` only |
|---|---|---|
| `internal/domain/pii` | 100.0% | 100.0% |
| `internal/domain/auth/token` | 90.9% | 90.9% |
| `internal/domain/auth/password` | 89.2% | 89.2% |
| `internal/domain/auth/lockout` | 83.8% | 83.8% |
| `internal/domain/audit` | 82.8% | 56.3% |
| `internal/domain/auth/mfa` | 82.7% | 56.1% |
| `internal/domain/auth/session` | 80.7% | 12.3% |
| **total** | **85.1% — above gate** | **66.0% — below gate** |

### Assertion Quality

No tautologies, no ghost loops, no assertion-without-production-code, no
mock-heavy files (max 1 `jest.mock` per file, 0 in three of four suites). The
`toBeTruthy()` calls in the RN suites all follow `getByRole`/`getByTestId`
queries, which already throw on absence, and every one of them sits alongside a
real value assertion (`toHaveStyle`, computed contrast ratio, `toHaveBeenCalledWith`).

| File | Line | Assertion | Issue | Severity |
|---|---|---|---|---|
| `app/app/(auth)/login.test.tsx` | 12-15 | single `getByRole(...).toBeTruthy()` | smoke-test-only; no behavioural assertion | SUGGESTION |
| `app/src/screens/LoginScreen.test.tsx` | 47-55 | regex over `LoginScreen.tsx` source | implementation-detail coupling — but the spec MUST ("MUST NOT declare a hand-written validation schema") is only checkable structurally | SUGGESTION |

**Assertion quality**: 0 CRITICAL, 0 WARNING, 2 SUGGESTION.

### Quality Metrics

**Linter**: `~/go/bin/golangci-lint run ./...` from `api/` → 0 issues.
**Formatter**: `gofumpt -l .` → clean.
**Type checker**: `pnpm turbo run typecheck` → 9/9 tasks pass.
**Custom Semgrep rules**: 0 findings when run manually — but never run by CI (CRITICAL-4).

### Issues Found

**CRITICAL**

1. **`make lint-scope` is broken; `api/cmd/lintscope` does not exist.**
   `Makefile:51` runs `cd api && go run ./cmd/lintscope ./internal/db/queries`.
   That directory is absent (`api/cmd/` contains only `lintcompose`,
   `openapi-gen`, `tzcheck`, `vecingest`). Observed:
   `stat .../api/cmd/lintscope: directory not found; make: *** [Makefile:51: lint-scope] Error 1`.
   `ci.yml`'s entire `scope` job is `run: make lint-scope`, so that job fails on
   the first CI execution. Tasks 0.3 and 11.7 are marked `[x]`.
   `openspec/config.yaml` states the tenant-column rule "is enforced by
   `make lint-scope`"; it currently enforces nothing.

2. **`ci.yml`'s `go` job fails its own coverage gate, deterministically.**
   The job stops the Docker daemon (a deliberate, correct design choice) and
   then runs
   `go test -race -short -covermode=atomic -coverprofile=coverage.out ./internal/domain/...`
   followed by a hard `< 80` fail. Measured: **66.0%**. Without `-short` — the
   form `openspec/config.yaml` actually declares
   (`go test -race -cover ./internal/domain/...`) — it is 85.1%. The `-short`
   flag was added so the step would run without Docker, but that removes exactly
   the Testcontainers tests that carry `session` (80.7% → 12.3%), `audit`
   (82.8% → 56.3%) and `mfa` (82.7% → 56.1%). The two goals are mutually
   exclusive inside one job. Tasks 11.2 and 11.3 are marked `[x]`.

3. **Trusted-proxy client IP resolution does not reject an untrusted peer —
   PRD §10.1 gate item 10 is marked green on a test that does not exist.**
   `request-protection` requires: "GIVEN a request whose immediate peer IP is
   not on the trusted-proxy allowlist … THEN the system uses the peer's actual
   connection IP, not a claimed `X-Forwarded-For` value."
   `router.clientIPMiddleware` installs `chi middleware.ClientIPFromXFF`, which
   **never reads `r.RemoteAddr`** (chi v5.3.2 `middleware/client_ip.go:92-117`;
   chi's own sibling doc says so outright: "This middleware reads ONLY
   X-Forwarded-For; it does not inspect r.RemoteAddr"). Reproduced against the
   pinned chi version with the production configuration (trusted proxy
   `172.18.0.2/32`):

   ```text
   peer(RemoteAddr) = 198.51.100.7      # NOT the trusted proxy
   claimed XFF      = 1.2.3.4
   resolved ClientIP= "1.2.3.4"
   RESULT: SPOOFED — the untrusted peer's claimed XFF value was accepted.
   ```

   That resolved value is the key for `limiter.PerIP()` (60/min),
   `limiter.LoginReset()` (10/min) and the `auth:fail:ip:*` lockout counter, so
   any caller that can reach the API directly defeats all three by rotating a
   header. The only existing test,
   `TestClientIPStep_SpoofedLeftmostEntryIgnored`, sets
   `RemoteAddr = "172.18.0.2:12345"` — the *trusted* proxy — so it exercises the
   other scenario. design.md:646 explicitly called for the untrusted-source row;
   it was never written. `docs/security/gates/M0.md` row 10 cites "the
   spoofed-`X-Forwarded-For`-from-an-untrusted-source table test passes" — that
   test does not exist. Network-layer isolation may mitigate this in the target
   deployment, but the spec makes it an application-level MUST and the gate
   claims it as tested.

4. **The three custom Semgrep rules are never executed by anything.**
   `api/.semgrep/{no-float-money,slog-pii-call-site,single-writer-audit-log}.yml`
   exist and are well written (0 findings when run manually), but
   `security.yml`'s semgrep job runs only
   `--config p/owasp-top-ten --config p/golang --config p/typescript`, and
   `make lint` never invokes semgrep at all. Consequences: D-O's single-writer
   audit invariant (task 3.16: "a Semgrep rule fails if any caller other than
   `audit.Append` invokes `InsertAuditLog`") blocks nothing — a direct
   `InsertAuditLog` call would silently break the hash chain; the
   `double precision`/`real` ban in `migrations/**.sql` (task 3.14) is
   unenforced; D-J's call-site rule (task 5.8) is unenforced. Tasks 3.14, 3.16
   and 5.8 are marked `[x]`. Fix is one `--config api/.semgrep` flag.

5. **`internal/mail` was never built; a spec MUST is unmet and `serve` demands
   SMTP credentials it never uses.**
   `auth-credentials: Progressive Lockout` states the system "MUST send an email
   alert to the affected user", and its scenario ends "…and has sent a lockout
   alert email". design.md's file table (line 473) mandates
   `api/internal/mail/` — "`Mailer` interface + go-mail SMTP sender + bounded
   async dispatch pool + `html/template` templates `password_reset.html` and
   `login_lockout.html` (D-N)" — and D-N (line 279) specifies the bounded pool
   registered in `cmd/vecingest`. **tasks.md contains no task to build any of
   it**; task 0.7 only creates the empty directory. What exists is
   `api/internal/platform/mail.LogMailer`, whose own doc comment concedes: "It
   MUST be replaced before M0 ships to production: PRD §5.1's lockout alert
   email and forgot-password email currently only reach the log."
   `cmd/vecingest/serve.go:167` wires `mail.LogMailer{}` while
   `internal/config` still *requires* `SMTP_URL` and `MAIL_FROM` for `serve`
   (`config.go:54-55`, `deploy/.env.example:45-46`). This is not a deviation
   recorded against a design decision — it is a design-mandated component with
   no implementing task, and an unmet spec MUST.

6. **Task 13.2 ("Run the full `ci.yml` + `security.yml` pipeline green") is
   marked `[x]` but no pipeline run exists, and two `ci.yml` jobs fail as
   written.** `docs/security/gates/M0.md` is honest about `security.yml` never
   having executed on GitHub, and about the 11.11 probe PR substitution. It is
   silent about `ci.yml` never having executed. Given CRITICAL-1 and
   CRITICAL-2, the first real run will be red in the `go` and `scope` jobs.

**WARNING**

1. `docs/security/gates/M0.md` rows 2, 3 and 4 attribute their evidence to "the
   `test/` e2e package" (refresh rotation/reuse, CSRF/Origin cases, superadmin
   route). `api/test/` contains only `privilege_matrix_test.go`,
   `docker_tzcheck_test.go` and `helpers_test.go`; `grep -rn "Origin\|CSRF"
   test/` returns nothing. Those tests are real and pass — they live in
   `internal/http/api/api_integration_test.go` (lines 224-265, 285-345,
   411-441). The evidence trail points at the wrong package, which is exactly
   the failure mode PRD §10's "evidencia, no una afirmación" rule guards against.
2. `user-profile: GET /v1/me Response Shape` superadmin scenario has no covering
   test; task 5.25 claims "for both a regular user and a superadmin".
   `is_superadmin` is asserted only as `false` (`api_integration_test.go:217`).
3. `Bearer-Authenticated Logout` "revokes the whole family" is proven only
   against a family containing a single session; "cookie cleared regardless of
   request path" is asserted only on the cookie *builder*, never on the logout
   response.
4. `service-health: Readiness` failure path is proven at the registry/check
   level, never as an HTTP status. The handler mapping (`ReadyHandler.Ready` →
   `apperr.New(503, …)`) is four obviously-correct lines, so the risk is low.
5. `platform-bootstrap: Immutable and Moving Image Tags` has no enforcement
   mechanism. `deploy.yml` pushes `sha-<short>`/`latest`/`v<semver>` but nothing
   prevents an overwrite; re-running a tag build would republish `v<semver>`.
   GHCR does not make tags immutable by default.
6. `app/`, `packages/shared/` and `site/` `lint` scripts are stubs
   (`echo … && exit 0`). `ci.yml`'s `ts` job runs `pnpm turbo run lint typecheck
   test`, so its lint leg is vacuous today. The wiring is correct and will start
   enforcing the moment real ESLint configs land; the apply record flags this.
7. The retrievable `apply-progress` (revision 8) contains no TDD Cycle Evidence
   table for Phases 0-10; it points at a prior revision's content. Strict-TDD
   evidence had to be reconstructed from the codebase. It reconstructs cleanly,
   but the artifact is lossy.
8. Stale risk entry: `TestVerifyAccess_RejectsTamperedSignature` is no longer
   flaky. It now flips the *first* base64url character of the signature (all 6
   bits significant) instead of the last. 300 consecutive `-race -count=300`
   runs passed. The apply record still lists it as ~1/64 flaky.
9. Task 11.11 (open a deliberately-failing probe PR) is marked `[x]` but was not
   literally performed. The substituted evidence — a real reachable
   `GO-2026-6253` in `moby/go-archive@v0.2.0` caught by `govulncheck` and fixed
   to v0.3.0 — is genuinely stronger, and the substitution is documented in the
   gate file. Recorded for honesty, not as a defect.
10. Task 11.8's literal "gosec" bullet is satisfied by
    `golangci-lint --enable-only=gosec`. Correct decision, documented, noted
    only because the task text and the implementation differ.

**SUGGESTION**

1. `guard.go:64` gates the revoke on
   `has_table_privilege('app_rw', child.relid, 'UPDATE')`. A partition that
   somehow held `DELETE` or `TRUNCATE` without `UPDATE` would escape the revoke.
   Under the fail-closed default-privilege baseline this cannot arise today;
   widening the predicate to any of UPDATE/DELETE/TRUNCATE would close it by
   construction.
2. `api/.golangci.bck.yml` is a stray backup file left in the repository.
3. `make lint` cannot run on the current machine: it invokes bare `gofumpt` and
   `golangci-lint`, which live in `~/go/bin` and are not on `PATH`
   (`make lint` → `/bin/sh: 1: gofumpt: not found`, exit 127). CI installs them
   itself, so CI is unaffected, but the declared "single workspace entrypoint"
   does not work as documented.
4. `docs/security/gates/M0.md` row 3 could name the exact subtests instead of a
   package (and, per WARNING-1, must name the right package).
5. The generated Zod `password: z.string().min(12)` also applies to *login*, so
   a user whose password predates the policy could never submit the form. No
   such user can exist at M0.
6. `app/app/(auth)/login.test.tsx` is a single-assertion smoke test; the real
   coverage is in `src/screens/LoginScreen*.test.tsx`.

### Verdict

**FAIL** — the implemented auth core is genuinely correct and well-tested (77/92
scenarios COMPLIANT, full suite and e2e green, `make gen` byte-stable, the
`-short` Docker-free contract empirically real), but one spec scenario is
contradicted by the running code (untrusted-peer XFF spoofing, PRD §10.1 gate
item 10), two `ci.yml` jobs fail deterministically as written, three custom
Semgrep invariants are never executed, and a design-mandated component
(`internal/mail`) has no implementing task while a spec MUST depends on it.
Checkpoint B is correctly left open; that is not what fails this verification.
