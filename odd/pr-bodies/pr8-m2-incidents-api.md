> Chained pull request 9 of 9 (slice 8). Base branch:
> `feature/m1-portal-memberships`. It must not be merged before its base.
> This is the tip of the chain.

## Summary

Adds the first M2 capability: the incident API for creating, listing and
reading community incidents, with tenant-safe persistence and typed
visibility enforced before any handler runs.

- Persistence: migration `00012` creates `incidents` with the tenant column
  `community_id`, a `common` or `unit` scope, and a composite foreign key
  `(community_id, unit_id)` to a new `UNIQUE (community_id, id)` on `units`,
  so a row cannot pair one community with another community's unit.
- Authorization: incident routes resolve an unforgeable
  `authz.IncidentAccess` grant (`scoped.Incident`). The resolver uses
  `GetVisibleIncidentByID` as the visibility proof before any handler runs,
  and the list and detail queries apply visibility in SQL.
- API: `POST /v1/communities/{id}/incidents`, `GET
  /v1/communities/{id}/incidents` (status, category and unit filters, page
  size 1 to 100, keyset pagination with an opaque cursor bound to the
  community, the caller and the filters; each page rechecks visibility) and
  `GET /v1/incidents/{id}`.
- `permission_matrix_test.go` is extended with the three incident operations.

- New OpenAPI `operationId`s: `createIncident`, `listIncidents`,
  `getIncident`.
- Schema migrations: `api/migrations/schema/00012_incidents.sql`.
- Added test assertions on forbidden/not-found status: 8 / 16.

### Commits

- `082f84d` docs(odd): settle initial M2 incident API contract
- `5634883` feat(incidents): add tenant-safe persistence and visibility queries
- `34d3971` feat(authz): enforce typed incident visibility before handlers
- `3bd48ae` feat(incidents): add authorized incident creation API
- `b70f526` feat(incidents): add incident list and detail API with keyset pagination
- `479dc8d` test(incidents): close review-flagged coverage gaps for incident API

### Changes

| Area | Files | Lines |
|---|---|---|
| `api/internal/http/` (`handlers/incidents.go`, `dto/incidents.go`, wiring) | 3 | +641 |
| `api/internal/db/` (`incidents.sql`, `unit_members.sql` + sqlc generated code and models) | 6 | +586 |
| Generated contract (`api/openapi/openapi.yaml`, `packages/shared` client and schemas) | 3 | +566 |
| `api/internal/authz/` (incident grant, resolution, assertion, registration) | 4 | +152/-19 |
| `api/migrations/schema/00012_incidents.sql` | 1 | +55 |
| Tests (`incident_test.go`, `test/incidents_persistence_test.go`, `scoped/register_test.go`, `assert_test.go`, `permission_matrix_test.go`) | 5 | +1651/-1 |
| `odd/tasks/m2-incident-api.md` | 1 | +112 |

### Size

23 files, +3763/-20 (`git diff --shortstat 7ff0eca...479dc8d`). This exceeds
the 400-line review budget; shrinking it would require rewriting commits that
carry burned review authority, so the slice is delivered as recorded.

### Verification

The only full verification on record is a local run at the chain tip
`479dc8d`, from a previous session:

- `./internal/http/api/...`: 117 RUN / 117 PASS, 0 FAIL, 0 SKIP, 303.099s.
- `./test/...`: ok 36.818s.
- lint-scope: OK.
- `git diff --check`: clean.
- Permission matrix: 37/37 documented operations with 216 assertions.

`479dc8d` is this pull request's head commit, but the run covers the whole
chain up to here; it is not a verification of this slice in isolation. It is
a local run, not a CI run; CI results will come from this pull request's own
checks.

### CI status note

`security.yml` is already failing on `main` at `f6c79eb`, before any slice of
this chain: runs 35775418922 (`security`) and 35775419322 (`deploy`) failed on
2026-09-22 while `ci` passed. That failure is pre-existing on `main` and is not
caused by this slice.

## Definition of Done (PRD_go.md section 9)

- [ ] Input/output structs have validations; `make gen` was run and
      `openapi.yaml`, sqlc code and the TS client are regenerated and
      committed (dirty-diff gate in `ci.yml`).
  - Not checked: the DTOs carry `enum`, `format`, `minimum` and `maximum`
    tags, and `openapi.yaml`, the sqlc code and the TS client are committed,
    but the `make gen` dirty-diff gate was not part of the recorded run.
- [ ] `goose` migration reviewed (expand/contract if it touches existing
      data); `.sql` queries filter by their tenant scope column.
  - Not checked: lint-scope passed at the chain tip, and `00012` is additive
    (a new table, plus a `UNIQUE (community_id, id)` on `units` that existing
    rows always satisfy because `id` is the primary key), but no migration
    review record is cited here.
