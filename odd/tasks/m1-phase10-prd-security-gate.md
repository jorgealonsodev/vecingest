# M1 Phase 10 — PRD Corrections, Security Gate, Checkpoint A Closure

Work unit WU-8 (PR8). Document-discipline phase: no production behavior changes.
Source of truth: `openspec/changes/m1-communities/tasks.md` lines 278-286.

Branch: `feature/m1-communities-pr7-app-portal` (phase 9 closed at `5d55abc`).

## Tasks

- [x] 10.1 Correct `PRD_go.md` §7.7 (Spanish): replace the "handler sin ella no compila" wording
      with the design D-1/D-2 text — typed `scoped.Community` / `scoped.Office` / `scoped.Self`
      constructors not assignable to `huma.Register`; `serve` startup assertions A1/A2/A3;
      A3 required because `Hidden: true` operations route but never reach `api.OpenAPI()`.
- [x] 10.2 Correct `PRD_go.md` §7.3 (Spanish): document `invitations.status` as an explicit column,
      the deliberate deviation from a timestamp-derived state.
- [x] 10.3 Modify `openspec/config.yaml`: add the permission-matrix requirement and the
      tenant-scope disclosure rule per the design binding.
- [x] 10.4 Create `docs/security/gates/M1.md`: Checkpoint A/B split per §10.1, including the
      scoped exception row for deferred Turnstile coverage on `register-company` / contact forms.
- [x] 10.5 Archive Checkpoint A evidence: `go test -race ./...`, `pnpm --filter app test`,
      permission-matrix 100% coverage, `make lint-scope` green — each gate item linked to its test.
- [x] 10.6 Verify by inspection: `make gen && git diff --exit-code` clean across `openapi.yaml`,
      sqlc code, TS client, Zod schemas for the whole M1 struct set.
- [x] 10.7 Cross-check: every Checkpoint A row in `docs/security/gates/M1.md` cites a passing test
      or a green workflow run, never a bare assertion.

## Test-first applicability

Not applicable. Every task edits documentation or declarative configuration with no runnable
deterministic test and no meaningful RED. Verification is the real command evidence collected in
10.5 / 10.6 plus structural inspection in 10.7.

## Evidence

Collected on a clean tree before any phase-10 write:

| Check | Exit | Result |
|---|---|---|
| `make gen` then `git diff --exit-code` | 0 / 0 | no diff in `openapi.yaml`, sqlc code, TS client, Zod schemas (10.6) |
| `make lint-scope` | 0 | `lintscope: OK — every query against a tenant-scoped table references its tenant column` |
| `cd api && go test -race ./... -count=1` | 0 | zero FAIL lines; `internal/http/api` ok 310.3s |
| `pnpm --filter app test` | 0 | 13 suites, 56 tests |
| `go test -race -count=1 -run 'TestPermissionMatrix' ./internal/http/api/...` | 0 | 3/3 PASS; `34/34 documented operations covered (100%)`, 180 assertions |

## Outcome

`docs/security/gates/M1.md` (197 lines) splits the seven PRD §10.1 M1 items into Checkpoint A
(items 1, 2, 3, 6, 7) and Checkpoint B (items 4, 5), with a scoped exception row for the deferred
Turnstile coverage on `register-company` / contact forms, which M1 does not ship and a passing
test asserts absent.

**Checkpoint A is recorded OPEN, not closed.** CI never runs the database-backed tests
(`newTestServer` skips under `-short`, `api/internal/http/api/api_integration_test.go:68`; the CI
`go` job runs `go test -race -short -count=1 ./...` with Docker stopped, `.github/workflows/ci.yml:138`;
`coverage` covers only `./internal/domain/...`, `:175`; `e2e` covers only `./test/...`, `Makefile:37`).
So `TestPermissionMatrix_ForeignResourceDenied` — the cross-tenant isolation proof — never runs on
a PR, while PRD item 1 requires exactly that. Closing criterion recorded in the gate: a CI run id
showing that test as PASS, not SKIP.

## Follow-ups raised, deliberately not done here

This phase is document-only; each of these touches code or another milestone's scope.

1. Make the database-backed `internal/http/api` tests run in a blocking CI job (blocks M1).
2. `api/internal/authz/assert.go:109` (A1 error message) and `api/internal/authz/scoped/register.go:1-2`
   (package doc) name only three of the five scoped constructors, omitting `Unit` and `Invitation`.
3. `TestInvitation_EnumerationLockoutIsIPAndDeviceScoped` no longer matches its implementation:
   `inviteLockoutKey` keys on the source address alone (`api/internal/http/handlers/invitations.go:692`)
   after the device leg was dropped in review lineage `review-0e1833930adf141a`.
4. `make lint-scope` has three query-level exceptions, not two; `SweepExpiredInvitations` is a
   cross-tenant sweep with no shape test pinning it.
5. `openspec/changes/m1-communities/tasks.md:70` gives a command (`go test ./api/test/... -run ...`)
   that cannot resolve from `api/`.
