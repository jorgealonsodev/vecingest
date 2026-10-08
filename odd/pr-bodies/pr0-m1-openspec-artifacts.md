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

`ci` run 37750325244 passed at `1755844`, this pull request's head commit.
`security` run 37750325254 at the same commit failed (see CI status note).

A passing `ci` is not evidence for any checklist item below, and it does not
make this slice merge-ready. `ci` succeeds when every job either succeeded or
was skipped by its path filter (the `ci-required` job in
`.github/workflows/ci.yml`), and the job-level results of this run are not
cited in this body.

### CI status note

`security` run 37750325254 at `1755844` failed. The cause is diagnosed and is
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
carry. `security` had also failed on `main` at `f6c79eb` (run 35775418922,
2026-09-22), before this chain was opened.

No check mechanically gates merging: `main` has no branch protection and the
repository has no rulesets, so no check is a required status. The gates are
this template and human review, and merging remains a human decision.

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
    `gitleaks` backstop fails on this pull request (see CI status note).
- [ ] Migration does not remove or weaken any append-only constraint.
  - Not applicable: no migration in this slice.
- [ ] `security.yml` is green (gitleaks, govulncheck, gosec, Semgrep,
      `pnpm audit --audit-level=high`, Trivy).
  - Not checked: `security` run 37750325254 at `1755844` failed on four
    jobs (`gitleaks`, `pnpm audit`, `semgrep`, `trivy`); the fixes exist only
    at the chain tip, in pull request #9 (see CI status note).
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
