# Request Protection Specification

## Purpose

Trusted-proxy client IP resolution, the rate-limiting `Limiter`
abstraction, and `slog` PII redaction, per PRD §6, §6.1, §7.7, and research
findings 5 and 8.

## Requirements

### Requirement: Trusted-Proxy Client IP Resolution

The system MUST resolve the client IP from `X-Forwarded-For` using an
allowlist containing the reverse proxy's IP, walking the chain from the
right and skipping trusted hops to select the first untrusted entry. A
fixed hop count MAY be used only as a documented development convenience,
never as the production mechanism (PRD 10.1 gate item 10).

#### Scenario: Spoofed leftmost entry ignored

- GIVEN `X-Forwarded-For: 203.0.113.9, <trusted-proxy-ip>` where
  `<trusted-proxy-ip>` is on the allowlist
- WHEN the client IP is resolved
- THEN the system uses `203.0.113.9`, not a claimed value beyond the
  trusted hop

#### Scenario: Untrusted intermediary rejected as source

- GIVEN a request whose immediate peer IP is not on the trusted-proxy
  allowlist
- WHEN the client IP is resolved
- THEN the system uses the peer's actual connection IP, not a claimed
  `X-Forwarded-For` value

### Requirement: Limiter Interface with Rate Budgets

Rate limiting MUST be implemented behind a `Limiter` interface, enforcing
10 req/min on login and reset endpoints, 300 req/min per authenticated
user, and 60 req/min per IP on public endpoints (PRD §6).

#### Scenario: Login rate limit enforced

- GIVEN 10 login requests already made by one client within a minute
- WHEN an 11th login request arrives within that minute
- THEN the system responds 429

### Requirement: slog PII Redaction

The `slog` handler MUST redact email, phone, and IBAN values both when they
appear as structured attributes and when they are interpolated into a
message string (PRD 10.1 gate item 11).

#### Scenario: Structured attribute redacted

- WHEN a log call includes `slog.String("email", "user@example.com")`
- THEN the emitted log line does not contain the literal email

#### Scenario: Interpolated message redacted

- WHEN a log call emits the message `"user user@example.com logged in"`
- THEN the emitted log line does not contain the literal email
