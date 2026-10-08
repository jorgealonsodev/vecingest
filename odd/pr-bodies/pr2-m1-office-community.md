> Chained pull request 3 of 9 (slice 2). Base branch:
> `feature/m1-tenant-schema-authz`. It must not be merged before its base.

## Summary

Adds office management and community management on top of the tenant schema
and typed authorization from slice 1: the HTTP handlers and DTOs, the sqlc
queries they need, the regenerated OpenAPI contract and TypeScript client, and
DB-backed API tests for both resources.

- New OpenAPI `operationId`s: `createOffice`, `addOfficeMember`,
  `getMyOffices`, `listMyOfficeMembers`, `createCommunity`, `getCommunity`,
  `updateCommunity`, `listMyCommunities`.
- Schema migrations: none.
- Added test assertions on forbidden/not-found status: 9 / 2.

### Commits

- `f35d85a` feat(m1-communities): add office management (PR2/8)
- `e9c582b` feat(m1-communities): add community management (Phase 3/8, WU-2/PR2)
- `86f8edc` test(m1-communities): cover the successful community PATCH

### Changes

| Area | Files | Lines |
|---|---|---|
| `api/internal/http/` (`handlers/offices.go`, `handlers/communities.go`, `dto/`, `api/api.go`) | 5 | +962/-1 |
| `api/internal/db/` (queries + sqlc generated code) | 7 | +193 |
| Generated contract (`api/openapi/openapi.yaml`, `packages/shared` client and schemas) | 3 | +1221 |
| `api/internal/authz/authz.go`, `api/cmd/vecingest/serve.go` | 2 | +17 |
| Tests (`office_test.go`, `community_test.go`, `api_integration_test.go`) | 3 | +476 |
| `openspec/changes/m1-communities/` (`apply-progress.md`, `tasks.md`, community spec) | 3 | +158/-21 |
| `go.work.sum` | 1 | +1 |

### Size

24 files, +3028/-22 (`git diff --shortstat a47eadd...86f8edc`). This exceeds
the 400-line review budget; shrinking it would require rewriting commits that
carry burned review authority, so the slice is delivered as recorded.

### Verification

No per-slice verification run exists for this slice. No test, linter or CI
run has been executed against `86f8edc` in isolation.

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
  - Not checked: `openapi.yaml`, the sqlc code and the TS client are
    committed in this diff, but no `make gen` dirty-diff check was run for
    this slice.
- [ ] `goose` migration reviewed (expand/contract if it touches existing
      data); `.sql` queries filter by their tenant scope column.
  - Not checked: no migration in this slice; `make lint-scope` was not run on
    the new queries at this slice.
- [ ] Membership middleware present on every new route, with a test
      confirming another role/community gets 403.
  - Not checked: `office_test.go` and `community_test.go` add 9 forbidden
    and 2 not-found assertions, but they were not run at this slice.
- [ ] Unit tests for the service and at least one e2e test of the main
      flow.
  - Not checked: DB-backed API tests cover both resources, but there is no
    separate service-layer unit test and nothing was run at this slice.
- [ ] Domain events/notifications defined per the design's event table and
      enqueued with `river.InsertTx` inside the same transaction.
  - Not checked: no event or notification is enqueued in this slice; whether
    the design's event table expects one for these operations was not
    verified.
- [ ] UI copy lives in i18n (es), not hardcoded strings; errors use stable
      codes.
  - Not applicable for UI copy: no UI in this slice. Error codes not verified.
- [ ] Accessible screen: labels, focus order, contrast — checked on iOS,
      Android and web where applicable.
  - Not applicable: no screen in this slice.
- [x] `audit_log` entry added if the action is sensitive (money, votes,
      personal data, permissions).
  - Evidence: every mutating operation calls `audit.Append` on the request
    transaction: `office.create` and `office_member.add` in
    `handlers/offices.go`; `community.create` and `community.update` in
    `handlers/communities.go`. Not exercised by a run at this slice.
- [ ] Short documentation added to the module's README and, if
      applicable, the runbook.
  - Not checked: documentation lives in OpenSpec `apply-progress.md`; no
    module README or runbook entry is added.

### Security review checklist (mandatory for every PR)

- [ ] Decoders use `DisallowUnknownFields` and `huma` validation tags.
  - Not checked: not verified for this slice.
- [ ] Membership/tenant scope is checked on every new or changed endpoint.
  - Not checked: scope markers and tests exist, but no run at this slice
    confirms them.
- [ ] No sensitive data appears in logs, error responses or push payloads.
  - Not checked: not verified for this slice.
- [ ] Secrets stay out of the code (verified locally; `gitleaks` in
      `security.yml` is the CI backstop).
  - Not checked: no local secret scan was run for this slice, and the
    `security.yml` backstop is failing on `main` (see CI status note).
- [ ] Migration does not remove or weaken any append-only constraint.
  - Not applicable: no migration in this slice.
- [ ] `security.yml` is green (gitleaks, govulncheck, gosec, Semgrep,
      `pnpm audit --audit-level=high`, Trivy).
  - Not checked: `security.yml` is already failing on `main` at `f6c79eb`
    (pre-existing, not caused by this slice).
- [ ] Permission-matrix test passes for every new/changed route.
  - Not checked: the permission-matrix test does not exist yet at this slice
    (it arrives in slice 7).

### Second-person review required?

- [x] This PR touches auth, files, money, votes or personal data → apply
      the `security-review` label and request a second reviewer.
  - Justification: adds tenant-scoped endpoints that manage office
    membership (permissions) and return member personal data.
  - The `security-review` label does not currently exist in this repository.
    It has to be created, or this requirement is tracked here in the body
    text. The label has not been applied.
- [ ] None of the above applies.

## Rollback plan

Revert order: revert this slice only after every later slice of the chain that
has been merged (slices 3 to 8) has been reverted, newest first, because they
build on these handlers, queries and the regenerated contract. Then revert
this slice's merge commit on the branch it landed on.

No migration is added, so no schema rollback is needed: the revert removes the
eight endpoints, their queries and their contract entries, and leaves the
slice 1 tables in place. Rows already written to `offices`, `office_members`
and `communities`, and the corresponding `audit_log` entries, remain; the
audit entries are append-only and must not be deleted.
