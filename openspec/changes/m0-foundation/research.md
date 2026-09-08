---
change: m0-foundation
phase: research
project: vecingest
source: Engram observation 5244
date: 2026-09-04
---

SDD phase artifact — change `m0-foundation`, phase `research`, project `vecingest`, date 2026-09-04. Status: complete with gaps. All sources accessed 2026-09-04.

## 1. goose with three database roles
- A single goose `Provider`/`migrate` invocation binds to ONE `*sql.DB`/DSN — it runs statements and tracks `goose_db_version` on that one connection. No native multi-role pattern is documented.
- SQL migrations run only on that connection, in a transaction by default (`-- +goose NO TRANSACTION` to disable), and support `-- +goose ENVSUB ON` for textual env substitution — but the substituted statement still executes as the migration connection's role.
- **Go migrations are the mechanism that solves this**: `goose.AddMigrationContext(up, down GoMigrationContext)` with signature `func(ctx context.Context, tx *sql.Tx) error`, or `AddMigrationNoTxContext` with `func(ctx context.Context, db *sql.DB) error`. Being arbitrary Go, they can call `os.Getenv` (needed because `app_rw`'s password cannot be hardcoded in SQL) and can open a SEPARATE `database/sql` connection to `BOOTSTRAP_DATABASE_URL` to run statements as another role. Source: https://pkg.go.dev/github.com/pressly/goose/v3
- CAVEAT: this is a sound inference from the documented API's generality, NOT a goose-endorsed pattern quoted in its docs.
- **Locking is opt-in and OFF by default.** https://pressly.github.io/goose/documentation/provider/ states plainly: "By default, goose does not lock the database during migrations." Use `NewProvider(..., WithSessionLocker(lock.NewPostgresSessionLocker(...)))`, which takes a PostgreSQL session-level exclusive advisory lock, 5-minute acquire timeout, 1-minute unlock timeout, with retry. Since v3.26.0 table-based lockers also exist (`NewPostgresTableLocker`). Source: https://pkg.go.dev/github.com/pressly/goose/v3/lock
- Current version: v3 (`github.com/pressly/goose/v3`), latest tag v3.28.0 (2024-09-02). No v4. Docs now centre on the `NewProvider` API rather than the legacy functions.

## 2. Migrations from a distroless/static image
- `//go:embed migrations/*.sql` into an `embed.FS`, then `goose.SetBaseFS(fs)` (functional API) or pass the `embed.FS` as the `fsys` argument to `goose.NewProvider(...)`, and call Up from `main()` at startup. No goose binary and no shell needed. Source: https://pressly.github.io/goose/blog/2021/embed-sql-migrations/
- `embed.FS` is read-only, so only Up-style operations work at runtime; `goose create`/`fix` are dev-time-only against the real filesystem.
- Race prevention between `api` and `worker` booting together requires the opt-in session locker above — the second replica blocks, then finds the schema at target version and no-ops.
- PostgreSQL 17 advisory lock family (https://www.postgresql.org/docs/17/functions-admin.html): `pg_advisory_lock(key)` session-level, exclusive, blocking, held until unlock or session end, and requests on the same key STACK; `pg_try_advisory_lock` non-blocking; `pg_advisory_xact_lock` transaction-scoped, auto-released at COMMIT/ROLLBACK, no stacking; `pg_try_advisory_xact_lock` both. The blocking SESSION-level lock is correct here — a transaction-scoped lock releases too early relative to goose's connection lifecycle, especially for `NO TRANSACTION` migrations.

## 3. River
- Requires `river_job`, `river_leader`, `river_migration` and supporting objects. Job kinds need NOT exist first — the schema is generic, so it CAN be provisioned before any job type is written. Source: https://riverqueue.com/docs/migrations
- Two official ways to apply it: the `river` CLI (`river migrate-up --database-url ...`) or the `rivermigrate` library (`rivermigrate.New(...)` + `Migrate()`/`MigrateTx()`). River's docs show an explicit pattern for folding its migrations into goose via Go-migration files calling `rivermigrate` at a pinned River schema version.
- Documented constraint: River's full migration set CANNOT all run in one transaction — PostgreSQL forbids creating an immutable function in the same transaction that just modified a dependent enum type (its migration 6 depends on migration 4's enum change). Relevant if you assume goose's one-transaction-per-file model composes trivially with an embedded `rivermigrate` call.
- **GAP — unverified**: River's exact current release tag and its minimum PostgreSQL version. Fetches returned internally inconsistent dates (the same tag reported as both 2024-08-31 and 2026-08-31), almost certainly a fetch artifact. Docs state River is "tested using the three most recent major versions of PostgreSQL". Re-check the README version badge and the docs Requirements section directly before pinning.

## 4. Argon2id in Go
- `golang.org/x/crypto/argon2` exposes only `IDKey(password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte` and `Key(...)`, plus `Version` (0x13). Source: https://pkg.go.dev/golang.org/x/crypto/argon2
- It does NOT provide: PHC string encoding/decoding, any verification function, or constant-time comparison. It is a pure KDF primitive.
- A correct implementation must add: 16-byte salt from `crypto/rand` (never `math/rand`); `argon2.IDKey(pw, salt, 2, 19456, 1, 32)` for the pinned parameters; hand-built PHC string `$argon2id$v=19$m=19456,t=2,p=1$<b64 salt>$<b64 tag>` using unpadded `RawStdEncoding`; comparison via `crypto/subtle.ConstantTimeCompare`, never `bytes.Equal` or `==`; and version/param mismatch handling for rehash-on-login when parameters are bumped.
- `github.com/alexedwards/argon2id` implements exactly this layer and is a reasonable reference shape, but is third-party convenience, not stdlib.

## 5. httprate and trusted proxies — CONTRADICTION with the PRD's assumption
- **`httprate` does NOT derive or validate client IPs.** Its own helpers `LimitByRealIP`, `KeyByRealIP`, `WithKeyByRealIP` are DEPRECATED precisely because they read `True-Client-IP`/`X-Real-IP`/`X-Forwarded-For` with no proxy-trust check, letting an attacker spoof the rate-limit key — either to evade limits or to pin a victim's IP and get them 429'd. Source: https://github.com/go-chi/httprate/blob/master/README.md
- The IP-trust mechanism lives in **chi itself since v5.3.0 (2024-05-22)**: `middleware.ClientIPFromHeader(trustedHeader)`, `middleware.ClientIPFromXFF(trustedIPPrefixes ...netip.Prefix)`, `middleware.ClientIPFromXFFTrustedProxies(numTrustedProxies int)`, `middleware.ClientIPFromRemoteAddr`; install exactly one as middleware and read `middleware.GetClientIP(r.Context())`. That release also fixed three CVEs in the old `middleware.RealIP` — GHSA-9g5q-2w5x-hmxf, GHSA-rjr7-jggh-pgcp, and GHSA-3fxj-6jh8-hvhx (Critical, 9.3). `RealIP` is deprecated but not removed. Source: https://github.com/go-chi/chi/releases/tag/v5.3.0
- httprate's current recommended pattern DELEGATES to it: `httprate.LimitBy(100, time.Minute, func(r *http.Request) (string, error) { return httprate.CanonicalizeIP(middleware.GetClientIP(r.Context())), nil })`. `CanonicalizeIP` buckets IPv6 by /64 to stop trivial rotation.
- For exactly one reverse proxy at a known IP (NPM): `middleware.ClientIPFromXFF` with that host's IP as a /32 trusted prefix — it walks `X-Forwarded-For` right-to-left skipping trusted hops and takes the first untrusted entry. Use `ClientIPFromXFFTrustedProxies(1)` instead if the proxy IP varies across container restarts.
- With that wiring, a spoofed leftmost XFF entry is skipped as untrusted. Without it, there is NO safe default — omitting the step silently reverts to spoofable behaviour.

## 6. CSRF for a cookie-borne refresh token — CONTRADICTION with the obvious library choice
- OWASP CSRF Prevention Cheat Sheet (live): stateful → synchronizer token; stateless → double-submit cookie; a custom-header check is strong for APIs because it forces a CORS preflight a cross-site form post cannot trigger; `SameSite` is defence-in-depth only (blocks unsafe methods only, applies at registrable-domain not full-origin granularity, useless against client-side CSRF). Source: https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html
- Naive double-submit (raw random value in cookie + header) is called out as vulnerable to cookie injection from a sibling subdomain. The **signed/HMAC variant** is OWASP's recommended fix and the one to use.
- **`gorilla/csrf` must NOT be the default choice**: CVE GO-2025-3884 / CVE-2025-24358 / CVE-2025-47909 — its `TrustedOrigins` implicitly trusts both the HTTP and HTTPS variants of a listed host, letting a network MITM host a malicious form on the HTTP origin of a trusted domain and forge requests against the HTTPS app (https://osv.dev/vulnerability/GO-2025-3884). The Gorilla toolkit was archived end-2022, revived mid-2023, and marked **discontinued on endoflife.date on 2026-05-01** (https://endoflife.date/gorilla).
- `justinas/nosurf` implements double-submit with `VerifyToken()` for header/JSON transmission; latest tag v1.2.0 (2025-05-13), ongoing cadence unconfirmed.
- **New since the PRD's Go floor was set**: `net/http.CrossOriginProtection`, stdlib in **Go 1.25** (~Aug 2025). It checks `Sec-Fetch-Site` when present and otherwise compares `Origin`/`Host`. Requests carrying NEITHER `Sec-Fetch-Site` NOR `Origin` are treated as same-origin-or-non-browser and allowed — so a native app sending `Authorization: Bearer` with no `Origin` passes through untouched, exactly the browser-strict/mobile-transparent property this PRD needs. **It issues NO token**, so it cannot satisfy the PRD's CSRF-token requirement on its own; it only reinforces the Origin-validation leg. Source: https://pkg.go.dev/net/http#CrossOriginProtection. Current Go stable is 1.27.1, so a 1.25+ toolchain is well within the PRD's "Go 1.23+". Source: https://go.dev/doc/devel/release
- Evidence points to: a hand-rolled signed/HMAC double-submit token (a few dozen lines, no dependency, avoids the CVE and the discontinued upstream), optionally layered with stdlib `CrossOriginProtection`.

## 7. PostgreSQL 17 append-only mechanics
- `ALTER DEFAULT PRIVILEGES` affects only FUTURE objects, never existing ones. Source: https://www.postgresql.org/docs/17/sql-alterdefaultprivileges.html
- Default privileges key off WHICH ROLE actually creates the object, not roles it belongs to. So `ALTER DEFAULT PRIVILEGES FOR ROLE <owner> ... REVOKE UPDATE, DELETE, TRUNCATE ON TABLES FROM app_rw` only bites if that owner role is what runs the migration `CREATE TABLE` — confirm it matches the `MIGRATIONS_DATABASE_URL` identity or the rule silently does not apply.
- Per-schema default privileges are ADDITIVE over the global default and CANNOT revoke something granted schema-lessly — `ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ...` is a no-op against a broader grant; the schema-less form is required.
- **`SECURITY DEFINER` bypass is real and documented**: such a function runs with its OWNER's privileges (https://www.postgresql.org/docs/17/sql-createfunction.html). A `SECURITY DEFINER` function or trigger function owned by the table owner CAN write to an append-only table regardless of what was revoked from `app_rw`, provided `app_rw` holds `EXECUTE` — and `EXECUTE` on new functions is granted to `PUBLIC` by default and must be revoked explicitly. The real enforcement boundary is therefore an invariant: **no `SECURITY DEFINER` function or trigger owned by the owner role may ever hold `EXECUTE` for `app_rw` or `PUBLIC` if it can write an append-only table.** Make it an explicit design rule with a migration-time assertion, not an implicit assumption.
- Ownership is a superset of any grantable privilege, so `app_rw` must not own those tables and must not be a member of the owner role — the PRD's requirement is correct and necessary.

## 8. slog PII redaction
- Three mechanisms, none sufficient alone:
  1. `slog.LogValuer` — `LogValue() slog.Value` on a type. GAP: it fires reliably only on the top-level value passed to a logging call, not automatically for arbitrary nested struct fields, so it works only if every sensitive field is itself a `LogValuer` type at construction.
  2. `HandlerOptions.ReplaceAttr` — per-attribute hook including nested group members; redact by key name. This is the mechanism the Go blog demonstrates (https://go.dev/blog/slog). GAP: key-name based, so it misses PII under an unexpected key, and it NEVER sees the message string — `slog.Info("user " + email + " logged in")` leaks straight through.
  3. A wrapping `slog.Handler` — implement `Handle(ctx, Record)`, walk and rewrite before delegating. Most powerful (can scan message text and nested groups) but is code you own; `github.com/m-mizutani/masq` already implements this shape.
- For "email, phone and IBAN must never reach the logs" a correct implementation needs all three legs: a lint/discipline rule that call sites never interpolate PII into the message string, `ReplaceAttr` (or a wrapping handler) keyed on the sensitive attribute names, and `LogValuer` types for PII-bearing domain types so redaction survives a forgetful call site.

## Contradictions to carry into the proposal
1. **`httprate` alone does not give trusted-proxy IP handling.** Its own IP helpers are deprecated-insecure; the fix is chi v5.3.0's `middleware.ClientIPFromXFF` installed AHEAD of `httprate`, with httprate's key function reading `middleware.GetClientIP(ctx)`. Two-package wiring, not a single config knob — must be explicit in the design.
2. **The obvious Go CSRF library is compromised and discontinued**, and the new stdlib alternative does not cover the whole requirement. Plan a small hand-rolled signed double-submit token; do not reach for `gorilla/csrf`.

## Gaps
- River's exact current release tag/date and citable minimum PostgreSQL version (inconsistent fetches).
- Whether goose's docs anywhere explicitly endorse the "Go migration opens a second connection under another role" pattern — inference, not a quote.
- `justinas/nosurf`'s 2025/2026 commit cadence beyond its v1.2.0 tag.
