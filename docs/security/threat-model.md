# vecingest — Initial Threat Model

Derived from PRD_go.md section 6.1 ("Seguridad: modelo de amenazas,
controles y acciones"). This document is the repository evidence for M0
security gate item 12 (PRD 10.1) and satisfies the platform-bootstrap spec's
"Initial Threat Model Document" requirement
(`openspec/changes/m0-foundation/specs/platform-bootstrap/spec.md`).

Target: **OWASP ASVS level 2** for the API and **OWASP MASVS-L1 + R**
(basic resilience) for the mobile/web app. The realistic attacker is not a
nation-state: it is a disgruntled resident, a former employee of a managing
firm, a credential-stuffing bot, or ransomware that gets a foothold on the
server.

## Critical assets (in order)

1. The database (identity + debts + IBAN).
2. Vote evidence and meeting minutes (`signature_evidence`, the audit hash
   chain).
3. Backups.
4. Keys (`ENCRYPTION_KEY`, JWT, R2, Expo/EAS).
5. The superadmin account.
6. The Expo/EAS account (can push code to every mobile device).
7. The Cloudflare account.
8. The server and Nginx Proxy Manager.

## Threats and controls

| Threat | Control |
|---|---|
| Credential stuffing and brute force | Argon2id, passwords checked against HIBP (with the fail-open exception below), progressive lockout, Turnstile (M1), mandatory 2FA for admin roles, new-login-location alerts |
| Session theft (XSS, lost device) | 15-minute access token in memory only, refresh token in a `Strict` + path-scoped cookie, rotation with reuse detection, remote session termination, CSP with no inline scripts |
| IDOR across communities or firms | `MembershipGuard` on every endpoint, scope-mandatory repositories, automated permission-matrix test calling every endpoint with every role and another tenant's resources, expecting 403/404 |
| Mass assignment | Separate input/output structs, `DisallowUnknownFields` decoding; the request body is never decoded directly onto the database model; output structs carry no `*_hash`/`*_encrypted` field |
| Malicious files (malware, SVG with script, polyglots) | Magic-byte allowlist, no SVG/HTML/ZIP, ClamAV in the worker, `Content-Disposition: attachment`, 5-minute download URLs, thumbnails generated and re-encoded client-side (M2 scope) |
| Injection (SQL, command, template) | `sqlc`-generated, always-parameterized queries (`fmt.Sprintf` in SQL forbidden by lint), `html/template` auto-escaping, PDF templates with no code evaluation, markdown sanitized with `bluemonday` |
| Vote/minutes tampering by an operator with DB access | Per-point hash chain plus `signature_evidence` and an external anchor: closing each meeting point seals `head_hash` with an RFC 3161 timestamp authority and emails it to the chair and secretary, so any later alteration is detectable by either of them (M7 scope; M0 ships chain construction and `VerifyChain` only, see design.md's Open Questions) |
| Tampering with the time-tracking log or `audit_log` | Append-only tables: the application's database role has no `UPDATE`/`DELETE`/`TRUNCATE` on `audit_log`, `votes`, `time_entries`, `signature_evidence`; chained hash and daily anchoring of `audit_log` |
| Leakage through logs or errors | PII redaction in the `slog` handler, generic errors to the client (stable `code`, detail server-side only), Sentry without PII, OpenAPI/Swagger UI disabled in production |
| IP spoofing to evade rate limits | `trust proxy` restricted to the Nginx Proxy Manager IP |
| SMS fraud (SMS pumping) | OTP only to phone numbers verified by an admin or the user, capped at 3 sends/hour and 10/day per number and account, premium-rate prefix blocking, alert if daily SMS spend exceeds a threshold (M7 scope) |
| Phishing using platform emails | SPF, DKIM (2048-bit), DMARC `p=reject` on the sending domain; emails never ask for a password or link to a third-party domain (M1 scope) |
| Server compromise (ransomware) | Backups encrypted with a separate key (`age`) to a write-only-credentialed, versioned/object-locked R2 bucket the API cannot delete from; monthly restore drills; the server exposes only 80/443 and key-based SSH (M3+ scope) |
| Compromise of a neighboring container on the shared NPM network | `db` on an internal-only network, `api` validates `Origin`/`Host`, containers are `read_only`, `no-new-privileges`, `cap_drop: ALL`, non-root, memory-limited |
| Supply chain (dependencies, images, OTA) | Renovate, `govulncheck` and `pnpm audit` (app) and Trivy in CI (blocking on a high-severity CVE), final image `distroless/static` with a statically linked binary, `go.sum` verified, EAS Update code signing, mandatory 2FA on GitHub/Expo/Cloudflare/the domain registrar |
| Abuse of public forms (company sign-up spam, contact) | Turnstile, per-IP limits, email verification before creating a `pending` record (M1 scope) |
| Exposure of debtors or third-party data | The debtor list only appears inside the meeting notice and minutes; downloads are logged; firms receive the minimum necessary (M5+ scope) |
| Improper access by platform staff | `superadmin` with 2FA from M0, a separate login route, 8-hour sessions, every action in `audit_log`, no direct production access except through a logged, key-based bastion |

## Key and secret management

- `ENCRYPTION_KEY` carries a version prefix (`v1:`) to support re-encryption
  rotation in the background. Portainer over standalone Docker has no
  `secrets:` primitive, so the key arrives as an environment variable; the
  compensating controls are: the app reads it at startup and removes it
  from `process.env` (`internal/config`, D-I), Portainer access requires
  2FA and admin role for exactly two people, and `docker.sock` is never
  exposed to another container.
- R2 credentials are scoped per bucket and per use: one read/write
  credential for the main bucket, a separate write-only credential for
  backups, and none with permission to delete the backup bucket.
- Annual rotation minimum for JWT (with `kid`), R2, SMTP and SMS
  credentials; immediate rotation on any suspicion.
- No secret is ever committed to the repository or baked into a Docker
  image — enforced by `gitleaks` in `security.yml`.

## Continuous verification

CI runs `govulncheck`, `gosec` (via `golangci-lint`'s embedded analyzer,
see the accepted-risk note below), `pnpm audit` (app), `gitleaks`, Trivy and
Semgrep (OWASP rules for Go and TypeScript), the permission-matrix test, and
a `DisallowUnknownFields` test on every decoder. All are blocking in
`security.yml`, plus a weekly schedule so a newly disclosed CVE against
unchanged code is still caught. See `docs/security/gates/M0.md` for the
archived evidence.

## Accepted residual risks

These are deliberate, reversible decisions the design and this batch's own
verification surfaced. Each carries compensating controls and is tracked
for revisit rather than silently ignored.

### 1. HIBP fail-open (design.md D-F)

The HIBP breached-password check (SHA-1 k-anonymity range query) **fails
open** on a transport error: a WARN log plus an `audit_log` entry
(`action = 'auth.hibp_unavailable'`) are recorded, and the request proceeds
under the length floor (15 chars without 2FA, 12 with TOTP) and Argon2id.

- **Risk accepted**: an attacker who can block egress to
  `api.pwnedpasswords.com` can register or reset to a breached password,
  provided it still clears the length floor.
- **Compensating controls**: the length floor, Argon2id, progressive
  lockout (D-N), and the `audit_log` entry, which makes a sustained outage
  visible instead of silent.
- **Why not fail closed**: failing closed would let a third-party outage
  block every password reset — trading a credential-quality risk for a
  self-inflicted availability outage on the account-recovery path, which
  design.md judges the worse of the two.

### 2. `golang.org/x/crypto/openpgp` — unmaintained subpackage (govulncheck GO-2026-5932)

`govulncheck` flags `golang.org/x/crypto@v0.56.0` at the **module** level
because the module contains the unmaintained, "unsafe by design"
`openpgp` subpackage. No fixed version exists (the subpackage's
deprecation is the fix).

- **Risk accepted**: none in practice — `govulncheck`'s own symbol-level
  analysis confirms `internal/domain/auth/password` (Argon2id) and
  `internal/platform/hibp` (SHA-1 range query) are the only consumers of
  `golang.org/x/crypto` in this codebase, and neither imports `openpgp`.
  Verified by running `govulncheck -show verbose ./...` from `api/` on
  2026-09-08: "Your code is affected by 0 vulnerabilities."
- **Compensating control**: none needed beyond the fact above; tracked so
  a future `x/crypto` major that removes `openpgp` is not mistaken for an
  unrelated breaking change.

### 3. `image-size` DoS, transitive via `metro` (CVE-2025-71329, CVE-2025-71330)

`pnpm audit` and Trivy both flag `image-size@1.2.1` (pulled in by
`metro@0.87.0`, the Expo/React Native bundler) for two denial-of-service
findings in its ICNS/JXL/HEIF parsers. No fixed version is published
upstream as of 2026-09-08 (`pnpm audit` reports "Patched versions: <0.0.0").

- **Risk accepted**: `image-size` only ever runs inside the local/CI build
  pipeline, measuring dimensions of this repository's own bundled assets.
  It never processes runtime, user-uploaded, or otherwise untrusted input —
  production file uploads are a separate Go/R2 pipeline (M2 scope, ships
  through `internal/domain` with its own magic-byte allowlist), not this
  Node bundler.
- **Compensating controls**: recorded in `pnpm-workspace.yaml`
  (`auditConfig.ignoreCves`) and `.trivyignore` with the same justification,
  so both scanners' otherwise-blocking gates reflect a reviewed acceptance
  rather than silent, undocumented suppression. Renovate tracks `metro`/
  `image-size` for a future fixed release (gate re-evaluated then).
