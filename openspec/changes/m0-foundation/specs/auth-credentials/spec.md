# Auth Credentials Specification

## Purpose

Password-based authentication for vecingest: length policy conditioned on the
second factor, breach checking, Argon2id hashing, and progressive lockout,
per PRD §5.1 and §6.1.

## Requirements

### Requirement: Conditional Password Length Floor

The system MUST enforce a minimum password length of 15 characters for
accounts without an active second factor and 12 characters for accounts with
TOTP active (NIST SP 800-63B rev. 4). The system MUST return an error that
states which rule applies (PRD 10.1 gate item 1).

#### Scenario: Password too short without 2FA

- GIVEN a user without TOTP enabled
- WHEN they submit a 14-character password
- THEN the system rejects it with an error stating the 15-character rule

#### Scenario: Password accepted with TOTP active

- GIVEN a user with TOTP enabled
- WHEN they submit a 12-character password meeting all other rules
- THEN the system accepts it

### Requirement: HIBP k-Anonymity Breach Check

The system MUST reject passwords found in the Have I Been Pwned breach
corpus, querying the k-anonymity range API with the 5-character SHA-1
prefix, sending `Add-Padding: true`, and discarding response entries whose
count is 0 (PRD 10.1 gate item 1).

On an HIBP transport error, the check MUST fail open: the password is
accepted on that basis alone (the length floor and Argon2id hashing still
apply), the system MUST log a WARN, and MUST write an `audit_log` entry, so
a third-party outage cannot block registration or password recovery.

#### Scenario: Breached password rejected

- GIVEN a password whose SHA-1 hash suffix appears in the HIBP range
  response with count > 0
- WHEN the user submits it during registration or password reset
- THEN the system rejects the password as breached

#### Scenario: Padded zero-count entry discarded

- GIVEN an HIBP range response containing a padding entry with count 0
  matching the submitted hash suffix
- WHEN the system evaluates the response
- THEN it MUST treat that entry as absent and not reject the password on
  its account

#### Scenario: HIBP transport error fails open

- GIVEN the HIBP range API is unreachable or returns a transport error
- WHEN a user submits a password meeting the applicable length floor
- THEN the system accepts the password, logs a WARN, and writes an
  `audit_log` entry recording the HIBP check failure

### Requirement: Argon2id Password Hashing

The system MUST hash passwords with Argon2id using `m=19456 KiB, t=2, p=1`,
a 16-byte salt from `crypto/rand`, and a 32-byte tag, encoded as a PHC
string. The system MUST verify with `crypto/subtle.ConstantTimeCompare` and
MUST rehash on successful login when stored parameters differ from the
pinned ones (PRD 10.1 gate item 1).

#### Scenario: Password stored as PHC string

- WHEN a password is hashed
- THEN the stored value is a `$argon2id$v=19$m=19456,t=2,p=1$...` PHC string

#### Scenario: Rehash on parameter bump

- GIVEN a stored hash with outdated parameters
- WHEN the user logs in successfully
- THEN the system rehashes the password with current parameters before
  completing login

### Requirement: Progressive Lockout

The system MUST block login attempts after 5 failed attempts within 15
minutes, counted independently by email and by source IP, and MUST send an
email alert to the affected user (PRD §5.1).

#### Scenario: Lockout by email

- GIVEN 5 failed login attempts for the same email within 15 minutes
- WHEN a 6th attempt arrives with any IP
- THEN the system rejects it and has sent a lockout alert email

#### Scenario: Lockout by IP

- GIVEN 5 failed login attempts from the same IP within 15 minutes
  targeting different emails
- WHEN a 6th attempt arrives from that IP
- THEN the system rejects it

### Requirement: Enumeration-Safe Auth Responses

Login, password-recovery, and invitation-acceptance responses MUST NOT
reveal whether a given email is registered.

#### Scenario: Unknown email on forgot-password

- WHEN a forgot-password request targets an unregistered email
- THEN the response is identical in shape and status to a request for a
  registered email
