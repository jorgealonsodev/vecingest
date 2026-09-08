<!--
  PRD_go.md section 9, Definition of Done, point 10 ("Revisión de
  seguridad del PR"). This checklist is mandatory for every PR. If the PR
  touches auth, files, money, votes or personal data, it additionally
  requires a second-person review under the `security-review` label
  (PRD DoD point 10).
-->

## Summary

<!-- What does this PR do and why? -->

## Definition of Done (PRD_go.md section 9)

- [ ] Input/output structs have validations; `make gen` was run and
      `openapi.yaml`, sqlc code and the TS client are regenerated and
      committed (dirty-diff gate in `ci.yml`).
- [ ] `goose` migration reviewed (expand/contract if it touches existing
      data); `.sql` queries filter by their tenant scope column.
- [ ] Membership middleware present on every new route, with a test
      confirming another role/community gets 403.
- [ ] Unit tests for the service and at least one e2e test of the main
      flow.
- [ ] Domain events/notifications defined per the design's event table and
      enqueued with `river.InsertTx` inside the same transaction.
- [ ] UI copy lives in i18n (es), not hardcoded strings; errors use stable
      codes.
- [ ] Accessible screen: labels, focus order, contrast — checked on iOS,
      Android and web where applicable.
- [ ] `audit_log` entry added if the action is sensitive (money, votes,
      personal data, permissions).
- [ ] Short documentation added to the module's README and, if
      applicable, the runbook.

### Security review checklist (mandatory for every PR)

- [ ] Decoders use `DisallowUnknownFields` and `huma` validation tags.
- [ ] Membership/tenant scope is checked on every new or changed endpoint.
- [ ] No sensitive data appears in logs, error responses or push payloads.
- [ ] Secrets stay out of the code (verified locally; `gitleaks` in
      `security.yml` is the CI backstop).
- [ ] Migration does not remove or weaken any append-only constraint.
- [ ] `security.yml` is green (gitleaks, govulncheck, gosec, Semgrep,
      `pnpm audit --audit-level=high`, Trivy).
- [ ] Permission-matrix test passes for every new/changed route.

### Second-person review required?

- [ ] This PR touches auth, files, money, votes or personal data → apply
      the `security-review` label and request a second reviewer.
- [ ] None of the above applies.

## Rollback plan

<!-- Required for any change touching auth, money, votes, files or
     personal data (PRD DoD, proposal rule). -->
