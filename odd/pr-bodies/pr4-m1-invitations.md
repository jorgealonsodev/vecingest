> Chained pull request 5 of 9 (slice 4). Base branch:
> `feature/m1-units-csv-import`. It must not be merged before its base.

## Summary

Adds community invitations on top of the `invitations` table created in
slice 1, and fixes the missing grants on River's sequences.

- Invitations: create, list, preview, accept, resend and revoke. Invitations
  carry a short code (`domain/invitations/shortcode.go`); the invitation
  email job is enqueued through River with `InsertTx` on the request
  transaction (`platform/queue/invitations.go`), and an HTML template is
  added under `api/internal/mail/templates/`.
- `api/cmd/lintscope` gains a query-level exception map with one entry,
  `invitations.sql:SweepExpiredInvitations`, the intentionally cross-tenant
  expiry sweep.
- River sequence grants (`604f6a5`): migration `00004` granted the `river_*`
  tables but not the sequences behind their identity columns, which surfaced
  as `permission denied for sequence river_job_id_seq` once the invitation
  email became the first real job producer. The grant is added as a new Go
  goose migration rather than by editing `00004`, so it also reaches the
  already-deployed database. This commit is one of the later fixes the earlier
  slices depend on.

- New OpenAPI `operationId`s: `createInvitation`, `listInvitations`,
  `previewInvitation`, `acceptInvitation`, `resendInvitation`,
  `revokeInvitation`.
- Schema migrations: no `.sql` schema migration. This slice does add the Go
  goose migration `api/migrations/schema/00009_river_sequence_grants.go`
  (registered in `api/migrations/schema/schema.go`), which only grants and,
  on `Down`, revokes `USAGE, SELECT` on the `river_*` sequences for `app_rw`.
- Added test assertions on forbidden/not-found status: 4 / 13.

### Known defects at this slice's tip, fixed in slice 5

This slice is historically accurate, not final. Slice 5
(`feature/m1-communities-pr5-turnstile`) corrects review findings against this
code, among them:

- `4e45d03`: the worker built its River client with `LogMailer`, a log-only
  stand-in, so invitation email jobs decrypted the short code, rendered the
  message, and returned `nil` without sending anything. The job was recorded
  completed, never retried and never dead-lettered, while the API answered 201
  and `resend` incremented `sent_count`. Invitation email was therefore never
  delivered at this slice. `4e45d03` wires the real
  `internal/mail.AsyncMailer` and makes `SMTP_URL`/`MAIL_FROM` declared
  requirements of `CommandWorker`, so a mail-less worker cannot boot.
  To be precise about blast radius: this is a silent delivery failure, NOT a
  credential leak. `LogMailer.SendRaw` at this slice's tip logs only `subject`
  and `body_len`, and discards the recipient address explicitly
  (`api/internal/platform/mail/log_mailer.go:41`). Neither the short code nor
  the recipient was ever written to a log. The wording "wrote it to a log" in
  `4e45d03`'s own commit message overstates what the code did.
- `4b6c63f`: accept-invitation did not require credential proof, and the
  lockout was not keyed on the address.
- `55ffe48` and `14d7f15`: further accept-invitation, lockout and short-code
  handling corrections.

### Commits

- `a6976ab` feat(m1-communities): add invitations (Phase 6/8, WU-4/PR4)
- `604f6a5` fix(schema): grant River's sequences in a new migration, not by editing 00004

### Changes

| Area | Files | Lines |
|---|---|---|
| `api/internal/http/` (`handlers/invitations.go`, DTOs, deps, wiring) | 4 | +843 |
| `api/internal/db/` (queries + sqlc generated code) | 3 | +336/-11 |
| Generated contract (`api/openapi/openapi.yaml`, `packages/shared` client and schemas) | 3 | +938/-8 |
| Domain, queue and mail (`domain/invitations/`, `platform/queue/`, `mail/templates*`) | 6 | +248/-1 |
| `api/internal/authz/` (invitation scope resolution and registration) | 3 | +73 |
| `api/cmd/` (`lintscope/main.go`, `vecingest/serve.go`, `vecingest/worker.go`) | 3 | +50/-1 |
| `api/migrations/schema/` (`00009_river_sequence_grants.go`, `schema.go`) | 2 | +83 |
| Tests (`invitation_test.go`, `invitation_register_test.go`, `register_test.go`, `api_integration_test.go`, `lintscope/main_test.go`, `queue_test.go`) | 6 | +877/-1 |
| `openspec/changes/m1-communities/` (`apply-progress.md`, `tasks.md`) | 2 | +146/-22 |

