> Chained pull request 9 of 9 (slice 8). Base branch:
> `feature/m1-portal-memberships`. It must not be merged before its base.
> This is the tip of the chain.

## Summary

Adds the first M2 capability: the incident API for creating, listing and
reading community incidents, with tenant-safe persistence and typed
visibility enforced before any handler runs. It also carries the
repository-wide CI and security gate work, one authentication fix and the
delivery records; see the scope disclosure below.

### Scope disclosure: this pull request is wider than its title

This pull request is no longer only the M2 incident API. It splits into two
halves:

1. The M2 incident API itself, `7ff0eca...479dc8d`: 6 commits, 23 files,
   +3763/-20. This is the work the title describes.
2. Later hardening, `479dc8d...def2249`: 12 commits, 26 files, +2228/-51
   (the two halves share four files, three incident test files and
   `api/internal/db/querier.go`, hence 45 files in total). This half
   is NOT the incident API. It is:
   - the repository-wide CI and security gate work: gofumpt formatting of
     three test files, a gosec G602 suppression, six pinned transitive CVEs,
     seven gitleaks fixture false positives ignored by fingerprint, and two
     accepted unfixable CVEs recorded in `docs/security/threat-model.md`;
   - one authentication fix: the OTP issuance window is taken off a
     floating-point query parameter (`c2da7da`), with a new boundary test
     (`d08168f`, `def2249`);
   - the delivery trackers and these very pull request bodies.

This widened a single pull request beyond its stated purpose. The hardening
would have been cleaner as its own chained pull request. It was not split
because GitHub cannot retarget an open pull request's head branch: splitting
would mean closing #9 and losing its review history. Review the two halves
separately; the commit list below is grouped by half.

### M2 incident API

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

### Later hardening

- CI recovery (`618efd9`, `94dc063`): `gofumpt` v0.9.2 flagged three M2 test
  files (semantically empty reformatting), and fixing that unmasked two gosec
  G602 false positives on `want[i]` in `assertIncidentIDs`, suppressed per
  line with a documented reason.
- Security gate (`d1b71c2`, `9c8b2ee`, `c2da7da`, `afe0b2e`): seven gitleaks
  findings, all one public example passphrase in a test fixture at
  `a6976ab`, ignored by fingerprint in `.gitleaksignore`; six fixable
  transitive CVEs pinned with range-scoped overrides in
  `pnpm-workspace.yaml`; the one blocking semgrep finding fixed (below); two
  CVEs with no upstream fix accepted in `.trivyignore`,
  `pnpm-workspace.yaml` (`auditConfig.ignoreCves`) and entries 4 and 5 of
  `docs/security/threat-model.md`.
- Authentication fix (`c2da7da`): `CountOTPChallengesIssuedSince` passed the
  MFA enrollment-code issuance window through
  `make_interval(secs => ...::double precision)`, which made sqlc generate a
  floating-point parameter under `api/internal/db`, where the project's own
  `no-float-money-go` rule forbids one. The window is now a whole-second
  `int32` multiplied into an interval. Three comments that tripped the same
  lexical rules were reworded rather than suppressed (two DTO comments and
  one comment in migration `00005`).
- Boundary test (`d08168f`, `def2249`): a new test ages the issued codes to
  just inside the window and requires the 429 to stand, pinning the lower
  bound that the existing tests left open; `def2249` asserts the backdating
  actually moved the expected rows, so the test cannot pass vacuously.
- Delivery records (`8a9c9f8`, `784cc6c`, `8616288`, `95cb6a8`): the chained
  delivery tracker `odd/tasks/m1-m2-chained-delivery.md` and the nine pull
  request bodies under `odd/pr-bodies/`.

- New OpenAPI `operationId`s: `createIncident`, `listIncidents`,
  `getIncident`.
- Schema migrations: `api/migrations/schema/00012_incidents.sql`. In
  addition, `c2da7da` rewords one comment line in the already-applied
  `00005_otp_email_channel.sql`; no SQL statement changes.
- Added test assertions on forbidden/not-found status: 8 / 16, all in the
  incident API half; the hardening half adds none.

### Commits

M2 incident API (`7ff0eca...479dc8d`):

- `082f84d` docs(odd): settle initial M2 incident API contract
- `5634883` feat(incidents): add tenant-safe persistence and visibility queries
- `34d3971` feat(authz): enforce typed incident visibility before handlers
- `3bd48ae` feat(incidents): add authorized incident creation API
- `b70f526` feat(incidents): add incident list and detail API with keyset pagination
- `479dc8d` test(incidents): close review-flagged coverage gaps for incident API

Later hardening (`479dc8d...def2249`):

