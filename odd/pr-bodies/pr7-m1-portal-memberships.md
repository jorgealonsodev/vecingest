> Chained pull request 8 of 9 (slice 7). Base branch:
> `feature/m1-mfa-enrollment`. It must not be merged before its base.

## Summary

Closes the M1 portal flow and the M1 authorization tooling: `GET /v1/me` now
returns the caller's memberships, the app renders a portal row per membership
and gains the invitation-code screen, the permission matrix is generated from
`openapi.yaml` at full route coverage, and the database-backed API
authorization suite becomes part of the CI `e2e` job.

- API: `GET /v1/me` returns `memberships`, one entry per office or community
  membership with its scope, id, name and role (`handlers/me.go`,
  `dto/me.go`), backed by new office- and unit-member queries.
- App (Expo): `PortalScreen` renders one selectable row per membership and a
  client-side portal context picks the active one (`portalContext.ts`); a new
  `InvitationScreen` (route `app/(auth)/invitation.tsx`) previews and accepts
  an invitation code and lands on the portal after a consumed invitation even
  when the refresh token cannot be persisted.
- Authorization tooling: `permission_matrix_test.go` is generated from
  `openapi.yaml` and asserts, per documented operation and caller, that a
  foreign tenant is denied with 403 or 404 and an in-tenant caller with an
  insufficient role gets exactly 403. `lintscope` retires the table-wide
  `audit_log` exception in favor of two named query exceptions.
- CI: `.github/workflows/ci.yml` adds the step
  `go test -v -race -count=1 ./internal/http/api/...` (Testcontainers) to the
  `e2e` job, which runs when backend paths change, so from this slice on the
  pull request CI runs the database-backed API suite.
- Docs: `PRD_go.md` and `openspec/config.yaml` authz wording corrected to the
  typed-scope model; `docs/security/gates/M1.md` opens the M1 security gate.
  That document records Checkpoint A as OPEN with one blocking finding (no
  archived remote CI evidence yet for the API suite wiring) and Checkpoint B
  as OPEN with no evidence collected.

- New OpenAPI `operationId`s: none. Extends the existing `GET /v1/me`
  response.
- Schema migrations: none.
- Added test assertions on forbidden/not-found status: 9 / 6.

### Commits

- `fd1e7a7` feat(m1-communities): return the caller's office and community memberships from get /v1/me
- `d2c4aa4` fix(m1-communities): retire the table-wide audit_log lint-scope exception for two named chain reads
- `a62d78f` test(m1-communities): generate the permission matrix from openapi.yaml at full route coverage
- `1e849a8` feat(m1-communities): render one selectable portal row per get /v1/me membership
- `0d119aa` feat(m1-communities): pick one portal context per membership, resolved client-side
- `8019dd4` docs(m1-communities): record phase 9 portal membership evidence and the accept-invitation gap
- `07dcf71` feat(m1-communities): enable the invitation-code entry with preview and accept
- `6275363` docs(m1-communities): record the invitation-code flow decision and 9.7-9.8 evidence
- `5d55abc` fix(m1-communities): land on the portal after a consumed invitation even when the refresh token cannot be persisted
- `a0486e2` docs(m1-communities): correct the authz reference docs and open the M1 security gate
- `d7340c8` ci(m1): run database-backed API authorization tests in required job
- `7ff0eca` docs(odd): record verified CI work unit and review closure

### Changes

| Area | Files | Lines |
|---|---|---|
| App (`InvitationScreen.tsx`, `PortalScreen.tsx`, `portalContext.ts`, `LoginScreen.tsx`, `errorMessages.ts`, invitation route) | 6 | +937/-95 |
| `api/internal/http/` (`handlers/me.go`, `dto/me.go`) | 2 | +78/-10 |
| `api/internal/db/` (queries + sqlc generated code) | 5 | +126 |
| Generated contract (`api/openapi/openapi.yaml`, `packages/shared` client and schemas) | 3 | +55 |
| `api/cmd/lintscope/main.go` | 1 | +20/-39 |
| `.github/workflows/ci.yml` | 1 | +4 |
| Tests (`permission_matrix_test.go`, `me_test.go`, `lintscope/main_test.go`, app screen and context tests) | 9 | +1469/-22 |
| Docs (`PRD_go.md`, `openspec/config.yaml`, `docs/security/gates/M1.md`, OpenSpec progress, tasks and spec, two ODD task files) | 8 | +686/-39 |