- [x] Membership middleware present on every new route, with a test
      confirming another role/community gets 403.
  - Evidence: at `479dc8d` the permission matrix passed 37/37 documented
    operations with 216 assertions, including `createIncident`,
    `listIncidents` and `getIncident`, and `./internal/http/api/...` passed
    117/117. The matrix asserts exactly 403 for an in-tenant caller with an
    insufficient role, and 403 or 404 for a foreign community, as
    `openspec/config.yaml` requires (404 preferred, so as not to confirm
    existence). `incident_test.go` adds 8 forbidden and 16 not-found
    assertions.
- [ ] Unit tests for the service and at least one e2e test of the main
      flow.
  - Not checked: there is no separate incident service layer. The handlers
    are covered by DB-backed API tests and `test/incidents_persistence_test.go`,
    and the authz registration by unit tests, all passing at the chain tip,
    but that is not the unit-plus-e2e split this item names.
- [ ] Domain events/notifications defined per the design's event table and
      enqueued with `river.InsertTx` inside the same transaction.
  - Not checked: no event or notification is enqueued; whether an M2 event
    table requires one for incident creation was not verified.
- [ ] UI copy lives in i18n (es), not hardcoded strings; errors use stable
      codes.
  - Not checked: no UI in this slice; error codes were not reviewed against
    this item.
- [ ] Accessible screen: labels, focus order, contrast — checked on iOS,
      Android and web where applicable.
  - Not applicable: no screen in this slice.
- [ ] `audit_log` entry added if the action is sensitive (money, votes,
      personal data, permissions).
  - Not checked: no audit entry is added for incident creation; whether
    incident creation counts as sensitive was not decided in this slice.
- [ ] Short documentation added to the module's README and, if
      applicable, the runbook.
  - Not checked: the contract is recorded in `odd/tasks/m2-incident-api.md`;
    no module README or runbook entry is added.

### Security review checklist (mandatory for every PR)

- [ ] Decoders use `DisallowUnknownFields` and `huma` validation tags.
  - Not checked: huma validation tags are present on the DTOs, but
    unknown-field rejection was not verified for this slice.
- [x] Membership/tenant scope is checked on every new or changed endpoint.
  - Evidence: the permission matrix passed 37/37 documented operations with
    216 assertions at `479dc8d`, covering the three new incident operations;
    the boot assertion requires every documented operation to be scoped.
- [ ] No sensitive data appears in logs, error responses or push payloads.
  - Not checked: `TestIncident_CreateAuthorizationValidationAndSafeProjection`
    covers the create response projection, but logs and other responses were
    not reviewed for this item.
- [ ] Secrets stay out of the code (verified locally; `gitleaks` in
      `security.yml` is the CI backstop).
  - Not checked: no local secret scan is part of the recorded run, and the
    `security.yml` backstop is failing on `main` (see CI status note).
- [x] Migration does not remove or weaken any append-only constraint.
  - Evidence: `00012_incidents.sql` only creates `incidents`, its indexes
    and grants, and adds a unique constraint on `units`; it does not reference
    `audit_log` or the append-only guard (`grep -ci` for `audit_log|append`
    returns 0).
- [ ] `security.yml` is green (gitleaks, govulncheck, gosec, Semgrep,
      `pnpm audit --audit-level=high`, Trivy).
  - Not checked: `security.yml` is already failing on `main` at `f6c79eb`
    (pre-existing, not caused by this slice).
- [x] Permission-matrix test passes for every new/changed route.
  - Evidence: the matrix passed at `479dc8d` (37/37 documented operations,
    216 assertions), and this diff adds matrix requests and fixtures for
    `createIncident`, `listIncidents` and `getIncident`. The run covers the
    chain tip, not this slice in isolation.

### Second-person review required?

- [x] This PR touches auth, files, money, votes or personal data → apply
      the `security-review` label and request a second reviewer.
  - Justification: adds new authorization grants and visibility rules, and
    incidents store resident-authored text linked to users and units.
  - The `security-review` label does not currently exist in this repository.
    It has to be created, or this requirement is tracked here in the body
    text. The label has not been applied.
- [ ] None of the above applies.

## Rollback plan

Revert order: this is the last slice of the chain, so it can be reverted
first and on its own. Revert its merge commit on the branch it landed on;
slices 0 to 7 do not depend on it.

Schema: the preferred rollback is code-only and keeps
`00012_incidents.sql` in the tree, matching the version recorded in
`goose_db_version_schema`; the unused `incidents` table and the extra
`units` constraint are harmless to the reverted code. If the schema must be
removed, there is no scripted down path (`vecingest migrate` applies up only):
take a backup, run the file's `-- +goose Down` block as `vecingest_owner`
(it drops `incidents` and every incident row, then drops
`units_community_id_id_key`), and delete version 12 from
`goose_db_version_schema`.