- `618efd9` style(incidents): apply gofumpt formatting to three test files
- `8a9c9f8` docs(odd): record the nine-slice chained delivery and its PR bodies
- `94dc063` test(incidents): suppress a gosec G602 false positive in the id assertion
- `784cc6c` docs(odd): record the ci recovery on chained slice nine
- `8616288` docs(odd): record the review receipt for the gosec work unit
- `d1b71c2` chore(security): ignore seven gitleaks false positives on one test fixture
- `9c8b2ee` fix(deps): pin six fixable transitive CVEs out of the dependency tree
- `c2da7da` fix(mfa): take the OTP issuance window off a float parameter
- `afe0b2e` chore(security): accept the two CVEs with no upstream fix
- `95cb6a8` docs(odd): record the security gate turning green on slice nine
- `d08168f` test(mfa): pin the lower bound of the OTP issuance window
- `def2249` test(mfa): assert the backdating moved rows in the window test

### Changes

| Area | Files | Lines |
|---|---|---|
| `api/internal/http/` (`handlers/incidents.go`, `dto/incidents.go`, wiring) | 3 | +641 |
| `api/internal/db/` (`incidents.sql`, `unit_members.sql`, `otp_challenges.sql` + sqlc generated code and models) | 8 | +613/-3 |
| Generated contract (`api/openapi/openapi.yaml`, `packages/shared` client and schemas) | 3 | +566 |
| `api/internal/authz/` (incident grant, resolution, assertion, registration) | 4 | +152/-19 |
| `api/migrations/schema/00012_incidents.sql` | 1 | +55 |
| Incident tests (`incident_test.go`, `test/incidents_persistence_test.go`, `scoped/register_test.go`, `assert_test.go`, `permission_matrix_test.go`) | 5 | +1673/-2 |
| `odd/tasks/m2-incident-api.md` | 1 | +112 |
| OTP issuance window (`domain/auth/mfa/enroll_email.go`, `http/api/mfa_enroll_email_test.go`) | 2 | +56/-1 |
| Comments reworded for semgrep (`dto/communities.go`, `dto/units.go`, `00005_otp_email_channel.sql`) | 3 | +9/-3 |
| Security gate configuration (`.gitleaksignore`, `.trivyignore`, `pnpm-workspace.yaml`, `pnpm-lock.yaml`) | 4 | +130/-25 |
| `docs/security/threat-model.md` (accepted CVEs, entries 4 and 5) | 1 | +65 |
| Delivery records (`odd/tasks/m1-m2-chained-delivery.md`, nine `odd/pr-bodies/*.md`) | 10 | +1901 |

### Size

45 files, +5973/-53 (`git diff --shortstat 7ff0eca...def2249`), of which the
incident API half is 23 files, +3763/-20 (`7ff0eca...479dc8d`) and the
hardening half 26 files, +2228/-51 (`479dc8d...def2249`). This exceeds the
400-line review budget; shrinking it would require rewriting commits that
carry burned review authority, so the slice is delivered as recorded. Of the
hardening half, 1901 lines are the delivery tracker and these pull request
bodies.

Every figure above is anchored to a named commit range rather than to "now",
on purpose: this file is itself part of the diff it measures. The commit that
refreshed these nine bodies lands on top of `def2249`, so it adds its own
changes to this slice without invalidating any number stated here. Re-measure
with the commands quoted above rather than trusting a running total.

### Verification

`ci` run 37830746662 passed at `def2249`, this pull request's head commit.
`security` run 37830746661 at the same commit passed all six jobs: gitleaks,
semgrep, trivy, pnpm audit, gosec and govulncheck.

A passing `ci` does not make this pull request merge-ready, and it is not
used below as evidence for any checklist item: `ci` succeeds when every job
either succeeded or was skipped by its path filter (the `ci-required` job in
`.github/workflows/ci.yml`), and the job-level results of this run are not
cited in this body.

Local runs on record, all covering the whole chain up to the named commit,
not this pull request in isolation:

- At `d08168f`: `./internal/http/api/...` 118 RUN / 118 PASS, 0 FAIL, 0 SKIP,
  ok 302.597s. The earlier figure of 117 RUN / 117 PASS (303.099s) belongs to
  the older tip `479dc8d`; the 118th is the window test `d08168f` adds. At the
  same commit: `golangci-lint` 2.13.2 0 issues, `gofumpt` 0.9.2 clean,
  `make lint-scope` OK, `git diff --check` clean.
- At `def2249`: `TestMFAEnrollEmail` 9 PASS / 0 FAIL / 0 SKIP in 25.099s,
  `golangci-lint` 0 issues, `gofumpt` clean. The full `./internal/http/api/...`
  suite was not re-run locally after this test-only commit.
- At `c2da7da`: `./test/...` ok 41.584s.
- Permission matrix: 37/37 documented operations with 216 assertions at
  `479dc8d`; 216 assertions again at `94dc063`
  (`-run 'Incident|PermissionMatrix'`, 24 RUN, 0 FAIL, 0 SKIP).
