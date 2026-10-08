# Invitations Specification

## Purpose

Create/list/resend/revoke/preview/accept, explicit status, 14-day expiry,
single use, hashed storage, IP+device enumeration lockout, per §5.1, §7.3,
§7.4, and proposal Decision 3. Tenant column: `community_id`, resolved
from the invitation row for `:id`-addressed routes.

## Requirements

### Requirement: Invitation Creation With Hashed, One-Time-Visible Secrets

`POST /v1/communities/:id/invitations` MUST require `admin`/`admin_staff`
membership scoped to the community, MUST generate an 8-character short
code from an unambiguous alphabet (excluding `O`, `0`, `I`, `1`) via
`crypto/rand`, MUST return the plaintext short code (and token, when an
email link is also generated) exactly once in the creation response, and
MUST persist only `token_hash`/`short_code_hash` (SHA-256, both unique).
It MUST send an invitation email when `email` is present.

#### Scenario: Created invitation exposes the short code exactly once

- GIVEN an admin scoped to community C
- WHEN they call `POST /v1/communities/C/invitations`
- THEN the response contains the plaintext short code, and no later read
  of that invitation returns the plaintext again

#### Scenario: Stored invitation contains only hashes

- WHEN an invitation is created
- THEN its persisted row contains `token_hash`/`short_code_hash` and no
  plaintext token or code column

### Requirement: Explicit Status Column

`invitations.status` MUST be an explicit column with values `pending`,
`accepted`, `revoked`, `blocked`, maintained transactionally on every
state-changing action. `expires_at` remains authoritative for expiry: a
`pending` invitation whose `expires_at` has passed MUST be reported as
`expired` at read time, and a periodic job MUST sweep expired pending
invitations.

#### Scenario: Invitation created with status pending

- WHEN an invitation is created
- THEN its `status` is `pending`

#### Scenario: Accept transitions status transactionally

- GIVEN a `pending` invitation
- WHEN it is accepted
- THEN `status` becomes `accepted` in the same transaction that creates
  or links the membership

#### Scenario: Pending invitation past expiry reads as expired

- GIVEN a `pending` invitation whose `expires_at` is in the past and the
  sweep job has not yet run
- WHEN it is read
- THEN the API reports it as expired

### Requirement: Fourteen-Day Expiry And Single Use

An invitation MUST expire 14 days after creation and MUST be usable at
most once.

#### Scenario: Accept after expiry rejected

- GIVEN an invitation with `expires_at` in the past
- WHEN `POST /v1/auth/accept-invitation` is called against it
- THEN the system rejects it

#### Scenario: Second accept on an already-accepted invitation rejected

- GIVEN an invitation with `status = accepted`
- WHEN accept is called again
- THEN the system rejects it

### Requirement: Preview Endpoint Is POST, Never GET With A Query Credential

`POST /v1/invitations/preview` MUST accept `{token}` or `{short_code}` in
the request body and MUST return the community, unit, and role for
confirmation before account creation. The system MUST NOT expose a `GET`
route that accepts a token or short code as a query parameter, since a
credential in a query string leaks into access logs, proxies, and
`Referer`.

#### Scenario: Preview via POST body returns community/unit/role

- GIVEN a valid, unexpired invitation
- WHEN `POST /v1/invitations/preview` is called with its short code
- THEN the response includes the community, unit, and role, and no
  account is created

#### Scenario: No GET route accepts a token or short_code query parameter

- WHEN the registered API surface is enumerated
- THEN no `GET` operation declares `token` or `short_code` as a query
  parameter

### Requirement: Accept Creates Or Links An Account Without Revealing Prior Existence

`POST /v1/auth/accept-invitation` MUST accept `{token}` or `{short_code}`
plus `{name, password, phone?, consent}`. When no account exists for the
invitation's email, it MUST create the `user` and the `unit_member` in one
transaction; when an account already exists, it MUST link the membership
without duplicating the user. The response MUST NOT reveal whether the
invited email already had an account.

#### Scenario: Accept with no existing account creates user and membership

- GIVEN an invitation for an email with no account
- WHEN accept is called with valid data
- THEN a new `user` and a `unit_member` are created together

#### Scenario: Accept with an existing account links the membership

- GIVEN an invitation for an email that already has a `user` account
- WHEN accept is called with valid data
- THEN a `unit_member` is linked to the existing user, and no duplicate
  user is created

#### Scenario: Accept response shape is identical either way

- WHEN accept succeeds for an email with a pre-existing account and,
  separately, for one without
- THEN both responses have the same shape and reveal no difference

### Requirement: Resend And Revoke

`POST /v1/invitations/:id/resend` MUST increment `sent_count` and be
rate-limited. `DELETE /v1/invitations/:id` MUST set `status = revoked` and
MUST make the invitation unusable for preview or accept thereafter.

#### Scenario: Resend increments sent_count

- GIVEN a `pending` invitation
- WHEN resend is called
- THEN `sent_count` increases by one

#### Scenario: Revoked invitation fails preview and accept

- GIVEN an invitation revoked via `DELETE /v1/invitations/:id`
- WHEN preview or accept is attempted against it
- THEN both are rejected

### Requirement: Enumeration Lockout Is IP+Device Scoped, Not Per-Invitation

The system MUST lock out further `preview`/`accept` attempts from the same
IP+device pair after 10 failed short-code guesses within that pair's
window, using the existing `Limiter` interface — because a wrong guess
matches no invitation row and therefore cannot be seen by a per-invitation
counter. `invitations.failed_attempts` MUST separately record attempts
made against a resolved invitation, but MUST NOT be the mechanism that
enforces the 10-attempt lockout (PRD §10.1 M1 gate).

#### Scenario: Ten failed guesses from one IP+device lock further attempts

- GIVEN 10 failed short-code guesses from the same IP+device pair,
  against 10 different nonexistent codes
- WHEN an 11th guess is attempted from that same pair
- THEN the system rejects it as locked out

#### Scenario: failed_attempts alone does not enforce the lockout

- GIVEN a resolved invitation with `failed_attempts` incremented by wrong
  guesses arriving from 10 different IP+device pairs
- WHEN an 11th distinct pair attempts it
- THEN that pair is not locked out by the per-invitation counter alone

### Requirement: Cross-Tenant Isolation Proven By Test

An invitation created for community A MUST NOT be listable, resendable, or
revocable through a request scoped to community B; each such cross-
community attempt MUST return 403 or 404 (PRD §10.1 M1 gate —
"Aislamiento").

#### Scenario: Admin of community B cannot list community A's invitations

- GIVEN an invitation belonging to community A and an admin scoped to
  community B
- WHEN they call `GET /v1/communities/B/invitations`
- THEN community A's invitation is not present

#### Scenario: Admin of community B cannot revoke community A's invitation

- GIVEN an invitation belonging to community A and an admin scoped only
  to community B
- WHEN they call `DELETE /v1/invitations/:id` for that invitation
- THEN the system returns 403 or 404
