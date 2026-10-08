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

`ci` run 37750334002 passed at `86f8edc`, this pull request's head commit.
`security` run 37750333995 at the same commit failed (see CI status note).

A passing `ci` is not evidence for any checklist item below, and it does not
make this slice merge-ready. `ci` succeeds when every job either succeeded or
was skipped by its path filter (the `ci-required` job in
`.github/workflows/ci.yml`), and the job-level results of this run are not
cited in this body.

Risk that did not occur: the slices are historically accurate, not
independently green. Later commits fix earlier ones: `4970237` (slice 3)
fixes the bootstrap role statement and `604f6a5` (slice 4) fixes River
sequence grants. This body predicted that `ci` might therefore fail on this
pull request and only converge further down the chain. The prediction did NOT
materialise: `ci` passed on every intermediate slice, this one included. The
reviewed commits were not amended or rebased, because they carry burned
native review authority.

### CI status note

`security` run 37750333995 at `86f8edc` failed. The cause is diagnosed and is
not specific to this slice: four jobs fail while `gosec` and `govulncheck`
pass.

- `gitleaks`: the blocking whole-history scan reported 7 leaks, all one test
  fixture value on seven lines of `api/internal/http/api/invitation_test.go`
  at commit `a6976ab`. That commit is not in this slice's history (it arrives
  in slice 4); the job checks out with `fetch-depth: 0` and scans the whole
  fetched repository history, not only this slice's commits.
- `pnpm audit --audit-level=high`: 20 vulnerabilities, 12 high and 1
  critical, in transitive npm dependencies.
- `semgrep`: 1 blocking finding. The genuine defect later fixed by `c2da7da`
  (a floating-point query parameter generated under `api/internal/db`) is
  introduced by `311c4fd` in slice 6 and is not in this slice's tree; which
  finding blocks at this slice was not identified separately.
- `trivy`: 9 HIGH/CRITICAL findings, all from `pnpm-lock.yaml`.

All four were diagnosed and fixed, but the fixes (`d1b71c2`, `9c8b2ee`,
`c2da7da`, `afe0b2e`) live at the tip of the chain, in pull request #9, so
this slice keeps failing: its tree predates them. This was not corrected,
because propagating the fixes to the base of the chain would require rebasing
all nine branches and destroying the native review receipts those commits
carry.

No check mechanically gates merging: `main` has no branch protection and the
repository has no rulesets, so no check is a required status. The gates are
this template and human review, and merging remains a human decision.

## Definition of Done (PRD_go.md section 9)

- [ ] Input/output structs have validations; `make gen` was run and
      `openapi.yaml`, sqlc code and the TS client are regenerated and
      committed (dirty-diff gate in `ci.yml`).
  - Not checked: `openapi.yaml`, the sqlc code and the TS client are
    committed in this diff, but no `make gen` dirty-diff result is cited for
    this slice.
- [ ] `goose` migration reviewed (expand/contract if it touches existing
      data); `.sql` queries filter by their tenant scope column.
  - Not checked: no migration in this slice; no `make lint-scope` result for
    the new queries at this slice is cited.
- [ ] Membership middleware present on every new route, with a test
      confirming another role/community gets 403.
  - Not checked: `office_test.go` and `community_test.go` add 9 forbidden
    and 2 not-found assertions, but no result for them at this slice is
    cited.
- [ ] Unit tests for the service and at least one e2e test of the main
      flow.
  - Not checked: DB-backed API tests cover both resources, but there is no
    separate service-layer unit test and no test result at this slice is
    cited.
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
    `handlers/communities.go`. No run result exercising them at this slice is
    cited.
- [ ] Short documentation added to the module's README and, if
      applicable, the runbook.
  - Not checked: documentation lives in OpenSpec `apply-progress.md`; no
    module README or runbook entry is added.

### Security review checklist (mandatory for every PR)

- [ ] Decoders use `DisallowUnknownFields` and `huma` validation tags.
  - Not checked: not verified for this slice.
- [ ] Membership/tenant scope is checked on every new or changed endpoint.
  - Not checked: scope markers and tests exist, but no run result cited at
    this slice confirms them.
- [ ] No sensitive data appears in logs, error responses or push payloads.
  - Not checked: not verified for this slice.
- [ ] Secrets stay out of the code (verified locally; `gitleaks` in
      `security.yml` is the CI backstop).
  - Not checked: no local secret scan was run for this slice, and the
    `gitleaks` backstop fails on this pull request (see CI status note).
- [ ] Migration does not remove or weaken any append-only constraint.
  - Not applicable: no migration in this slice.
- [ ] `security.yml` is green (gitleaks, govulncheck, gosec, Semgrep,
      `pnpm audit --audit-level=high`, Trivy).
  - Not checked: `security` run 37750333995 at `86f8edc` failed on four
    jobs (`gitleaks`, `pnpm audit`, `semgrep`, `trivy`); the fixes exist only
    at the chain tip, in pull request #9 (see CI status note).
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
