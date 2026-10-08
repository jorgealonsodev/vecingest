> Chained pull request 7 of 9 (slice 6). Base branch:
> `feature/m1-communities-pr5-turnstile`. It must not be merged before its
> base.

## Summary

Hardens the existing MFA enrollment endpoints from slice 5 and closes two
invitation-accept findings. No route is added.

- Email-confirmed TOTP enrollment: a password plus the caller's own
  authenticator no longer activates a factor. `POST /v1/me/mfa/verify` also
  requires a six-digit code emailed to the address stored on the account
  (`domain/auth/mfa/enroll_email.go`). Without it, a password alone could bind
  an attacker's authenticator to an account that had no factor yet. Every
  rejected confirmation returns the single code
  `AUTH_MFA_ENROLLMENT_CONFIRMATION_INVALID`, whichever factor was wrong.
- The code is delivered through a River `mfa_enroll_email` job enqueued with
  `InsertTx`, with the code sealed in the job row; the worker skips a
  challenge that is no longer open. Issuance is capped per user in a sliding
  window, and enrollment writes are serialized on the user row.
- Migration `00011` widens the `otp_challenges.purpose` check to accept
  `mfa_enroll`.
- Invitation accept (`c6fe3a7`, `ef7e825`): an existing account's matching
  credential is no longer re-validated against the current password policy,
  and a policy-violating attempt is recorded on both the create and the
  linking branch so they answer and lock out identically.

- New OpenAPI `operationId`s: none. Hardens the existing MFA enrollment
  endpoints.
- Schema migrations: `api/migrations/schema/00011_otp_mfa_enroll_purpose.sql`.
- Added test assertions on forbidden/not-found status: 0 / 0. This slice adds
  no route, so it adds no membership 403 coverage.

### What the 8 test files in this diff assert

Four test files are new and four existing ones are modified:

- `domain/auth/mfa/enroll_email_test.go` (new): the enrollment code is six
  decimal digits with leading zeros kept, and code matching is keyed and
  exact.
- `http/api/mfa_enroll_email_test.go` (new): a valid TOTP without the email
  code is rejected; an expired code is rejected; the sixth attempt is
  rejected even with the correct code; re-enrolling invalidates the prior
  code; both factors right activates the factor and spends the challenge; the
  job targets the stored address with a sealed code; issuance is capped per
  window with 429 and `Retry-After`, and the cap forgets codes outside the
  window.
- `mail/templates_test.go` (new): the enrollment email includes the code and
  states the lifetime passed in, so it cannot drift from the challenge TTL.
- `platform/queue/mfa_enroll_internal_test.go` (new): the worker opens the
  sealed code before sending, skips a challenge that is superseded, expired,
  verified or missing, and fails for retry when the challenge lookup fails.
- `http/api/invitation_test.go` (modified, three new tests): accept does not
  apply the password policy to an existing account's correct password; a
  policy violation is indistinguishable across the create and linking
  branches; and both branches lock out identically.
- `http/api/mfa_test.go` and `http/api/mfa_session_gate_test.go` (modified):
  existing verify calls now send the emailed code; the session-gate test
  asserts activation still opens nothing for a password-only session even
  when the attacker also controls the mailbox.
- `http/api/api_integration_test.go` (modified): wires the
  `MFAEnrollQueue` into the test server.

### Commits

- `c6fe3a7` fix(m1-communities): stop re-validating an existing credential against the password policy on invitation accept
- `ef7e825` fix(m1-communities): record policy-violating accept attempts on the create branch so both branches lock out identically
- `3b7d6cb` feat(m1-communities): add the mfa_enroll otp purpose and the challenge queries enrollment confirmation needs
- `74f1ccd` feat(m1-communities): require an emailed code to confirm a totp enrollment
- `311c4fd` feat(m1-communities): cap mfa enrollment email codes per user and serialize enrollment writes on the user row
- `ce447d3` fix(m1-communities): skip mailing an enrollment code whose challenge is no longer open and render its lifetime from the ttl
- `cfb1a35` docs(m1-communities): specify email-confirmed totp enrollment and its issuance cap
- `f5434cf` docs(m1-communities): record review outcomes for mfa enrollment email confirmation

