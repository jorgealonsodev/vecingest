# Public Form Protection Specification

## Purpose

`CaptchaVerifier` (Cloudflare Turnstile) and per-IP limits on the public
forms that exist at M1 — login after the third failure, forgot-password —
per §5.1 and proposal Decision 4. No tenant column: these routes are
unauthenticated and pre-membership.

## Requirements

### Requirement: CaptchaVerifier Interface Abstraction

The system MUST verify a Turnstile token through a `CaptchaVerifier`
interface before processing a protected public-form submission, so the
verifier implementation is swappable without changing callers.

#### Scenario: Invalid Turnstile token rejected before form logic runs

- GIVEN a request carrying an invalid Turnstile token
- WHEN it reaches a protected public form
- THEN the request is rejected before any form-specific logic executes

### Requirement: Turnstile Required After The Third Login Failure

The login endpoint MUST require a valid Turnstile token starting on the
third failed attempt for the same email or IP within the lockout window
(§5.1).

#### Scenario: Third failed login without Turnstile rejected

- GIVEN two prior failed login attempts for the same email within the
  window
- WHEN a third attempt is made without a Turnstile token
- THEN the system rejects it and requires Turnstile

#### Scenario: Third failed login with a valid Turnstile token proceeds

- GIVEN two prior failed login attempts for the same email
- WHEN a third attempt includes a valid Turnstile token
- THEN the system proceeds to normal credential verification

### Requirement: Turnstile Always Required On Forgot-Password

`POST /v1/auth/forgot-password` MUST always require a valid Turnstile
token, regardless of prior attempt count.

#### Scenario: Forgot-password without Turnstile rejected

- WHEN `POST /v1/auth/forgot-password` is called without a Turnstile token
- THEN the system rejects it

### Requirement: Per-IP Limits Independent Of Turnstile

Login and forgot-password MUST enforce per-IP rate limits behind the
existing `Limiter` interface, independent of Turnstile verification.

#### Scenario: Excessive requests from one IP rejected despite valid Turnstile

- GIVEN an IP that has exceeded its rate limit on forgot-password
- WHEN it submits another request with a valid Turnstile token
- THEN the system still rejects it for rate limiting

### Requirement: Company Registration And Contact Forms Out Of M1 Scope

The system MUST NOT ship `POST /v1/auth/register-company` or a public
contact-form endpoint in M1. Turnstile coverage for those forms is
deferred to the milestone that introduces them and is recorded as a scoped
exception in `docs/security/gates/M1.md`.

#### Scenario: register-company is not part of M1's registered operations

- WHEN the M1 API surface is enumerated
- THEN no operation for company self-registration exists
