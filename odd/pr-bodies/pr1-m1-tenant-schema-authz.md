> Chained pull request 2 of 9 (slice 1). Base branch:
> `docs/m1-openspec-artifacts`. It must not be merged before its base.

## Summary

Adds the M1 tenant schema and the typed authorization infrastructure every
later M1 route is built on. No HTTP route is added.

- Tenant schema: `offices`, `office_members`, `communities`, `units` and
  `unit_members` (migration `00007`), each carrying its tenant column
  (`office_id` or `community_id`), plus the `invitations` table (migration
  `00008`). The sqlc queries and generated code for those tables are included.
- Typed authorization (`api/internal/authz`): membership resolution,
  `scoped.*` registration markers, a public-operation allowlist, an eager
  check hooked into huma's `OnAddOperation`, and a fail-closed boot assertion
  (`authz.AssertScopedRegistration`) that `serve` runs before listening, so an
  operation registered without a scope or an allowlist entry aborts startup.
- `00008_invitations.sql` precedes its feature: the invitation endpoints
  arrive in slice 4 (`feature/m1-invitations`). This slice ships the table and
  its queries only; it does not ship invitations.

- New OpenAPI `operationId`s: none. Typed authorization infrastructure plus
  tenant schema.
- Schema migrations: `api/migrations/schema/00007_m1_tenant_schema.sql` and
  `api/migrations/schema/00008_invitations.sql`.
- Added test assertions on forbidden/not-found status: 3 / 2.

### Commits

- `ec9a492` feat(m1-communities): add tenant schema and authz enforcement (PR1/8)
- `a47eadd` docs(openspec): records, in the OpenSpec tasks, the review-size
  exception approved for this slice

### Changes

| Area | Files | Lines |
|---|---|---|
| `api/internal/authz/` (allowlist, assert, resolve, context, `scoped/register.go`) | 6 | +703 |
| `api/internal/db/` (queries + sqlc generated code) | 14 | +1097 |
| `api/migrations/schema/` (`00007`, `00008`) | 2 | +170 |
| `api/cmd/vecingest/serve.go`, `api/internal/http/api/api.go` (boot assertion and eager check wiring) | 2 | +18/-1 |
| Tests (`authz` unit and negative-registration tests, `db/m1_queries_test.go`, `api/test/m1_*`) | 11 | +1043 |
| `openspec/changes/m1-communities/` (`apply-progress.md`, `tasks.md`) | 2 | +163/-25 |

### Size

37 files, +3194/-26 (`git diff --shortstat 1755844...a47eadd`). This exceeds
the 400-line review budget; shrinking it would require rewriting commits that
carry burned review authority, so the slice is delivered as recorded.

### Verification

No per-slice verification run exists for this slice. No test, linter or CI
run has been executed against `a47eadd` in isolation.

The slices are historically accurate, not independently green. Later commits
fix earlier ones: `4970237` (slice 3) fixes the bootstrap role statement and
`604f6a5` (slice 4) fixes River sequence grants. Required CI may therefore
fail on this pull request and only converge further down the chain. Any such
failure is reported on the pull request; the reviewed commits are not amended
or rebased, because they carry burned native review authority.

### CI status note

`security.yml` is already failing on `main` at `f6c79eb`, before any slice of
this chain: runs 35775418922 (`security`) and 35775419322 (`deploy`) failed on
2026-09-22 while `ci` passed. That failure is pre-existing on `main` and is not
caused by this slice.

## Definition of Done (PRD_go.md section 9)

- [ ] Input/output structs have validations; `make gen` was run and
      `openapi.yaml`, sqlc code and the TS client are regenerated and
      committed (dirty-diff gate in `ci.yml`).
  - Not checked: sqlc generated files are committed alongside the new
    queries, but no `make gen` dirty-diff check was run for this slice.
- [ ] `goose` migration reviewed (expand/contract if it touches existing
      data); `.sql` queries filter by their tenant scope column.
  - Not checked: both migrations only create new tables, so no
    expand/contract step applies, but `make lint-scope` was not run on this
    slice and no review record is cited here.
- [ ] Membership middleware present on every new route, with a test
      confirming another role/community gets 403.
  - Not applicable: no route is added. The boot assertion that enforces
    scope registration for later routes is added here, untested at this slice.