- At `afe0b2e`, with the scanner versions CI pins: `gitleaks` no leaks,
  `semgrep` exit 0, `trivy` fs HIGH,CRITICAL exit 0,
  `pnpm audit --audit-level=high` exit 0.

### Review receipts

The twelve hardening commits are covered by these native review receipts,
each approved and acknowledged with authority burned:

- `review-bc2e16bc8286f674`: `618efd9`, `8a9c9f8`.
- `review-18d981b9d7ddebee`: `94dc063`, `784cc6c`.
- `review-75b0cee4531ab958`: `8616288`.
- `review-327034840632eee3` and `review-8d95fc92ac319c12`: `d1b71c2`,
  `9c8b2ee`, `c2da7da`, `afe0b2e`.
- `review-d8b8ca4bb182bb41`: `95cb6a8`.
- `review-81c452f559671922`: `d08168f`.
- `review-03490c7138c73289`: `def2249`.

The accumulated whole-branch candidate is NOT reviewable as one unit: the
native controller rejects it with `lens_context_budget_exceeded`, whose own
message says to review the change as smaller candidates. That is why
delivery is chained.

### Disclosed limitations

- A fractional-second OTP issuance window now loses its fraction, because Go
  truncates before the value reaches the database. No current caller uses
  one; the window is exactly `time.Hour`.
- The new boundary test ages codes to five seconds inside the window, so a
  window off by a second or two still passes; the margin is chosen to avoid a
  flaky test.
- Two accepted CVEs have no upstream fix, recorded as entries 4 and 5 of
  `docs/security/threat-model.md` and in both ignore lists:
  - `CVE-2026-93687` (`braces@3.0.3`, stack-exhaustion denial of service),
    reachable only through the Jest devDependency chain expanding this
    repository's own globs.
  - `CVE-2026-85393` (`node-forge@1.4.0`, RSA PKCS#1 v1.5 signature
    forgery), arriving through `expo`, a production dependency. Its
    acceptance rests on verified unreachability and carries a MANDATORY
    re-evaluation trigger: adopting expo-updates code signing or an EAS
    Update pipeline puts the vulnerable code path into use and invalidates
    the acceptance.
- `security` is green with documented exceptions: the seven gitleaks
  fingerprints and the two accepted CVEs above are ignored, not removed.
- Open advisory from review `review-18d981b9d7ddebee`,
  `R3-blanket-gosec-suppression`: the two `//nolint:gosec` directives in
  `api/internal/http/api/incident_test.go` silence every gosec rule on those
  lines, not G602 alone. Left as separate later work.

### CI status note

`security` run 37830746661 at `def2249` is green on all six jobs. The fixes
that made it green live only here, at the tip of the chain; pull requests #1
to #8 have older trees and their `security` runs stay red. That was not
corrected, because propagating the fixes to the base of the chain would
require rebasing all nine branches and destroying the native review receipts
those commits carry.

No check mechanically gates merging: `main` has no branch protection and the
repository has no rulesets, so no check is a required status. The gates are
this template and human review, and merging remains a human decision.

## Definition of Done (PRD_go.md section 9)

- [ ] Input/output structs have validations; `make gen` was run and
      `openapi.yaml`, sqlc code and the TS client are regenerated and
      committed (dirty-diff gate in `ci.yml`).
  - Not checked: the DTOs carry `enum`, `format`, `minimum` and `maximum`
    tags, and `openapi.yaml`, the sqlc code and the TS client are committed,
    but no local `make gen` dirty-diff run is recorded and the `gen` job
    result of `ci` run 37830746662 is not cited here.
- [ ] `goose` migration reviewed (expand/contract if it touches existing
      data); `.sql` queries filter by their tenant scope column.
  - Not checked: `make lint-scope` was OK at `d08168f`, and `00012` is
    additive (a new table, plus a `UNIQUE (community_id, id)` on `units` that
    existing rows always satisfy because `id` is the primary key); the
    `00005` change is a one-line comment rewording. No migration review
    record is cited here.
- [x] Membership middleware present on every new route, with a test
      confirming another role/community gets 403.
  - Evidence: the permission matrix passed 37/37 documented operations with
    216 assertions at `479dc8d`, including `createIncident`, `listIncidents`
    and `getIncident`, and 216 assertions again at `94dc063`;
    `./internal/http/api/...` passed 118/118 at `d08168f`. The matrix asserts
    exactly 403 for an in-tenant caller with an insufficient role, and 403 or
    404 for a foreign community, as `openspec/config.yaml` requires (404
    preferred, so as not to confirm existence). `incident_test.go` adds 8
    forbidden and 16 not-found assertions.
