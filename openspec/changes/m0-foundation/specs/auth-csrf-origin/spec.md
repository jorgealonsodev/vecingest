# Auth CSRF and Origin Specification

## Purpose

Defense-in-depth for `/auth/refresh`: signed CSRF token, `Origin`
validation, and scoped cookie attributes, per PRD §5.1 (amended) and
research finding 6.

## Requirements

### Requirement: CSRF Token Delivered In-Body, Never By Cookie

For the cookie-transport path (web), `/auth/refresh` MUST require a valid
signed (HMAC) CSRF token echoed in the `X-CSRF-Token` request header. The
token MUST be issued in the JSON body of the login and refresh responses —
never set or readable via a cookie — because the web client is served from
`app.DOMAIN` while the refresh cookie is scoped to `api.DOMAIN`, so
`document.cookie` can never read a cookie-scoped token. The web client MUST
hold the CSRF token in memory only, exactly as the access token
(PRD §5.1: "el access token vive solo en memoria"). A cookie-transport
request with a valid refresh cookie but a missing or invalid CSRF header
MUST be rejected with 403 (PRD 10.1 gate item 3).

#### Scenario: Login response carries the CSRF token in the body

- WHEN a web client logs in successfully
- THEN the response body includes a CSRF token, and no cookie carries it

#### Scenario: Valid cookie, missing CSRF header

- GIVEN a request to `/auth/refresh` with a valid refresh cookie and no
  `X-CSRF-Token` header
- WHEN the request is processed
- THEN the system responds 403

#### Scenario: Valid cookie and valid CSRF header

- GIVEN a request with a valid refresh cookie and a matching signed
  `X-CSRF-Token` header, previously issued in a login or refresh response
  body
- WHEN the request is processed
- THEN the system proceeds to rotate the refresh token

#### Scenario: Native body-transport path requires no CSRF token

- GIVEN a native client presenting the refresh token in the request body,
  with no refresh cookie present
- WHEN the request is processed
- THEN the system proceeds without requiring a CSRF token, because there
  is no ambient cookie authority for a cross-site attacker to abuse

### Requirement: Origin Validation on Refresh

`/auth/refresh` MUST validate the `Origin` header against the configured
allowed origin and reject a mismatching or foreign origin with 403
(PRD 10.1 gate item 3).

#### Scenario: Foreign origin rejected

- GIVEN a request to `/auth/refresh` with `Origin: https://attacker.example`
- WHEN the request is processed
- THEN the system responds 403

### Requirement: CSRF Token Recovery Endpoint

`GET /v1/auth/refresh/csrf` MUST restore the CSRF token after a page
reload, at a path under the refresh cookie's `Path=/v1/auth/refresh` so
the cookie is actually attached (RFC 6265 §5.1.4 path-match). It MUST be
a safe method with no side effect and MUST NOT rotate the refresh token.
It MUST respond 401 when the refresh cookie is missing, expired, or
revoked. Successful responses MUST include `Cache-Control: no-store`,
`Vary: Origin`, and `Cross-Origin-Resource-Policy: same-origin` — all
three are required, not optional hardening: `Vary: Origin` prevents a
shared cache from serving the `app.DOMAIN` response variant (carrying
`Access-Control-Allow-Credentials: true`) to a request from another
origin, which would otherwise be a cache-mediated CORS bypass.

#### Scenario: Restores the CSRF token with a valid refresh cookie

- GIVEN a valid, unexpired refresh cookie
- WHEN `GET /v1/auth/refresh/csrf` is called
- THEN it responds 200 with a CSRF token bound to that session and does
  not rotate the refresh token

#### Scenario: Missing or invalid refresh cookie rejected

- GIVEN no refresh cookie, or one that is expired or revoked
- WHEN `GET /v1/auth/refresh/csrf` is called
- THEN it responds 401

#### Scenario: Response headers prevent cache-mediated CORS bypass

- WHEN `GET /v1/auth/refresh/csrf` responds successfully
- THEN the response includes `Cache-Control: no-store`, `Vary: Origin`,
  and `Cross-Origin-Resource-Policy: same-origin`

### Requirement: Refresh Cookie Scope and Flags

The web refresh cookie MUST be `HttpOnly`, `Secure`, `SameSite=Strict`,
scoped with `Path=/v1/auth/refresh`, and set on `Domain=api.<DOMAIN>` only
(PRD 10.1 gate item 3).

#### Scenario: Cookie attributes on login

- WHEN a web client logs in successfully
- THEN the `Set-Cookie` response for the refresh token includes
  `HttpOnly`, `Secure`, `SameSite=Strict`, and `Path=/v1/auth/refresh`