### Changes

| Area | Files | Lines |
|---|---|---|
| Domain, queue and mail (`mfa/enroll_email.go`, `queue/mfa_enroll.go`, `mail/templates*`) | 6 | +313/-7 |
| `api/internal/db/` (`otp_challenges` and `users` queries + sqlc generated code) | 5 | +219 |
| `api/internal/http/` (`handlers/mfa.go`, `handlers/invitations.go`, DTO, `apperr`, deps) | 5 | +208/-34 |
| Generated contract (`api/openapi/openapi.yaml`, `packages/shared` client, schemas, errors) | 4 | +13 |
| `api/migrations/schema/00011_otp_mfa_enroll_purpose.sql` | 1 | +33 |
| `api/cmd/vecingest/serve.go` | 1 | +2 |
| Tests (listed above) | 8 | +841/-9 |
| Docs (`odd/tasks/mfa-enroll-email-confirmation.md`, `auth-mfa-totp` spec) | 2 | +138/-1 |

### Size

32 files, +1767/-51 (`git diff --shortstat 9ba4715...f5434cf`). This exceeds
the 400-line review budget; shrinking it would require rewriting commits that
carry burned review authority, so the slice is delivered as recorded.

### Verification

`ci` run 37750366525 passed at `f5434cf`, this pull request's head commit.
`security` run 37750366511 at the same commit failed (see CI status note).

A passing `ci` is not evidence for any checklist item below, and it does not
make this slice merge-ready. `ci` succeeds when every job either succeeded or
was skipped by its path filter (the `ci-required` job in
`.github/workflows/ci.yml`), and the job-level results of this run are not
cited in this body. The reviewed commits are not amended or rebased, because
they carry burned native review authority.

### CI status note

`security` run 37750366511 at `f5434cf` failed. The cause is diagnosed: four
jobs fail while `gosec` and `govulncheck` pass.

- `gitleaks`: the blocking whole-history scan reported 7 leaks, all one test
  fixture value on seven lines of `api/internal/http/api/invitation_test.go`
  at commit `a6976ab` (slice 4), a well-known public example passphrase used
  as a test request body, not a credential.
- `pnpm audit --audit-level=high`: 20 vulnerabilities, 12 high and 1
  critical, in transitive npm dependencies.
- `semgrep`: 1 blocking finding. This slice's own commit `311c4fd` introduces
  the defect `c2da7da` later fixes: `CountOTPChallengesIssuedSince` passed
  its issuance window through `make_interval(secs => ...::double precision)`,
  which made sqlc generate a floating-point parameter under `api/internal/db`,
  where the project's own `no-float-money-go` rule forbids one.
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
  - Not checked: the `email_code` field is added to `openapi.yaml`, the TS
    client and the sqlc code in this diff, but no `make gen` dirty-diff
    result is cited for this slice.
- [ ] `goose` migration reviewed (expand/contract if it touches existing
      data); `.sql` queries filter by their tenant scope column.
  - Not checked: `00011` only widens a check constraint and its `Down`
    deletes `mfa_enroll` rows before narrowing it again, but no review record
    is cited here and no `make lint-scope` result at this slice is cited.
- [ ] Membership middleware present on every new route, with a test
      confirming another role/community gets 403.
  - Not applicable: no route is added (0 forbidden / 0 not-found assertions
    added).
- [ ] Unit tests for the service and at least one e2e test of the main
      flow.
  - Not checked: unit tests and DB-backed API tests are added (listed
    above), but no test result at this slice is cited.
- [ ] Domain events/notifications defined per the design's event table and
      enqueued with `river.InsertTx` inside the same transaction.
  - Not checked: the `mfa_enroll_email` job is enqueued with
    `Client.InsertTx` on the caller's transaction, but conformance to the
    design's event table was not verified.