- [ ] Unit tests for the service and at least one e2e test of the main
      flow.
  - Not checked: there is no separate incident service layer. The handlers
    are covered by DB-backed API tests and `test/incidents_persistence_test.go`,
    and the authz registration by unit tests, all passing in the local runs
    above, but that is not the unit-plus-e2e split this item names.
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
  - Not checked: the contract is recorded in `odd/tasks/m2-incident-api.md`
    and the accepted CVEs in `docs/security/threat-model.md`; no module
    README or runbook entry is added.

### Security review checklist (mandatory for every PR)

- [ ] Decoders use `DisallowUnknownFields` and `huma` validation tags.
  - Not checked: huma validation tags are present on the DTOs, but
    unknown-field rejection was not verified for this slice.
- [x] Membership/tenant scope is checked on every new or changed endpoint.
  - Evidence: the permission matrix passed 37/37 documented operations with
    216 assertions at `479dc8d`, covering the three new incident operations,
    and 216 assertions again at `94dc063`; the boot assertion requires every
    documented operation to be scoped. The hardening half adds or re-scopes
    no endpoint; `c2da7da` changes only the issuance-cap query behind the
    existing, identity-scoped MFA enrollment endpoint.
- [ ] No sensitive data appears in logs, error responses or push payloads.
  - Not checked: `TestIncident_CreateAuthorizationValidationAndSafeProjection`
    covers the create response projection, but logs and other responses were
    not reviewed for this item.
- [ ] Secrets stay out of the code (verified locally; `gitleaks` in
      `security.yml` is the CI backstop).
  - Not checked: the `gitleaks` backstop passed in run 37830746661 at
    `def2249`, with seven fixture fingerprints ignored, but the item asks for
    local verification and the only local `gitleaks` run on record was at
    `d1b71c2` and `afe0b2e`, not at this pull request's head.
- [x] Migration does not remove or weaken any append-only constraint.
  - Evidence: `00012_incidents.sql` only creates `incidents`, its indexes
    and grants, and adds a unique constraint on `units`; it does not reference
    `audit_log` or the append-only guard (`grep -ci` for `audit_log|append`
    returns 0). The only other migration change, in `00005`, rewords one
    comment line and changes no SQL statement.
- [x] `security.yml` is green (gitleaks, govulncheck, gosec, Semgrep,
      `pnpm audit --audit-level=high`, Trivy).
  - Evidence: `security` run 37830746661 at `def2249`, this pull request's
    head commit, passed all six jobs: gitleaks, semgrep, trivy, pnpm audit,
    gosec and govulncheck. Green with the documented exceptions listed under
    Disclosed limitations.
- [x] Permission-matrix test passes for every new/changed route.
  - Evidence: the matrix passed at `479dc8d` (37/37 documented operations,
    216 assertions) and again at `94dc063` (216 assertions), and this diff
    adds matrix requests and fixtures for `createIncident`, `listIncidents`
    and `getIncident`. The runs cover the chain up to those commits, not this
    slice in isolation.

### Second-person review required?

- [x] This PR touches auth, files, money, votes or personal data → apply
      the `security-review` label and request a second reviewer.
  - Justification: adds new authorization grants and visibility rules, and
    incidents store resident-authored text linked to users and units. The
    hardening half also changes the MFA enrollment-code issuance query and
    the security gate configuration, including two accepted CVEs.
  - The `security-review` label does not currently exist in this repository.
    It has to be created, or this requirement is tracked here in the body
    text. The label has not been applied.
- [ ] None of the above applies.

## Rollback plan

Revert order: this is the last slice of the chain, so it can be reverted
first and on its own. Revert its merge commit on the branch it landed on;
slices 0 to 7 do not depend on it.

Partial revert: the two halves can be withdrawn separately. Reverting only
the incident API (`082f84d` to `479dc8d`) also needs the formatting and
suppression commits on its test files (`618efd9`, `94dc063`) reverted first.
Reverting the hardening half brings the four failing `security` jobs back
(`d1b71c2`, `9c8b2ee`, `afe0b2e`) and the floating-point issuance-window
parameter with them (`c2da7da`); prefer keeping it. Neither partial revert
has been tried; confirm `ci` and `security` on it before merging the revert.

Schema: the preferred rollback is code-only and keeps
`00012_incidents.sql` in the tree, matching the version recorded in
`goose_db_version_schema`; the unused `incidents` table and the extra
`units` constraint are harmless to the reverted code. If the schema must be
removed, there is no scripted down path (`vecingest migrate` applies up only):
take a backup, run the file's `-- +goose Down` block as `vecingest_owner`
(it drops `incidents` and every incident row, then drops
`units_community_id_id_key`), and delete version 12 from
`goose_db_version_schema`. The `00005` comment change needs no database
action.
