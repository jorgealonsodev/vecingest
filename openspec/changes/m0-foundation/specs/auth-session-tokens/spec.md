# Auth Session Tokens Specification

## Purpose

JWT access/refresh issuance, rotation with family invalidation, transport
rules, and session tracking, per PRD §5.1.

## Requirements

### Requirement: JWT Access and Refresh Issuance

The system MUST issue a JWT access token valid for 15 minutes and a refresh
token valid for 30 days on successful authentication, except for
`superadmin` accounts, whose refresh token MUST be valid for 8 hours
(PRD §6.1).

#### Scenario: Successful login issues both tokens

- GIVEN a non-superadmin user
- WHEN they authenticate successfully
- THEN the response includes an access token expiring in 15 minutes and a
  refresh token valid for 30 days

#### Scenario: Superadmin login issues a short-lived refresh token

- GIVEN a `superadmin` account
- WHEN they authenticate successfully
- THEN the response includes an access token expiring in 15 minutes and a
  refresh token valid for 8 hours, not 30 days

### Requirement: Refresh Rotation with Family Invalidation

Each use of `/auth/refresh` MUST issue a new refresh token, invalidate the
previous one, and retain the family relationship between them. Presenting
an already-invalidated refresh token MUST revoke every token in that family
(PRD 10.1 gate item 2).

#### Scenario: Normal rotation

- GIVEN a valid refresh token
- WHEN it is used at `/auth/refresh`
- THEN a new refresh token is issued, the used one is invalidated, and both
  share the family id

#### Scenario: Reuse revokes the family

- GIVEN a refresh token already invalidated by a prior rotation
- WHEN it is presented at `/auth/refresh`
- THEN the system revokes every token in that family and rejects the
  request

### Requirement: Refresh Transport Mutual Exclusion

`/auth/refresh` MUST accept the refresh token by cookie (web) or by request
body (mobile), and MUST reject with `400 AUTH_AMBIGUOUS_TOKEN_TRANSPORT` a
request that carries both.

#### Scenario: Both cookie and body present

- GIVEN a request to `/auth/refresh` carrying a refresh token in both the
  cookie and the body
- WHEN the request is processed
- THEN the system responds `400 AUTH_AMBIGUOUS_TOKEN_TRANSPORT` without
  attempting rotation

### Requirement: Hashed Storage of Sensitive Tokens

Refresh tokens, password-reset tokens, and invitation tokens MUST be stored
as a SHA-256 hash, never in cleartext (PRD 10.1 gate item 2).

#### Scenario: Refresh token at rest

- WHEN a refresh token is persisted
- THEN the `sessions` row stores only its SHA-256 hash

### Requirement: Session Tracking and Remote Revocation

The system MUST record each session in the `sessions` table with device and
last-activity data, and MUST let the owning user list and revoke their
remote sessions via `GET /v1/me/sessions` and `DELETE /v1/me/sessions/:id`
(PRD §5.1; confirmed in scope for M0).

#### Scenario: User revokes a remote session

- GIVEN a user with two active sessions
- WHEN they call `DELETE /v1/me/sessions/:id` for one of them
- THEN that session is marked revoked and the other remains active

### Requirement: Bearer-Authenticated Logout

`POST /v1/auth/logout` MUST require a valid `Authorization: Bearer
<access-token>` and MUST be rejected without one; the refresh cookie's
`Path=/v1/auth/refresh` means it is never sent to `/v1/auth/logout`, so
logout MUST NOT accept or require a CSRF token — the request carries no
ambient authority for a cross-site attacker to abuse. Logout MUST take
`sid` (family id) from the verified access-token claims and revoke every
session in that family. The response MUST still clear the refresh
cookie by emitting `Set-Cookie` with the same cookie name,
`Path=/v1/auth/refresh`, and `Max-Age=0`, because setting a cookie is not
path-matched the way sending one is.

#### Scenario: Missing Bearer token rejected

- GIVEN a request to `POST /v1/auth/logout` with no `Authorization`
  header
- WHEN the request is processed
- THEN the system rejects it and revokes no session

#### Scenario: Valid Bearer token revokes the whole family

- GIVEN a valid access token whose claims carry `sid`
- WHEN `POST /v1/auth/logout` is called with that token
- THEN every session in that family is revoked

#### Scenario: No CSRF token required or checked

- GIVEN a valid access token and no `X-CSRF-Token` header
- WHEN `POST /v1/auth/logout` is called
- THEN the request succeeds without a CSRF token

#### Scenario: Refresh cookie cleared regardless of request path

- WHEN `POST /v1/auth/logout` succeeds
- THEN the response includes `Set-Cookie` for the refresh cookie's name
  with `Path=/v1/auth/refresh` and `Max-Age=0`

### Requirement: Immediate Session Revocation Effect

Revoking a session MUST reject that session's already-issued access token
immediately, not merely block a future refresh.

#### Scenario: Access token rejected right after revocation

- GIVEN a session with a currently valid, unexpired access token
- WHEN the session is revoked
- THEN the next request using that access token is rejected as
  unauthorized

#### Scenario: Degraded fallback after a LISTEN outage or fresh restart

- GIVEN the revocation cache's `LISTEN` connection has been down longer
  than the access-token TTL, or the process booted less than one TTL ago
- WHEN a request presents an access token belonging to a just-revoked
  session
- THEN the system falls back to an indexed `sessions` lookup by session id
  and still rejects the request, favoring correctness over latency