- [ ] UI copy lives in i18n (es), not hardcoded strings; errors use stable
      codes.
  - Not checked: a stable code `AUTH_MFA_ENROLLMENT_CONFIRMATION_INVALID` is
    added to `apperr` and `packages/shared/src/errors.ts`; no app UI changes,
    and the email template copy was not verified against i18n.
- [ ] Accessible screen: labels, focus order, contrast — checked on iOS,
      Android and web where applicable.
  - Not applicable: no screen in this slice.
- [ ] `audit_log` entry added if the action is sensitive (money, votes,
      personal data, permissions).
  - Not checked: no audit entry is added; whether issuing an enrollment code
    needs one was not decided here.
- [ ] Short documentation added to the module's README and, if
      applicable, the runbook.
  - Not checked: the `auth-mfa-totp` spec and an ODD task file are updated;
    no module README or runbook entry is added.

### Security review checklist (mandatory for every PR)

- [ ] Decoders use `DisallowUnknownFields` and `huma` validation tags.
  - Not checked: not verified for this slice.
- [ ] Membership/tenant scope is checked on every new or changed endpoint.
  - Not checked: the changed MFA endpoints are identity-scoped, and no run
    result cited at this slice confirms their behavior.
- [ ] No sensitive data appears in logs, error responses or push payloads.
  - Not checked: the enrollment code is sealed in the job row and rejections
    share one code, but the slice was not verified as a whole for this item.
- [ ] Secrets stay out of the code (verified locally; `gitleaks` in
      `security.yml` is the CI backstop).
  - Not checked: no local secret scan was run for this slice, and the
    `gitleaks` backstop fails on this pull request (see CI status note).
- [x] Migration does not remove or weaken any append-only constraint.
  - Evidence: `00011_otp_mfa_enroll_purpose.sql` only drops and re-adds the
    `otp_challenges_purpose_check` with `mfa_enroll` added; it does not
    reference `audit_log` or the append-only guard (`grep -ci` for
    `audit_log|append` returns 0).
- [ ] `security.yml` is green (gitleaks, govulncheck, gosec, Semgrep,
      `pnpm audit --audit-level=high`, Trivy).
  - Not checked: `security` run 37750366511 at `f5434cf` failed on four
    jobs (`gitleaks`, `pnpm audit`, `semgrep`, `trivy`); the fixes exist only
    at the chain tip, in pull request #9 (see CI status note).
- [ ] Permission-matrix test passes for every new/changed route.
  - Not checked: the permission-matrix test does not exist yet at this slice
    (it arrives in slice 7).

### Second-person review required?

- [x] This PR touches auth, files, money, votes or personal data → apply
      the `security-review` label and request a second reviewer.
  - Justification: changes how a second factor is enrolled and confirmed,
    emails codes to users' stored addresses, and changes invitation-accept
    credential and lockout handling.
  - The `security-review` label does not currently exist in this repository.
    It has to be created, or this requirement is tracked here in the body
    text. The label has not been applied.
- [ ] None of the above applies.

## Rollback plan

Revert order: revert this slice only after every later slice of the chain that
has been merged (slices 7 and 8) has been reverted, newest first. Then revert
this slice's merge commit on the branch it landed on.

Security impact of a revert: it reinstates password-only TOTP enrollment (the
account-takeover path `00011`'s header describes) and the two invitation-accept
defects fixed by `c6fe3a7` and `ef7e825`. Prefer a forward fix; revert only if
the email confirmation itself is broken.

Schema: the preferred rollback is code-only and keeps
`00011_otp_mfa_enroll_purpose.sql` in the tree, matching the version recorded
in `goose_db_version_schema`; the widened check is harmless to the reverted
code. If the migration must be undone, there is no scripted down path
(`vecingest migrate` applies up only): take a backup, roll back `00012` first if
applied, run the file's `-- +goose Down` block as `vecingest_owner` (it deletes
every `mfa_enroll` challenge, then restores the narrower check), and delete
version 11 from `goose_db_version_schema`.

Queue: pending `mfa_enroll_email` jobs should be cancelled before the worker
that consumes them is reverted.