- [ ] Unit tests for the service and at least one e2e test of the main
      flow.
  - Not checked: unit tests for `authz` and schema tests are added, but none
    was run at this slice, and there is no user-facing flow yet.
- [ ] Domain events/notifications defined per the design's event table and
      enqueued with `river.InsertTx` inside the same transaction.
  - Not applicable: no event or notification in this slice.
- [ ] UI copy lives in i18n (es), not hardcoded strings; errors use stable
      codes.
  - Not applicable: no UI and no new error code in this slice.
- [ ] Accessible screen: labels, focus order, contrast — checked on iOS,
      Android and web where applicable.
  - Not applicable: no screen in this slice.
- [ ] `audit_log` entry added if the action is sensitive (money, votes,
      personal data, permissions).
  - Not applicable: no action is exposed in this slice.
- [ ] Short documentation added to the module's README and, if
      applicable, the runbook.
  - Not checked: documentation lives in OpenSpec `apply-progress.md`; no
    module README or runbook entry is added.

### Security review checklist (mandatory for every PR)

- [ ] Decoders use `DisallowUnknownFields` and `huma` validation tags.
  - Not applicable: no request decoder in this slice.
- [ ] Membership/tenant scope is checked on every new or changed endpoint.
  - Not checked: no endpoint is added, and whether wiring the boot assertion
    changes the behavior of existing endpoints was not verified by a run.
- [ ] No sensitive data appears in logs, error responses or push payloads.
  - Not checked: not verified for this slice.
- [ ] Secrets stay out of the code (verified locally; `gitleaks` in
      `security.yml` is the CI backstop).
  - Not checked: no local secret scan was run for this slice, and the
    `security.yml` backstop is failing on `main` (see CI status note).
- [x] Migration does not remove or weaken any append-only constraint.
  - Evidence: `00007` and `00008` only `CREATE TABLE` / `CREATE INDEX`;
    neither references `audit_log` or the append-only guard (`grep -ci` for
    `audit_log|append` returns 0 on both files), and their `Down` blocks drop
    only the tables they create.
- [ ] `security.yml` is green (gitleaks, govulncheck, gosec, Semgrep,
      `pnpm audit --audit-level=high`, Trivy).
  - Not checked: `security.yml` is already failing on `main` at `f6c79eb`
    (pre-existing, not caused by this slice).
- [ ] Permission-matrix test passes for every new/changed route.
  - Not checked: no route is added, and the permission-matrix test does not
    exist yet at this slice (it arrives in slice 7).

### Second-person review required?

- [x] This PR touches auth, files, money, votes or personal data → apply
      the `security-review` label and request a second reviewer.
  - Justification: adds the tenant-isolation authorization layer and tables
    holding membership and invitation personal data.
  - The `security-review` label does not currently exist in this repository.
    It has to be created, or this requirement is tracked here in the body
    text. The label has not been applied.
- [ ] None of the above applies.

## Rollback plan

Revert order: revert this slice only after every later slice of the chain that
has been merged (slices 2 to 8) has been reverted, newest first, because they
all build on this schema and on `api/internal/authz`. Revert this slice's
merge commit on the branch it landed on.

Code: the revert removes the `authz` package, the boot assertion in `serve`
and the eager check in `api.New`. No route depends on them at this slice.

Schema: `00007` and `00008` are additive (new tables only), so the preferred
rollback is code-only. Keep `00007_m1_tenant_schema.sql` and
`00008_invitations.sql` in the tree when reverting, so the versions already
recorded in `goose_db_version_schema` still match files on disk, and leave
both tables in place, unused. If the tables must be removed, note that
`vecingest migrate` (and `serve --migrate`) applies migrations up only and no
repository command runs goose down. Schema removal is therefore manual: take a
backup, roll back every later migration first in reverse version order, then
run the `-- +goose Down` block of `00008_invitations.sql` followed by that of
`00007_m1_tenant_schema.sql` as `vecingest_owner`, and delete both version
rows from `goose_db_version_schema` before removing the files. The `Down`
blocks drop the tables and destroy any office, community, unit, membership
and invitation data.