### Size

32 files, +3594/-44 (`git diff --shortstat 4970237...604f6a5`). This exceeds
the 400-line review budget; shrinking it would require rewriting commits that
carry burned review authority, so the slice is delivered as recorded.

### Verification

No per-slice verification run exists for this slice. No test, linter or CI
run has been executed against `604f6a5` in isolation. Any CI failure on this
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
  - Not checked: `00009` only grants privileges, but `make lint-scope` was
    not run on the new queries at this slice and no review record is cited
    here.
- [ ] Membership middleware present on every new route, with a test
      confirming another role/community gets 403.
  - Not checked: `invitation_test.go` adds 4 forbidden and 13 not-found
    assertions, but it was not run at this slice.
- [ ] Unit tests for the service and at least one e2e test of the main
      flow.
  - Not checked: registration unit tests and DB-backed API tests exist, but
    nothing was run at this slice.
- [ ] Domain events/notifications defined per the design's event table and
      enqueued with `river.InsertTx` inside the same transaction.
  - Not checked: the invitation email job is enqueued with
    `river.Client.InsertTx` on the caller's transaction (design D-6), but at
    this slice the worker consumed the job with the log-only `LogMailer` and
    completed it without delivering any mail (fixed by `4e45d03` in slice 5).
- [ ] UI copy lives in i18n (es), not hardcoded strings; errors use stable
      codes.
  - Not checked: no app UI in this slice; the invitation email template copy
    and the error codes were not verified against this item.
- [ ] Accessible screen: labels, focus order, contrast — checked on iOS,
      Android and web where applicable.
  - Not applicable: no screen in this slice.
- [x] `audit_log` entry added if the action is sensitive (money, votes,
      personal data, permissions).
  - Evidence: every mutating operation calls `audit.Append` on the request
    transaction in `handlers/invitations.go`: `invitation.create`,
    `invitation.resend`, `invitation.revoke` and `invitation.accept`. Not
    exercised by a run at this slice.
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
  - Not checked: no log or response audit was run for this slice. The one
    logging path inspected directly does NOT leak:
    `LogMailer.SendRaw` logs only `subject` and `body_len` and never the
    recipient or the body, so the invitation short code is not written to a
    log (`api/internal/platform/mail/log_mailer.go:41`). A grep for a logged
    `code`, `secret` or `token` across `internal/queue`,
    `internal/http/handlers` and `internal/platform/mail` at this tip returns
    nothing. That is one inspected path, not a full audit, so this box stays
    unchecked.
- [ ] Secrets stay out of the code (verified locally; `gitleaks` in
      `security.yml` is the CI backstop).
  - Not checked: no local secret scan was run for this slice, and the
    `security.yml` backstop is failing on `main` (see CI status note).
- [x] Migration does not remove or weaken any append-only constraint.
  - Evidence: `00009_river_sequence_grants.go` only issues
    `GRANT USAGE, SELECT` on `river_*` sequences (and the matching `REVOKE` on
    `Down`); it does not reference `audit_log` or the append-only guard
    (`grep -ci` for `audit_log|append` returns 0).
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
  - Justification: invitation accept creates or links user accounts and
    grants community membership, and invitations store and email personal
    addresses.
  - The `security-review` label does not currently exist in this repository.
    It has to be created, or this requirement is tracked here in the body
    text. The label has not been applied.
- [ ] None of the above applies.

## Rollback plan

Revert order: revert this slice only after every later slice of the chain that
has been merged (slices 5 to 8) has been reverted, newest first; slice 5
changes `handlers/invitations.go` substantially (+328/-75). Then revert this slice's merge
commit on the branch it landed on.

Keep `604f6a5` and its migration `00009_river_sequence_grants.go`. To withdraw
the invitation feature, revert `a6976ab` only. `00009` is safe to leave
applied (`GRANT` is idempotent and only allows `app_rw` to use River's
sequences); removing the file while version 9 stays recorded in
`goose_db_version_schema` would leave an applied version with no file on disk,
and its Go `Down` is not reachable from any repository command
(`vecingest migrate` applies up only).

Data: invitation rows and their `audit_log` entries remain after a code
revert; the audit entries are append-only and must not be deleted. Pending
`invitation_email` jobs in River should be cancelled before the worker that
consumes them is reverted.