### Size

35 files, +3375/-205 (`git diff --shortstat f5434cf...7ff0eca`). This exceeds
the 400-line review budget; shrinking it would require rewriting commits that
carry burned review authority, so the slice is delivered as recorded.

### Verification

No per-slice verification run exists for this slice. No test, linter or CI
run has been executed against `7ff0eca` in isolation. Any CI failure on this
pull request is reported on it; the reviewed commits are not amended or
rebased, because they carry burned native review authority.

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
  - Not checked: no migration; `make lint-scope` was not run on the new
    queries at this slice.
- [ ] Membership middleware present on every new route, with a test
      confirming another role/community gets 403.
  - Not checked: no route is added; the permission matrix that asserts this
    for every documented operation is added here but was not run at this
    slice.
- [ ] Unit tests for the service and at least one e2e test of the main
      flow.
  - Not checked: `me_test.go` and the app screen tests are added, but nothing
    was run at this slice.
- [ ] Domain events/notifications defined per the design's event table and
      enqueued with `river.InsertTx` inside the same transaction.
  - Not applicable: no event or notification in this slice.
- [ ] UI copy lives in i18n (es), not hardcoded strings; errors use stable
      codes.
  - Not checked: Spanish messages are added to
    `app/src/screens/errorMessages.ts`, but the new screens were not verified
    for hardcoded copy.
- [ ] Accessible screen: labels, focus order, contrast — checked on iOS,
      Android and web where applicable.
  - Not checked: no accessibility check on iOS, Android or web is recorded
    for `InvitationScreen` or `PortalScreen`.
- [ ] `audit_log` entry added if the action is sensitive (money, votes,
      personal data, permissions).
  - Not applicable: `GET /v1/me` is a read; invitation accept uses the
    existing endpoint, which already writes `invitation.accept`.
- [ ] Short documentation added to the module's README and, if
      applicable, the runbook.
  - Not checked: `docs/security/gates/M1.md`, `PRD_go.md` and OpenSpec
    records are updated; no module README or runbook entry is added.

### Security review checklist (mandatory for every PR)

- [ ] Decoders use `DisallowUnknownFields` and `huma` validation tags.
  - Not checked: not verified for this slice.
- [ ] Membership/tenant scope is checked on every new or changed endpoint.
  - Not checked: `GET /v1/me` is identity-scoped; no run at this slice
    confirms the changed response.
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
  - Not checked: the matrix is added here, but it was not run at this slice.

### Second-person review required?

- [x] This PR touches auth, files, money, votes or personal data → apply
      the `security-review` label and request a second reviewer.
  - Justification: `GET /v1/me` now returns the caller's memberships and
    roles, the app adds the invitation accept flow, and the authz
    enforcement tooling (`lintscope`, permission matrix, CI step) changes.
  - The `security-review` label does not currently exist in this repository.
    It has to be created, or this requirement is tracked here in the body
    text. The label has not been applied.
- [ ] None of the above applies.

## Rollback plan

Revert order: revert this slice only after slice 8, if merged, has been
reverted; slice 8 extends `permission_matrix_test.go` and the unit-member
queries. Then revert this slice's merge commit on the branch it landed on.

No migration is added, so no schema rollback is needed. A full revert removes
the `memberships` field from `GET /v1/me`; app builds that read it must be
reverted or redeployed together with the API.

If only the app or the `/v1/me` change must be withdrawn, revert the feature
commits (`fd1e7a7`, `1e849a8`, `0d119aa`, `07dcf71`, `5d55abc`) and keep the
authorization safeguards: the generated permission matrix (`a62d78f`), the
narrowed `lintscope` exceptions (`d2c4aa4`) and the CI step (`d7340c8`).
Such a partial revert has not been tried; confirm the API suite passes on it
before merging the revert.
