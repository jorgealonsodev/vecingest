# Delta for User Profile

Base spec: `openspec/changes/m0-foundation/specs/user-profile/spec.md`
(M0 is not archived yet; this delta modifies that text directly, per the
proposal's Capabilities note).

## MODIFIED Requirements

### Requirement: GET /v1/me Response Shape

`GET /v1/me` MUST return the caller's user id, email, `is_superadmin`, and
a `memberships` array. Each entry MUST be discriminated by a `scope` field
(`office` | `community`) and MUST include the resource id, resource name,
and role. Company memberships (`scope: company`) will join the same array
in M8 with no breaking change; M1 never returns a `company` entry since
`company_members` does not exist yet.
(Previously: returned only id, email, and is_superadmin, and explicitly
deferred memberships until M1's office/community schema existed.)

#### Scenario: Authenticated caller reads own profile

- GIVEN an authenticated non-superadmin user with no memberships
- WHEN they call `GET /v1/me`
- THEN the response contains their user id, email, `is_superadmin: false`,
  and an empty `memberships` array

#### Scenario: Superadmin caller

- GIVEN an authenticated superadmin
- WHEN they call `GET /v1/me`
- THEN the response includes `is_superadmin: true`

#### Scenario: User with one office membership and one community membership

- GIVEN a user who is `admin_staff` in office O and `owner` in community C
- WHEN they call `GET /v1/me`
- THEN `memberships` contains one entry with `scope: office`,
  `role: admin_staff`, and one entry with `scope: community`,
  `role: owner`

### Requirement: Remote Session Listing and Revocation Endpoints

The system MUST expose `GET /v1/me/sessions` to list the caller's own
sessions and `DELETE /v1/me/sessions/:id` to revoke one of them (PRD
§5.1). Revoking a session MUST NOT change the caller's `memberships` in
the `GET /v1/me` response: session state and membership state are
independent. The underlying session model is defined by the
`auth-session-tokens` capability.
(Previously: stated `GET /v1/me` returns only id/email/is_superadmin after
revocation, which no longer holds now that `memberships` is part of the
response; this requirement now asserts session state and membership state
are independent instead.)

#### Scenario: List sessions

- GIVEN an authenticated user with two active sessions
- WHEN they call `GET /v1/me/sessions`
- THEN the response lists both sessions with device and last-activity data

#### Scenario: Revoke a session leaves memberships unchanged

- GIVEN an authenticated user with an active session identified by `:id`
  and a non-empty `memberships` array
- WHEN they call `DELETE /v1/me/sessions/:id` and then `GET /v1/me`
- THEN the session is revoked and `memberships` is unchanged
