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
  valid code together with the code emailed to their stored address
  (see Email-Confirmed Enrollment)
- THEN TOTP becomes active on their account

#### Scenario: admin_staff verifies TOTP via the non-superadmin endpoint

- GIVEN an authenticated `admin_staff` user with TOTP enrolled
- WHEN they submit a valid TOTP code to the non-superadmin verify endpoint
- THEN verification succeeds

### Requirement: Email-Confirmed Enrollment

A non-superadmin TOTP enrollment MUST be confirmed with a one-time
6-digit code emailed to the address stored on the account, in addition
to a valid TOTP code. There is no TOTP disable or recovery path yet, so
whoever activates the first factor holds it for good; the emailed code
proves control of the mailbox, so a password alone can no longer bind an
attacker's authenticator to the account. The code MUST be sent only to
the stored address, never to a client-supplied one, MUST expire after 10
minutes, MUST allow at most 5 confirmation attempts, and MUST be single
use. Every failed confirmation spends an attempt, whichever code was
wrong, and all failures return the same response. Re-enrolling MUST
supersede every previously issued code. Because each code carries its
own attempt budget, the system MUST issue at most 5 codes per user per
sliding hour and refuse further enrollments in that window with
`429 AUTH_TOO_MANY_ATTEMPTS` and a `Retry-After` header.

#### Scenario: Valid TOTP code without the emailed code is rejected

- GIVEN a user who started enrollment and holds the enrollment secret
- WHEN they submit a valid TOTP code with a missing or wrong email code
- THEN the system rejects the confirmation with the same generic error
  it returns for a wrong TOTP code
- AND TOTP stays inactive

#### Scenario: Both codes valid activates the factor

- GIVEN a user who started enrollment
- WHEN they submit a valid TOTP code and the code emailed to their stored
  address
- THEN TOTP becomes active, recovery codes are returned once, and the
  emailed code cannot be used again

#### Scenario: Expired, exhausted, or superseded code is rejected

- GIVEN a user whose emailed code expired, already took 5 failed
  attempts, or was issued for an enrollment they have since restarted
- WHEN they submit that code with a valid TOTP code
- THEN the system rejects the confirmation and TOTP stays inactive

#### Scenario: Enrollment code issuance is capped

- GIVEN a user who was issued 5 enrollment codes within the last hour
- WHEN they call the enroll endpoint again
- THEN the system responds `429 AUTH_TOO_MANY_ATTEMPTS` with a
  `Retry-After` header, and no new code is issued or emailed
- AND once those codes fall outside the one-hour window, enrolling
  succeeds again

#### Scenario: A stale code is not delivered

- GIVEN an enrollment email still queued for delivery
- WHEN its code was superseded, expired, or used before the email is sent
- THEN the email is not sent

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
