# MFA enrollment email confirmation

## Objective

Require a one-time code, emailed to the account's stored address, to confirm a non-superadmin TOTP enrollment.

## Problem

TOTP is mandatory only for `admin`/`admin_staff` (`api/internal/authz/resolve.go:137`). Owners and tenants usually have no factor. Anyone holding such an account's password can log in, call `POST /v1/me/mfa/enroll` + `POST /v1/me/mfa/verify` with their own authenticator, and activate a factor. No disable or recovery path exists, so the legitimate owner is locked out permanently (round-5 finding `R1-totp-becomes-load-bearing-with-no-disable-or-recovery-path`).

## Why

Proving control of the account's mailbox at enrollment time closes the password-only takeover-by-enrollment path without building the full recovery flow, which the user deferred to the end of M1. This blocks deployment.

## Scope

- Enrollment issues a 6-digit email code bound to the user and purpose `mfa_enroll`.
- Confirmation (first `POST /v1/me/mfa/verify` while `enabled_at IS NULL`) requires both the TOTP code and the email code.
- Out of scope: TOTP disable, recovery-code login, app UI (deferred to end of M1).

## Constraints

- Never edit an applied migration; next migration is `00011`.
- Code is sent only to the stored `users.email`, never a client-supplied address.
- Code stored as HMAC-SHA256 under `Deps.MFAKey`; sealed with `mfa.EncryptSecret` inside any `river_job` payload.
- TTL about 10 minutes; at most 5 attempts per challenge; the attempt increment must survive a rejected confirmation.
- Re-enrolling invalidates every previous open `mfa_enroll` challenge.
- A failed confirmation returns one generic 401 without revealing which code was wrong.
- No AI attribution in commits; Conventional Commits.

## Tasks

- [x] T1 — Schema and queries: migration `00011_otp_mfa_enroll_purpose.sql` adds purpose `mfa_enroll` (drop/re-add CHECK, Down restores the 00006 list); queries to fetch the latest open challenge `FOR UPDATE` and to invalidate open challenges per user+purpose; `make gen`.
- [x] T2 — Delivery: River job `mfa_enroll_email` with sealed code, worker registered in `queue.NewClient`, template `mfa_enroll.html` + `RenderMFAEnrollCode`, wiring in `serve.go`/`Deps`.
- [x] T3 — Enroll and confirm: `EnrollMFA` issues the challenge in the same transaction as the pending secret; `confirmMFAEnrollment` requires `email_code`, enforces expiry/attempts/single use, marks the challenge verified before activation. Test-first; update existing MFA tests.
- [x] T5 — Issuance cap: limit how many `mfa_enroll` challenges one user can be issued per window, so re-enrolling cannot mint fresh attempt budgets (review finding: ~250 guesses/min via repeated enroll under the 300 req/min per-user limit). Test-first. Outcome: `mfa.EnrollEmailIssueLimit` = 5 per `mfa.EnrollEmailIssueWindow` = 1h (sliding, counted on `created_at` with the DB clock via new query `CountOTPChallengesIssuedSince`); past it, 429 `AUTH_TOO_MANY_ATTEMPTS` with `Retry-After: 3600`, reusing the existing code (no new error code, so openapi/shared unchanged). The count runs under a new users-row lock (`LockUserForMFAEnrollment`, `FOR NO KEY UPDATE`) so concurrent enrolls cannot all pass it; the already-active 409 check moved inside the same transaction. No migration needed.
- [x] T4 — Spec: add the email-confirmed enrollment scenario to `openspec/changes/m1-communities/specs/auth-mfa-totp/spec.md`. Outcome: new requirement "Email-Confirmed Enrollment" with five scenarios (missing/wrong email code, both valid, expired/exhausted/superseded, issuance cap, stale code not delivered); the admin enroll scenario now mentions the emailed code.
- [x] T6 — Review follow-up R3/R4 lock order: real. Enroll locked `user_mfa` (upsert) then `otp_challenges` (invalidate); confirm locked the challenge then `user_mfa` (activate) — a possible deadlock. Fixed in 311c4fd: both paths now lock the users row first, then `otp_challenges`, then `user_mfa` (enroll issues the challenge before upserting the secret). No deterministic test: the deadlock needs interleaved transactions; covered by the full suite staying green.
- [x] T7 — Review follow-up R4 stale codes on retry: real (the job carried no challenge identity). Fixed in ce447d3: job args carry `challenge_id`; the worker re-reads the challenge and completes without sending when it is missing, expired/superseded or verified; a lookup error fails the job so River retries. Tests: worker stub cases + HTTP test asserting the job names the issued challenge.
- [x] T8 — Review follow-up R2 TTL hardcoded in template: real. Fixed in ce447d3: `MFAEnrollCodeData.ExpiresInMinutes`, filled by the worker from `mfa.EnrollEmailTTL`; template renders it. Tests in `mail` and `queue`.
- [x] T9 — Review follow-up R2 attempt-cap literal in tests: real. Fixed in ce447d3: the HTTP tests use `mfa.EnrollEmailMaxAttempts`.
- Not changed (out of scope by decision): the invitations.go policy-infra finding, and "a wrong TOTP burns an email attempt" (intended: every failed confirmation counts).

