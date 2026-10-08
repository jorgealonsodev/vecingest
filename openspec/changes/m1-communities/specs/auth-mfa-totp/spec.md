# Delta for Auth MFA TOTP

Base spec: `openspec/changes/m0-foundation/specs/auth-mfa-totp/spec.md`
(M0 is not archived yet). This delta only adds behavior; the existing
enrollment/verification/replay/recovery/throttling requirements for
`superadmin` are unchanged and reused as-is for the new non-superadmin
surface.

## ADDED Requirements

### Requirement: Non-Superadmin TOTP HTTP Endpoints

The system MUST expose TOTP enroll, verify, and recovery-code HTTP
endpoints for authenticated non-superadmin users, wiring the existing
`internal/domain/auth/mfa` domain logic already used for `superadmin`.
This capability's existing Enrollment, Verification Parameters, Replay
Protection, One-Time Recovery Codes, and Attempt Throttling requirements
apply identically to these endpoints.

#### Scenario: Admin enrolls TOTP via the non-superadmin endpoint

- GIVEN an authenticated `admin` user with no TOTP enrolled
- WHEN they call the non-superadmin TOTP enroll endpoint and verify a
  valid code
- THEN TOTP becomes active on their account

#### Scenario: admin_staff verifies TOTP via the non-superadmin endpoint

- GIVEN an authenticated `admin_staff` user with TOTP enrolled
- WHEN they submit a valid TOTP code to the non-superadmin verify endpoint
- THEN verification succeeds

### Requirement: Mandatory TOTP For Admin And Admin_staff Scope Access

A user with an `admin` or `admin_staff` membership MUST NOT be granted
access to any admin-scoped route until TOTP is enrolled and active on
their account (PRD §10.1 M1 gate item 3). This is independent of, and in
addition to, the existing mandatory-TOTP requirement for `superadmin`;
`owner`, `tenant`, `company`, and `worker` roles are unaffected and keep
TOTP optional.

#### Scenario: Admin without TOTP blocked from admin-scoped route

- GIVEN an `admin` user with no active TOTP
- WHEN they call any admin-scoped route
- THEN the system rejects the request and requires TOTP enrollment first

#### Scenario: Admin with active TOTP accesses admin-scoped routes normally

- GIVEN an `admin` user with active TOTP
- WHEN they call an admin-scoped route
- THEN the request is processed normally

#### Scenario: Owner without TOTP is unaffected

- GIVEN an `owner` user with no TOTP enrolled
- WHEN they call a unit-scoped route available to `owner`
- THEN the request is processed normally, since TOTP remains optional for
  that role
