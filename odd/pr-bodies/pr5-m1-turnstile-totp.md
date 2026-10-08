> Chained pull request 6 of 9 (slice 5). Base branch:
> `feature/m1-invitations`. It must not be merged before its base.

## Summary

Adds Cloudflare Turnstile protection for public forms and TOTP for
non-superadmin accounts, makes the mandatory-TOTP gate a per-session fact, and
carries the corrections from successive adversarial review rounds against the
invitation flow from slice 4. This branch already existed on `origin` at
`9ba4715`; its pull request was never opened.

- Turnstile: a verifier in `platform/captcha/turnstile.go`, a captcha domain
  type and handler helper. Login requires a token after the third failure and
  forgot-password always requires one; both stay fail-closed through a
  verifier outage (`public_form_protection_test.go`).
  `TURNSTILE_SECRET` is required outside development and blocks the deploy
  when missing (`0bdbc5a`, `docs/pendientes-despliegue.md`, `env.example`).
- TOTP for non-superadmin accounts: `enrollMFA` and `verifyMFA`
  (`handlers/mfa.go`). Admin and `admin_staff` scope requires a session that
  completed a second factor: migration `00010` adds the nullable
  `sessions.mfa_at`, and the gate reads the session rather than the account
  (`f6c9118`).
- Invitation hardening (`4b6c63f`, `55ffe48`, `4e45d03`, `14d7f15`):
  credential proof on accept, lockout keyed on the address, authorization
  resolved on the primary, the short code sealed in the job row, the
  invitation email sent through the real mailer instead of a log, the code
  re-issued on resend, the captcha degradation path removed, and the accept
  transaction restructured around a written ordering invariant.
- Merge of `origin/main` (`9ba4715`): brings in `main`'s `f6c79eb`, which drops
  the weekly scheduled run from `.github/workflows/security.yml`. That hunk
  appears in this diff only because the base branch predates it; it is
  already on `main` and is not authored by this slice.

- New OpenAPI `operationId`s: `enrollMFA`, `verifyMFA`.
- Schema migrations: `api/migrations/schema/00010_sessions_mfa.sql`.
- Added test assertions on forbidden/not-found status: 25 / 13.

### Commits

- `3873794` feat(m1-communities): add Turnstile protection and non-superadmin TOTP (Phase 7/8, WU-5/PR5)
- `4b6c63f` fix(m1-communities): require credential proof on invitation accept, key the lockout on the address, make TOTP activation atomic
- `aee0345` docs(m1-communities): record the review-0e1833930adf141a correction in apply-progress and annotate task 7.14
- `55ffe48` fix(m1-communities): resolve authz on the primary, lock the account on invitation-accept guesses, seal the short code in the job row, and make Turnstile fail deliberately
- `0bdbc5a` docs(despliegue): TURNSTILE_SECRET es ahora obligatorio y bloquea el despliegue
- `f6c9118` fix(auth): make the admin TOTP gate check the session, not the account
- `4e45d03` fix(m1-communities): dispatch invitation email for real, gate Self scope on TOTP, make captcha degradation reachable, shrink the accept transaction, and key the short-code digest
- `14d7f15` fix(m1-communities): restructure accept-invitation around a written ordering invariant, delete the captcha degradation, re-issue the code on resend, and make the email job complete on delivery
- `f6c79eb` chore: drop the weekly scheduled run from the security workflow (from `main`)
- `9ba4715` Merge remote-tracking branch 'origin/main' into feature/m1-communities-pr5-turnstile

### Changes

