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
- [ ] T5 — Issuance cap: limit how many `mfa_enroll` challenges one user can be issued per window, so re-enrolling cannot mint fresh attempt budgets (review finding: ~250 guesses/min via repeated enroll under the 300 req/min per-user limit). Test-first.
- [ ] T4 — Spec: add the email-confirmed enrollment scenario to `openspec/changes/m1-communities/specs/auth-mfa-totp/spec.md`.

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

## Next step

T5 then T4 via one delegated writer.
