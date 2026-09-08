# User Profile Specification

## Purpose

The authenticated caller's own profile endpoint at M0, before
office/community membership schema exists (M1), per PRD §7.4 and proposal
assumption 2.

## Requirements

### Requirement: GET /v1/me Response Shape

`GET /v1/me` MUST return the caller's user id, email, and `is_superadmin`.
It MUST NOT return a role or memberships at M0; the "usuario + membresías"
shape of PRD §7.4 applies from M1 onward, once office/community schema
exists.

#### Scenario: Authenticated caller reads own profile

- GIVEN an authenticated non-superadmin user
- WHEN they call `GET /v1/me`
- THEN the response contains their user id, email, and
  `is_superadmin: false`, and no `role` or `memberships` field

#### Scenario: Superadmin caller

- GIVEN an authenticated superadmin
- WHEN they call `GET /v1/me`
- THEN the response includes `is_superadmin: true`

### Requirement: Remote Session Listing and Revocation Endpoints

The system MUST expose `GET /v1/me/sessions` to list the caller's own
sessions and `DELETE /v1/me/sessions/:id` to revoke one of them; these
endpoints are confirmed in scope for M0 (PRD §5.1). They do not change the
`GET /v1/me` response shape: it MUST still return only `id`, `email`, and
`is_superadmin`. The underlying session model — family lineage, listing
query, and revocation — is defined by the `auth-session-tokens` capability.

#### Scenario: List sessions

- GIVEN an authenticated user with two active sessions
- WHEN they call `GET /v1/me/sessions`
- THEN the response lists both sessions with device and last-activity data

#### Scenario: Revoke a session leaves GET /v1/me unchanged

- GIVEN an authenticated user with an active session identified by `:id`
- WHEN they call `DELETE /v1/me/sessions/:id` and then `GET /v1/me`
- THEN the session is revoked and the `GET /v1/me` response still contains
  only `id`, `email`, and `is_superadmin`

### Requirement: GET /v1/me Performance Budget

`GET /v1/me` MUST serve at a p95 latency under 50 ms when measured on the
deployed server (PRD 10.1 gate item 6).

#### Scenario: p95 latency measured on deployed server

- GIVEN the API deployed on the target server
- WHEN `GET /v1/me` is load-tested with `k6`
- THEN the measured p95 latency is under 50 ms and the result is archived
  in `docs/security/gates/M0.md`
