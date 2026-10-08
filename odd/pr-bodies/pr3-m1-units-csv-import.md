> Chained pull request 4 of 9 (slice 3). Base branch:
> `feature/m1-office-community`. It must not be merged before its base.

## Summary

Adds unit management, unit membership management and the CSV unit import, and
fixes the bootstrap role statement.

- Units and unit members: create a unit, list unit members, update and delete
  a unit member, each behind a unit- or community-scoped authorization marker
  added to `api/internal/authz/scoped`.
- CSV import: `importUnits` ingests a CSV file (bounded by a size limit and a
  5000-row limit) and `getUnitImportTemplate` serves the template. Cells are
  passed through a single shared formula-injection guard
  (`handlers/csv_safety.go`) on both the template and the ingestion path.
- Bootstrap fix (`4970237`): the role statement in
  `api/migrations/bootstrap/00001_roles.go` is taken out of the dollar-quoted
  `DO` block. An operator password containing `$do$` closed the block early and
  broke deployment; the commit records that the injected SQL did not execute
  when tried, and the fix is structural rather than relying on that accident.
  This commit is one of the later fixes the earlier slices depend on.
- `api/go.mod` promotes `github.com/shopspring/decimal v1.4.0` from indirect to
  direct; no version change.

- New OpenAPI `operationId`s: `createUnit`, `importUnits`,
  `getUnitImportTemplate`, `listUnitMembers`, `updateUnitMember`,
  `deleteUnitMember`.
- Schema migrations: none. The bootstrap role migration
  `api/migrations/bootstrap/00001_roles.go` is modified (see above).
- Added test assertions on forbidden/not-found status: 4 / 9.

### Commits

- `1f5bcda` feat(m1-communities): add unit management and CSV import (Phase 4-5/8, WU-3/PR3)
- `4970237` fix(bootstrap): take the role statement out of the dollar-quoted block

### Changes

| Area | Files | Lines |
|---|---|---|
| `api/internal/http/` (unit, unit-member and CSV import handlers, `csv_safety.go`, DTOs, `apperr`, wiring) | 9 | +1108 |
| `api/internal/db/` (queries + sqlc generated code) | 5 | +261/-14 |
| Generated contract (`api/openapi/openapi.yaml`, `packages/shared` client and schemas) | 3 | +960 |
| `api/internal/authz/` (unit scope resolution and registration) | 3 | +65 |
| `api/migrations/bootstrap/00001_roles.go` | 1 | +69/-11 |
| Tests (`unit_test.go`, `unit_csv_import_test.go`, `csv_safety_test.go`, `unit_register_test.go`, `register_test.go`, `cmd/vecingest/migrate_test.go`) | 6 | +1024 |
| `openspec/changes/m1-communities/` (`apply-progress.md`, `tasks.md`) | 2 | +152/-24 |
| `api/go.mod` | 1 | +1/-1 |

### Size

30 files, +3640/-50 (`git diff --shortstat 86f8edc...4970237`). This exceeds
the 400-line review budget; shrinking it would require rewriting commits that
carry burned review authority, so the slice is delivered as recorded.

### Verification

`ci` run 37750338344 passed at `4970237`, this pull request's head commit.
`security` run 37750338334 at the same commit failed (see CI status note).

A passing `ci` is not evidence for any checklist item below, and it does not
make this slice merge-ready. `ci` succeeds when every job either succeeded or
was skipped by its path filter (the `ci-required` job in
`.github/workflows/ci.yml`), and the job-level results of this run are not
cited in this body.

Risk that did not occur: the slices are historically accurate, not
independently green. Later commits fix earlier ones: `4970237`, at this
slice's tip, fixes the bootstrap role statement, and `604f6a5` (slice 4)
fixes River sequence grants. This body predicted that `ci` might therefore
fail on this pull request and only converge further down the chain. The
prediction did NOT materialise: `ci` passed on every intermediate slice, this
one included. The reviewed commits were not amended or rebased, because they
carry burned native review authority.

### CI status note

`security` run 37750338334 at `4970237` failed. The cause is diagnosed and is
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
  - Not checked: no schema migration; the bootstrap change is described
    above. No `make lint-scope` result for the new queries at this slice is
    cited.
- [ ] Membership middleware present on every new route, with a test
      confirming another role/community gets 403.
  - Not checked: `unit_test.go` and `unit_csv_import_test.go` add 4 forbidden
    and 9 not-found assertions, but no result for them at this slice is
    cited.
- [ ] Unit tests for the service and at least one e2e test of the main
      flow.
  - Not checked: unit tests (`csv_safety_test.go`, `unit_register_test.go`)
    and DB-backed API tests exist, but no test result at this slice is cited.
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
    transaction: `unit.create` (`handlers/units.go`), `unit.import`
    (`handlers/unit_csv_import.go`), `unit_member.update` and
    `unit_member.delete` (`handlers/unit_members.go`). No run result
    exercising them at this slice is cited.
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
- [x] Migration does not remove or weaken any append-only constraint.
  - Evidence: the only migration change is in
    `api/migrations/bootstrap/00001_roles.go`; its removed lines are the
    `DO $do$ ... END $do$` role statement and an outdated comment. No line
    touching the append-only event trigger is removed. No schema migration is
    added.
- [ ] `security.yml` is green (gitleaks, govulncheck, gosec, Semgrep,
      `pnpm audit --audit-level=high`, Trivy).
  - Not checked: `security` run 37750338334 at `4970237` failed on four
    jobs (`gitleaks`, `pnpm audit`, `semgrep`, `trivy`); the fixes exist only
    at the chain tip, in pull request #9 (see CI status note).
- [ ] Permission-matrix test passes for every new/changed route.
  - Not checked: the permission-matrix test does not exist yet at this slice
    (it arrives in slice 7).

### Second-person review required?

- [x] This PR touches auth, files, money, votes or personal data → apply
      the `security-review` label and request a second reviewer.
  - Justification: adds a CSV file upload and unit-member endpoints that read
    and change residents' personal data and roles, and changes the bootstrap
    statement that creates the application database role.
  - The `security-review` label does not currently exist in this repository.
    It has to be created, or this requirement is tracked here in the body
    text. The label has not been applied.
- [ ] None of the above applies.

## Rollback plan

Revert order: revert this slice only after every later slice of the chain that
has been merged (slices 4 to 8) has been reverted, newest first. Then revert
this slice's merge commit on the branch it landed on.

No schema migration is added, so no goose rollback is needed. Rows written to
`units` and `unit_members` remain, and their `audit_log` entries are
append-only and must not be deleted.

Bootstrap: reverting `4970237` would bring back the dollar-quoted role
statement that breaks deployment for any `APP_DB_PASSWORD` containing `$do$`.
If only the unit and CSV import feature must be withdrawn, revert `1f5bcda`
alone and keep `4970237`. The bootstrap change alters only how the
application role is created or updated, not existing data, so it needs no
database rollback.
