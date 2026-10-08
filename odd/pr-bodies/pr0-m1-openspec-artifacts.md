> Chained pull request 1 of 9 (slice 0). Base branch: `main`. This is the first
> slice of the chain; it must not be merged before its base, and none of the
> eight later slices may be merged before it.

## Summary

Adds the OpenSpec planning artifacts for milestone M1 (communities): research,
proposal, technical design, the delta specs for each M1 capability, and the
task breakdown. Documentation only; no code, schema, contract or workflow file
is touched. Every later slice in this chain implements and cites these
artifacts.

- New OpenAPI `operationId`s: none. Documentation only, 14 markdown files.
- Schema migrations: none.
- Added test assertions on forbidden/not-found status: 0 / 0.

### Commits

- `18f667e` docs(openspec): add M1 research and proposal artifacts
- `34dd097` docs(openspec): add M1 technical design
- `4d069ae` docs(openspec): add M1 delta specs
- `1755844` docs(openspec): add M1 task breakdown, close the research gaps

### Changes

| Area | Files | Lines |
|---|---|---|
| `openspec/changes/m1-communities/` (`research.md`, `proposal.md`, `design.md`, `tasks.md`) | 4 | +974 |
| `openspec/changes/m1-communities/specs/*/spec.md` (10 capabilities) | 10 | +1020 |

### Size

14 files, +1994 (`git diff --shortstat refs/remotes/origin/main...1755844`).
This exceeds the 400-line review budget; shrinking it would require rewriting
commits that carry burned review authority, so the slice is delivered as
recorded.

### Verification

No per-slice verification run exists for this slice. No test, linter or CI
run has been executed against `1755844` in isolation; CI results will come
only from this pull request's own checks.

### CI status note

`security.yml` is already failing on `main` at `f6c79eb`, before any slice of
this chain: runs 35775418922 (`security`) and 35775419322 (`deploy`) failed on
2026-09-22 while `ci` passed. That failure is pre-existing on `main` and is not
caused by this slice.

## Definition of Done (PRD_go.md section 9)

- [ ] Input/output structs have validations; `make gen` was run and
      `openapi.yaml`, sqlc code and the TS client are regenerated and
      committed (dirty-diff gate in `ci.yml`).
  - Not applicable: no struct, query or contract file in this slice.
- [ ] `goose` migration reviewed (expand/contract if it touches existing
      data); `.sql` queries filter by their tenant scope column.
  - Not applicable: no migration and no query in this slice.
- [ ] Membership middleware present on every new route, with a test
      confirming another role/community gets 403.
  - Not applicable: no route in this slice.
- [ ] Unit tests for the service and at least one e2e test of the main
      flow.
  - Not applicable: no code in this slice.
- [ ] Domain events/notifications defined per the design's event table and
      enqueued with `river.InsertTx` inside the same transaction.
  - Not applicable: no code in this slice. The design text defines the
    `river.InsertTx` requirement that later slices implement.
- [ ] UI copy lives in i18n (es), not hardcoded strings; errors use stable
      codes.
  - Not applicable: no UI in this slice.
- [ ] Accessible screen: labels, focus order, contrast — checked on iOS,
      Android and web where applicable.
  - Not applicable: no screen in this slice.
- [ ] `audit_log` entry added if the action is sensitive (money, votes,
      personal data, permissions).
  - Not applicable: no action implemented in this slice.
- [ ] Short documentation added to the module's README and, if
      applicable, the runbook.
  - Not checked: the slice adds OpenSpec artifacts, not a module README or
    runbook entry.

### Security review checklist (mandatory for every PR)

- [ ] Decoders use `DisallowUnknownFields` and `huma` validation tags.
  - Not applicable: no decoder in this slice.
- [ ] Membership/tenant scope is checked on every new or changed endpoint.
  - Not applicable: no endpoint in this slice.
- [ ] No sensitive data appears in logs, error responses or push payloads.
  - Not applicable: no code path that logs or responds.
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
  - Not applicable: no route in this slice.

### Second-person review required?

- [ ] This PR touches auth, files, money, votes or personal data → apply
      the `security-review` label and request a second reviewer.
- [x] None of the above applies.
  - Evidence: the diff is 14 markdown files under
    `openspec/changes/m1-communities/`; no executable code, schema or
    workflow changes. The auth, invitation and MFA designs described here are
    reviewed as code in slices 1 to 7.
  - For reference: the `security-review` label does not currently exist in
    this repository; the slices that need it track the requirement in their
    body text.

## Rollback plan

Documentation only: a plain `git revert` of the merge commit suffices. If later
slices of the chain have already been merged, revert them first, newest first,
because they edit `openspec/changes/m1-communities/tasks.md` and cite these
artifacts.