| Area | Files | Lines |
|---|---|---|
| `api/internal/http/` (`handlers/mfa.go`, `captcha.go`, auth and invitation handlers, DTOs, `apperr`, wiring) | 13 | +856/-81 |
| Domain, platform, mail and config (`platform/captcha/turnstile.go`, `domain/auth/*`, `platform/queue/`, `mail/`, `config/`) | 12 | +353/-35 |
| `api/internal/authz/` (session MFA gate, `wiring.go`, Self scope) | 4 | +205/-2 |
| `api/internal/db/` (queries + sqlc generated code) | 8 | +144/-37 |
| Generated contract (`api/openapi/openapi.yaml`, `packages/shared` client, schemas, errors) | 4 | +294 |
| `api/migrations/schema/00010_sessions_mfa.sql` | 1 | +40 |
| `api/cmd/vecingest/` (`serve.go`, `worker.go`) | 2 | +97/-20 |
| `.github/workflows/security.yml` (from `main`'s `f6c79eb`) | 1 | +11/-5 |
| `app/src/screens/errorMessages.ts`, `env.example` | 2 | +5 |
| Tests (MFA, session gate, public-form protection, invitation and concurrency, Turnstile, lockout, mail, worker, migration upgrade) | 24 | +2834/-29 |
| Docs (`openspec/.../apply-progress.md`, `tasks.md`, `docs/pendientes-despliegue.md`) | 3 | +903/-16 |

### Size

74 files, +5742/-225 (`git diff --shortstat 604f6a5...9ba4715`). This exceeds
the 400-line review budget; shrinking it would require rewriting commits that
carry burned review authority, so the slice is delivered as recorded.

### Verification

`ci` run 37750363025 passed at `9ba4715`, this pull request's head commit.
`security` run 37750363138 at the same commit failed (see CI status note).

A passing `ci` is not evidence for any checklist item below, and it does not
make this slice merge-ready. `ci` succeeds when every job either succeeded or
was skipped by its path filter (the `ci-required` job in
`.github/workflows/ci.yml`), and the job-level results of this run are not
cited in this body. The reviewed commits are not amended or rebased, because
they carry burned native review authority.

### CI status note

`security` run 37750363138 at `9ba4715` failed. The cause is diagnosed and is
not specific to this slice: four jobs fail while `gosec` and `govulncheck`
pass.

- `gitleaks`: the blocking whole-history scan reported 7 leaks, all one test
  fixture value on seven lines of `api/internal/http/api/invitation_test.go`
  at commit `a6976ab` (slice 4), a well-known public example passphrase used
  as a test request body, not a credential.
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
carry. The `security.yml` hunk this slice carries from `main`'s `f6c79eb`
only drops the weekly schedule; it changes none of the six jobs.

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
  - Not checked: `00010` is additive and deliberately nullable so existing
    sessions stay un-elevated, and `sessions_mfa_upgrade_test.go` targets the
    upgrade path, but no `make lint-scope` result at this slice and no
    review record is cited here.
- [ ] Membership middleware present on every new route, with a test
      confirming another role/community gets 403.
  - Not checked: the slice adds 25 forbidden and 13 not-found assertions
    (scope registration, session gate, MFA, invitation and community tests),
    but no result for them at this slice is cited.
- [ ] Unit tests for the service and at least one e2e test of the main
      flow.
  - Not checked: unit tests (Turnstile, lockout, token, mail, queue) and
    DB-backed API tests exist, but no test result at this slice is cited.
- [ ] Domain events/notifications defined per the design's event table and
      enqueued with `river.InsertTx` inside the same transaction.
  - Not checked: the invitation email is enqueued with `InsertTx` on the
    rotation transaction and now delivered by the real mailer, but
    conformance to the design's event table was not verified.
- [ ] UI copy lives in i18n (es), not hardcoded strings; errors use stable
      codes.
  - Not checked: new error codes are added to `apperr` and
    `packages/shared/src/errors.ts`, and one Spanish message for
    `AuthMFARequired` to `app/src/screens/errorMessages.ts`, but i18n
    placement was not verified.
- [ ] Accessible screen: labels, focus order, contrast — checked on iOS,
      Android and web where applicable.
  - Not applicable: no screen is added; only error messages change.
- [ ] `audit_log` entry added if the action is sensitive (money, votes,
      personal data, permissions).
  - Not checked: factor activation writes an `mfa.enroll` entry
    (`handlers/mfa.go`), but audit coverage of the other changed sensitive
    paths (login second factor, password reset, accept) was not verified.
- [ ] Short documentation added to the module's README and, if
      applicable, the runbook.
  - Not checked: the deploy runbook `docs/pendientes-despliegue.md` gains the
    `TURNSTILE_SECRET` requirement, but no module README is added.

### Security review checklist (mandatory for every PR)

- [ ] Decoders use `DisallowUnknownFields` and `huma` validation tags.
  - Not checked: not verified for this slice.
- [ ] Membership/tenant scope is checked on every new or changed endpoint.
  - Not checked: no run result cited at this slice confirms it.
- [ ] No sensitive data appears in logs, error responses or push payloads.
  - Not checked: `4e45d03` removes the slice 4 log-only mailer that logged
    invitation short codes, but the slice as a whole was not verified for
    this item.
- [ ] Secrets stay out of the code (verified locally; `gitleaks` in
      `security.yml` is the CI backstop).
  - Not checked: no local secret scan was run for this slice, and the
    `gitleaks` backstop fails on this pull request (see CI status note).
- [x] Migration does not remove or weaken any append-only constraint.
  - Evidence: `00010_sessions_mfa.sql` only adds the nullable column
    `sessions.mfa_at` (and drops it on `Down`); it does not reference
    `audit_log` or the append-only guard (`grep -ci` for `audit_log|append`
    returns 0).
- [ ] `security.yml` is green (gitleaks, govulncheck, gosec, Semgrep,
      `pnpm audit --audit-level=high`, Trivy).
  - Not checked: `security` run 37750363138 at `9ba4715` failed on four
    jobs (`gitleaks`, `pnpm audit`, `semgrep`, `trivy`); the fixes exist only
    at the chain tip, in pull request #9 (see CI status note).
- [ ] Permission-matrix test passes for every new/changed route.
  - Not checked: the permission-matrix test does not exist yet at this slice
    (it arrives in slice 7).

### Second-person review required?

- [x] This PR touches auth, files, money, votes or personal data → apply
      the `security-review` label and request a second reviewer.
  - Justification: adds TOTP enrollment and a session-level second-factor
    gate, changes login, refresh and password reset, and hardens invitation
    accept and lockout.
  - The `security-review` label does not currently exist in this repository.
    It has to be created, or this requirement is tracked here in the body
    text. The label has not been applied.
- [ ] None of the above applies.

## Rollback plan

Revert order: revert this slice only after every later slice of the chain that
has been merged (slices 6 to 8) has been reverted, newest first; slice 6
builds directly on the MFA enrollment endpoints added here.

Reverting this slice while keeping slice 4 reinstates slice 4's known defects
(log-only invitation email that logged the short code, accept without
credential proof). Revert slice 4's invitation feature (`a6976ab`) together
with this slice, after it.

The `.github/workflows/security.yml` hunk comes from `main`'s `f6c79eb`. When
reverting, do not restore the weekly schedule unless that is decided
separately; once this slice's base contains `f6c79eb`, the revert no longer
includes that hunk.

Schema: `00010` adds a nullable column that the reverted code no longer reads,
so the preferred rollback is code-only and keeps
`00010_sessions_mfa.sql` in the tree, matching the version recorded in
`goose_db_version_schema`. If the column must be removed, there is no scripted
down path (`vecingest migrate` applies up only): take a backup, roll back
`00012` and `00011` first if applied, run the file's `-- +goose Down` block as
`vecingest_owner`, then delete version 10 from `goose_db_version_schema`.

Deployment: the reverted code no longer requires `TURNSTILE_SECRET`; leaving
it set is harmless. Enrolled `user_mfa` factors stay in the database.