## Acceptance criteria

- Password + valid TOTP code without the email code → 401, `enabled_at` stays NULL.
- Expired code, 6th attempt, and a code from a superseded enrollment are all rejected.
- Valid TOTP + valid email code → factor active, challenge marked verified, code unusable again.
- The enrollment email goes to the stored address.
- Existing MFA and session-gate tests pass after adaptation.

## Checks

- `go test -race -count=1 ./...` under `api/` (needs Docker)
- `PATH=$PATH:$HOME/go/bin make lint`
- `make gen` leaves no diff

## Route

Delegated direct: one writer (2+ non-trivial files across db, queue, mail, handlers, tests). Trigger: writer trigger.

## Delivery

Forecast about 500–700 authored changed lines. Strategy: `single-pr` on the existing branch with one work-unit commit per task group; RDD assess per commit against boundary `9ba4715`.

## Progress

- 2026-10-06: Design mapped (otp_challenges table exists unused). Document created.
- 2026-10-06: T1 committed as 3b7d6cb (delegated writer).
- 2026-10-06: T2+T3 implemented by the delegated writer. Independent read-only review: all constraints and acceptance criteria met; `go test -race -count=1 ./...` 31 packages ok, `make lint` 0 issues, `make gen` no diff. Review finding fixed inline: the expiry re-check under the challenge lock now reads the clock after the lock, so a re-enrollment committed between attempt consumption and the lock is rejected (no deterministic test: the window needs interleaved transactions; http tests 112 passed after the fix). Medium finding added as T5.
- 2026-10-06: T5 committed as 311c4fd (delegated writer; route: delegated direct, writer trigger). RED observed first (`TestMFAEnrollEmail_IssuanceIsCappedPerWindow`: 200 instead of 429), then GREEN. Includes the T6 lock-order fix (same users-row lock).
- 2026-10-06: T7–T9 committed as ce447d3. RED observed first (worker skip cases sent the stale code; template ignored the given lifetime), then GREEN.
- 2026-10-06: T4 spec + this document committed. Checks after the last code commit: `go test -race -count=1 ./...` 433 passed in 39 packages; `make lint` 0 issues (gofumpt clean, golangci-lint, biome, lint-compose OK); `make lint-scope` OK; `make gen` exit 0 with no diff.

- 2026-10-06: Native review of 74f1ccd (lineage review-479695069147b6d9): granted, approved, acknowledged. Its advisory findings became T5–T9.
- 2026-10-06: Native review of 311c4fd..cfb1a35 (lineage review-ca42364fdeb97d40, high risk, 16 files / 509 lines): granted, approved, acknowledged. Non-blocking advisories, all in the delivery worker `api/internal/platform/queue/mfa_enroll.go`: the open-challenge check reads `time.Now()` instead of an injectable clock; jobs without `challenge_id` (none exist, the feature is undeployed) are dropped silently; an exhausted challenge (attempts at the cap) is still mailed; the issuance-cap race has no deterministic test. Left as optional follow-ups.
- 2026-10-06: A whole-branch review against `main` (146 files, 22,770 lines) stopped with `lens_context_budget_exceeded`; no authority created. Branch-wide review needs smaller chained slices at PR time.

## Next step

All tasks done and reviewed. Optional: the worker advisories above. Push/PR remain the user's decision; a PR of the whole branch will need chained slices to be reviewable.
