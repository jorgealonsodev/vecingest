# Apply Progress: M1 — Communities, units, invitations, role portals

## Work Unit: WU-1 / PR1 — Phase 1: Schema + `authz` Foundation

**Status**: Phase 1 (tasks 1.1–1.25) COMPLETE. Phase 2+ NOT started (out of this
work unit's scope per the feature-branch-chain delivery strategy).

**Branch**: `feature/m1-communities-pr1-authz-schema` (base: tracker branch
`feature/m1-communities`). Not merged, not pushed — delivery is the user's
decision.

### Completed Tasks (Phase 1, 1.1–1.25)

All 25 tasks marked `[x]` in `tasks.md`.

- [x] 1.1 Scaffold `api/internal/authz/{authz.go,resolve.go,assert.go,allowlist.go,scoped/register.go,testdata/negative_register/}`
- [x] 1.2 RED (Testcontainers): `00007_m1_tenant_schema.sql` up→down→up idempotent
- [x] 1.3 GREEN: implement `00007_m1_tenant_schema.sql`
- [x] 1.4 RED (Testcontainers): `00008_invitations.sql`
- [x] 1.5 GREEN: implement `00008_invitations.sql`
- [x] 1.6 RED: sqlc queries compile check
- [x] 1.7 GREEN: write queries/*.sql; ran `sqlc generate`; committed `internal/db/*.sql.go`
- [x] 1.8 RED: negative_register — scoped handler fails to compile against `huma.Register`
- [x] 1.9 GREEN: implement `authz.go` — `Membership`, `Memberships`, `Scope`, `Role`, `CommunityScoped`/`OfficeScoped`, `MetadataKey`
- [x] 1.10 RED: zero-value Membership fails accessors; missing-scope-interface fails to compile
- [x] 1.11 GREEN: panicking accessors; `scoped/register.go` — `Community`/`Office`/`Self` stamping the marker
- [x] 1.12 RED: scoped constructor resolves membership before invoking handler
- [x] 1.13 GREEN: `resolve.go` — resolvers by route shape (D-4)
- [x] 1.14 RED: foreign resource resolves to nothing; header-supplied tenant id ignored
- [x] 1.15 GREEN: path-only resolution enforced (by construction — `ScopeCommunityID()`/`ScopeOfficeID()` only ever read the parsed path input)
- [x] 1.16 RED: wrong role on own resource rejected 403; admin scope via office_members
- [x] 1.17 GREEN: role check in `scoped.Community`/`scoped.Office`; admin-scope resolution
- [x] 1.18 RED: A1/A2 in-memory humachi boot-assertion failures
- [x] 1.19 RED: A3 Hidden-operation failure / allowlisted pass
- [x] 1.20 RED: marker survives `huma.NewGroup` prefixing
- [x] 1.21 GREEN: `assert.go` — `AssertScopedRegistration` (A1/A2/A3) + `EagerCheck` (`OnAddOperation` additive trigger)
- [x] 1.22 RED: allowlist hygiene (`PublicOperations` undefined)
- [x] 1.23 GREEN: `allowlist.go` — `PublicOperations` seeded with M0's public routes
- [x] 1.24 Modified `api/internal/http/api/api.go` (wired `authz.EagerCheck` into `OnAddOperation`) and `api/cmd/vecingest/serve.go` (calls `authz.AssertScopedRegistration` after `httpapi.New(...)`, before `ListenAndServe`)
- [x] 1.25 `go build ./...` clean; `go test -race ./internal/authz/...` — 16/16 pass, including A1's "deliberately-unmarked test route fails boot" (`TestAssertScopedRegistration_A1_UnregisteredScopedOperationBlocksBoot`)

### Files Changed

| File | Action | What Was Done |
|---|---|---|
| `api/migrations/schema/00007_m1_tenant_schema.sql` | Created | `offices`, `office_members`, `communities`, `units`, `unit_members`; explicit `GRANT UPDATE, DELETE`; D-5 indexes/uniques |
| `api/migrations/schema/00008_invitations.sql` | Created | `invitations`; `status` CHECK; unique `token_hash`/`short_code_hash` (both NOT NULL — see Deviations); community_id+status index |
| `api/internal/db/queries/{offices,office_members,communities,units,unit_members,invitations}.sql` | Created | sqlc queries, every tenant-scoped query filters/selects its tenant column |
| `api/internal/db/{offices,office_members,communities,units,unit_members,invitations}.sql.go`, `models.go`, `querier.go` | Generated | `sqlc generate` (sqlc v1.31.1, installed locally for this session) |
| `api/internal/db/m1_queries_test.go` | Created | Testcontainers integration test, full membership chain |
| `api/test/m1_tenant_schema_test.go`, `m1_schema_helpers_test.go` | Created | Migration up/down/up idempotency + grants/indexes/CHECK assertions |
| `api/internal/authz/authz.go` | Created | `Membership`, `Memberships`, `Scope`/`Kind`, `Role`, `CommunityScoped`/`OfficeScoped`, `MetadataKey`, `Marker` |
| `api/internal/authz/context.go` | Created | `ContextWithUserID`/`UserIDFromContext` — the authenticated-caller seam scoped.* reads before resolving |
| `api/internal/authz/resolve.go` | Created | `Querier` interface, `Configure`, `ResolveCommunity`/`ResolveOffice`/`ResolveSelf`, `RoleAllowed`, `ErrNoMembership` |
| `api/internal/authz/assert.go` | Created | `AssertScopedRegistration` (A1/A2/A3), `EagerCheck` (OnAddOperation additive trigger) |
| `api/internal/authz/allowlist.go` | Created | `Entry`/`Allowlist`/`Contains`, `PublicOperations` |
| `api/internal/authz/scoped/register.go` | Created | `Community[I,O,PI]`, `Office[I,O,PI]`, `Self[I,O]` |
| `api/internal/authz/testdata/negative_register/*.go` | Created | Two non-compiling fixtures (rung-1 proof) |
| `api/internal/authz/*_test.go` | Created | 20 tests total across `authz` + `authz/scoped` packages |
| `api/internal/http/api/api.go` | Modified | Wired `authz.EagerCheck` into `humaConfig.OnAddOperation` |
| `api/cmd/vecingest/serve.go` | Modified | Captured `hapi` from `httpapi.New`; calls `authz.AssertScopedRegistration` before binding the listener |

### TDD Cycle Evidence

| Task | Test File | Layer | RED | GREEN | TRIANGULATE |
|---|---|---|---|---|---|
| 1.2/1.3 | `test/m1_tenant_schema_test.go` | Integration (Testcontainers) | ✅ (relation "offices" does not exist) | ✅ | ➖ single migration |
| 1.4/1.5 | `test/m1_tenant_schema_test.go` | Integration (Testcontainers) | ✅ (relation "invitations" does not exist) | ✅ | ➖ single migration |
| 1.6/1.7 | `internal/db/m1_queries_test.go` | Integration (Testcontainers) | ✅ (undefined: q.InsertOffice) | ✅ | ✅ full membership chain + a negative (no unit_members row) case |
| 1.8/1.9 | `internal/authz/negative_register_test.go` | Compile-fail (go build subprocess) | ✅ (undefined: authz.Membership) | ✅ | ➖ structural |
| 1.10/1.11 | `internal/authz/membership_test.go` + `negative_register_test.go` | Unit + compile-fail | ✅ (m.UserID undefined) | ✅ | ✅ zero-value (4 accessors) + valid-membership case |
| 1.12/1.13 | `internal/authz/scoped/register_test.go` | Integration (in-memory humachi) | ✅ (undefined: authz.Configure) | ✅ | ✅ 5 scenarios (resolve, foreign, header-ignored, wrong-role, admin-via-office) |
| 1.14/1.15 | `internal/authz/scoped/register_test.go` | Integration (in-memory humachi) | ✅ (same RED as above) | ✅ | ✅ |
| 1.16/1.17 | `internal/authz/scoped/register_test.go` | Integration (in-memory humachi) | ✅ (same RED as above) | ✅ | ✅ |
| 1.18–1.20 | `internal/authz/assert_test.go` | Integration (in-memory humachi) | ✅ (undefined: AssertScopedRegistration) | ✅ | ✅ 5 scenarios (A1, A2, A3-fail, A3-pass, group-prefix) |
| 1.21 (EagerCheck) | `internal/authz/eager_check_test.go` | Unit | ✅ (undefined: EagerCheck) | ✅ | ✅ panic case + 2 no-panic cases |
| 1.22/1.23 | `internal/authz/allowlist_test.go` | Unit | ✅ (undefined: PublicOperations) | ✅ | ✅ 2 hygiene rules |

### Work Unit Evidence

| Evidence | Value |
|---|---|
| Focused test command and exact result | `cd api && go test -race ./internal/authz/...` → **16/16 pass** (`internal/authz` 11, `internal/authz/scoped` 5) |
| Runtime harness command/scenario and exact result | `go test ./cmd/vecingest/... -run TestRunServe` → **pass** (real `serve` boot path, with `AssertScopedRegistration` wired in, against a real Testcontainers Postgres) |
| Rollback boundary | Revert `api/internal/authz/`, `api/internal/db/{offices,office_members,communities,units,unit_members,invitations}.sql.go`, `api/internal/db/queries/{offices,office_members,communities,units,unit_members,invitations}.sql`, `api/test/m1_*schema*_test.go`, `api/internal/db/m1_queries_test.go`; `goose down` both migrations (00008 then 00007); revert the two-line changes in `api.go`/`serve.go` |

### Additional verification run
- `cd api && go test -race ./...` → **291/291 pass** (full repo, all packages)
- `gofumpt -l .` → clean
- `golangci-lint run ./internal/authz/... ./internal/http/api/... ./cmd/vecingest/... ./internal/db/... ./migrations/... ./test/...` → 0 issues
- `gosec ./internal/authz/... ./cmd/vecingest/...` → 0 new issues (pre-existing nolint'd finding in `seed.go` unrelated to this change)
- `make gen` equivalent (`go run ./cmd/openapi-gen`) → `openapi/openapi.yaml` unchanged (no new DTOs in Phase 1)

### Deviations from Design

1. **`invitations.short_code_hash` is `NOT NULL`, not nullable.** §7.3's original sketch marks it nullable, but design D-6 states both `token_hash` and `short_code_hash` are generated together at creation ("both unique") and the sequence diagram never creates one without the other. Made both `NOT NULL UNIQUE` for schema-level correctness. Documented inline in the migration.
2. **`GetCommunityByID` uses an explicit column list, not `SELECT *`.** A bare `SELECT *` lookup by the community's own id (itself the tenant root, D-5) doesn't literally reference the word `office_id` in its SQL text, which fails the existing M0 `lint-scope` tool's naive text-match check. Explicit column list keeps the identical result shape while including `office_id` textually. Documented inline in the query file.
3. **`EagerCheck` (the D-2/V4 additive `OnAddOperation` trigger) was added beyond the RED tasks explicitly listed for 1.18–1.20**, because task 1.21's GREEN description names it ("register `OnAddOperation` as an additive eager trigger, never the sole check"). Implemented with its own RED→GREEN cycle (`eager_check_test.go`) and wired into `api.go`.
4. **The `{unitId}`/`{invitationId}` → community_id indirect-resolution path (D-4's second and third table rows) is NOT implemented in Phase 1.** No Phase 1 task or test exercises it (Phase 1 has zero real handlers — only the mechanism and test fixtures). `scoped.Community` currently resolves directly via `PI.ScopeCommunityID()` returning the community's own id (the direct `{communityId}` case). Phases 4/6 (unit-member and invitation routes keyed by `{unitId}`/`{invitationId}`) will need to extend the resolution path — e.g. by having those DTOs' `ScopeCommunityID()` return an id that a widened resolver first translates via `units.community_id`/`invitations.community_id` (queries `GetUnitCommunityID`/`GetInvitationCommunityID` are already written and available for that). Flagging this explicitly so Phase 4/6's `sdd-apply` run does not treat it as a regression.
5. **Caller-identity plumbing (`authz.ContextWithUserID`) is not yet wired into real Bearer-auth middleware.** Phase 1 registers no real HTTP handlers, so there is nothing yet to wire. The mechanism (context key + `UserIDFromContext`) is in place and covered by tests using it directly; Phase 2 (first real scoped handlers) must call `authz.ContextWithUserID` from the authenticated-group middleware (mirroring the existing `bearerAuthAndRateLimit` pattern in `api.go`) before a scoped operation's wrapped handler runs.

### Review Workload / PR Boundary

- Mode: chained PR slice (`feature-branch-chain`, per tasks.md's own Review Workload Forecast — this PR was already forecast `400-line budget risk: High`, `Chained PRs recommended: Yes`).
- Current work unit: WU-1 (Phase 1, tasks 1.1–1.25) — schema + `authz` package + boot wiring.
- Boundary: starts from the tracker branch `feature/m1-communities` (clean M0 baseline); ends with a fully green `go test -race ./...` including the wired boot assertion.
- Authored line count (excluding sqlc-generated `*.sql.go`/`models.go`/`querier.go`): ~2097 lines. This is the smallest cohesive slice for this work unit (schema + the tenant-isolation mechanism cannot be meaningfully split further without breaking the design's own "resolvers exist only if the schema they query already does" ordering) and matches the size the tasks artifact itself already forecast for PR1 — no additional `size:exception` decision needed beyond the already-recorded chain strategy.

### Status

25/25 Phase 1 tasks complete. Ready for `sdd-verify`. Phase 2 (WU-2, PR2) not started.
