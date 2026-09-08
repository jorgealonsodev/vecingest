# Auth MFA TOTP Specification

## Purpose

TOTP enrollment, verification, and recovery per RFC 6238, mandatory for
`superadmin` at M0, per PRD §5.1.

## Requirements

### Requirement: TOTP Enrollment

The system MUST generate a 160-bit TOTP secret per user during enrollment
and MUST NOT consider TOTP active until the user verifies one valid code
against it.

#### Scenario: Enrollment requires verification

- GIVEN a user starting TOTP enrollment
- WHEN they submit an invalid code
- THEN TOTP remains inactive on their account

### Requirement: TOTP Verification Parameters

The system MUST verify TOTP codes per RFC 6238 with a 30-second period, 6
digits, and a tolerance of ±1 time step.

#### Scenario: Code from adjacent step accepted

- GIVEN a code valid for the immediately preceding 30-second step
- WHEN the user submits it within the drift window
- THEN the system accepts it

#### Scenario: Code outside drift window rejected

- GIVEN a code valid two steps in the past
- WHEN the user submits it
- THEN the system rejects it

### Requirement: TOTP Replay Protection

The system MUST NOT accept the same TOTP code twice within its validity
window (RFC 6238 §5.2).

#### Scenario: Replayed code rejected

- GIVEN a TOTP code already accepted once
- WHEN the same code is submitted again within its original window
- THEN the system rejects it

### Requirement: One-Time Recovery Codes

The system MUST issue single-use recovery codes at TOTP enrollment, and
MUST invalidate each code after one successful use.

#### Scenario: Recovery code reused

- GIVEN a recovery code already used once
- WHEN it is submitted again
- THEN the system rejects it

### Requirement: TOTP Attempt Throttling

The system MUST throttle TOTP and recovery-code verification attempts
independently of login-failure lockout (RFC 4226 §7.3), because a
verified first factor never touches the login counters
(`auth:fail:email:*`, `auth:fail:ip:*`). Failed TOTP verifications MUST
increment a per-user counter keyed `auth:fail:totp:<user_id>` on the same
`AttemptCounter` store, window, threshold, and escalation as login
lockout (5 failures in 15 minutes, escalating 15/30/60 minutes). Failed
recovery-code verifications MUST increment that same per-user counter, so
recovery codes are not a way around the TOTP budget. A successful TOTP
verification MUST reset the per-user counter and MUST NOT reset the
shared per-IP login counter.

#### Scenario: Fifth TOTP failure blocks the sixth attempt

- GIVEN 5 failed TOTP verifications for the same user within 15 minutes
- WHEN a 6th TOTP verification is attempted for that user
- THEN the system rejects it as throttled

#### Scenario: Recovery-code failures count toward the same budget

- GIVEN 4 failed TOTP verifications and 1 failed recovery-code
  verification for the same user within 15 minutes
- WHEN a 6th verification (TOTP or recovery code) is attempted for that
  user
- THEN the system rejects it as throttled

#### Scenario: Success resets the per-user counter

- GIVEN a user with 3 failed TOTP verifications recorded within the
  current window
- WHEN they submit a valid TOTP code
- THEN the per-user counter resets and a subsequent failed attempt is
  counted as the first of a new window

#### Scenario: Success does not reset the shared IP counter

- GIVEN failed login attempts from an IP have already accumulated toward
  the shared `auth:fail:ip` counter
- WHEN a user reachable from that IP successfully verifies TOTP
- THEN the shared per-IP counter is unchanged

### Requirement: Mandatory TOTP for Superadmin on a Separate Route

TOTP MUST be mandatory for the `superadmin` role from M0, and superadmin
login MUST use a route distinct from the regular login endpoint
(PRD 10.1 gate item 4).

#### Scenario: Superadmin without TOTP cannot complete login

- GIVEN a superadmin account with no TOTP enrolled
- WHEN they attempt to log in
- THEN the system requires TOTP enrollment before granting a session

#### Scenario: Superadmin login route is separate

- WHEN a superadmin authenticates
- THEN the request targets a login route distinct from `/v1/auth/login`
