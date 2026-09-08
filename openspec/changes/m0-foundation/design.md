---
change: m0-foundation
phase: design
project: vecingest
date: 2026-09-04
authority: PRD_go.md §5.1, §6/§6.1, §7 (§7.1–§7.4, §7.7, §7.8, §7.10), §8/§8.1/§8.2, §9, §10.1, §11
binds: openspec/changes/m0-foundation/proposal.md (scope, D1–D6, findings table), openspec/changes/m0-foundation/research.md, openspec/changes/m0-foundation/exploration.md, docs/design/ (README, vecingest-light.md, vecingest-dark.md)
citation_scheme: >
  Two incompatible numbering schemes exist for the same facts; this document never
  mixes them. "research §N" cites research.md's own eight numbered sections — the
  scheme the specs use. "proposal finding N" cites the renumbered ten-row table in
  proposal.md's Approach section. A bare "finding N" never appears.
size_note: >
  This artifact exceeds the generic 800-word design budget by explicit orchestrator
  instruction: the phase brief enumerates exact tables/columns/indexes, middleware
  order, token derivation, image strategy and CI topology as required output. The
  corrective re-run adds the audit hash chain, progressive lockout, readiness probe,
  TOTP replay protection and DB-handle split that the specs require and the first
  pass omitted. The third corrective pass adds the RFC 6265 path analysis that moves
  the CSRF recovery endpoint under the refresh cookie's path, the Bearer-based logout
  design, the recovery endpoint's cache/CORP headers, the corrected event-trigger
  command tags with their explicitly-inferred `objid` semantics, the isolation-level
  dependency and throttling of TOTP verification, the `seed`/`worker` subcommand
  designs, and the skippability and golden-file rules the project's go-testing skill
  requires.
---

# Design: M0 Foundation — monorepo, auth, and Portainer-deployable stack

## Technical Approach

One Go module under `api/` produces one static binary (`vecingest`) with subcommands, a two-set embedded `goose` migration chain under a Postgres session advisory lock, and a `chi` + `huma` HTTP surface whose input/output structs are the sole source of `api/openapi/openapi.yaml`. Everything downstream of that file — the TypeScript client, the Zod schemas, the Expo login form — is generated. Layering is Screaming/Hexagonal in the shape PRD §7.2 already dictates: `internal/http` (transport, middleware, huma handlers) → `internal/domain/...` (services, all policy) → `internal/db` (sqlc, no ORM). Handlers contain no logic (PRD §9); the domain never imports `net/http`, `chi`, `httprate`, or `river`.

Four phase-A/phase-B seams are declared as interfaces in M0 so the phase-B swap is configuration, not code (PRD §7.10 point 5). D-N adds a fifth, `AttemptCounter`, because `Limiter` counts requests and progressive lockout counts failures; it follows the same discipline.

| Interface | M0 (phase A) implementation | Phase B | M0 callers |
|---|---|---|---|
| `Limiter` | `httprate` in-process store | `httprate-redis` (Valkey) | rate-limit middleware, login/reset lockout counters |
| `Cache` | in-process TTL map + Postgres `LISTEN/NOTIFY` | Valkey | session revocation set |
| `Queue` | River on Postgres (schema + client + leader election only; **zero producers at M0**) | River → NATS JetStream | none at M0; `worker` wiring only |
| `Search` | **not touched at M0** — no M0 entity is searchable | Postgres FTS → Meilisearch | none |

No handler, domain service or CLI command imports `httprate`, `river` or a cache driver directly; each depends on the interface, wired in `cmd/vecingest`.

### Disposal of the three binding `rules.design`

`openspec/config.yaml` `rules.design` contains exactly three rules, and all three are discharged in this document: **sequence diagrams for multi-actor / multi-step flows** (Data Flow below carries four: startup + migration, refresh rotation with reuse detection, audit-chain append, superadmin bootstrap + first login); the **`Cache` / `Limiter` / `Queue` / `Search` disclosure** with its phase-A vs phase-B confirmation (the table above, extended by the `AttemptCounter` seam in D-N); and the **tenant-scope column of every new table** (next subsection).

### Cross-cutting rules from `rules.apply.guidelines` and `rules.specs` that M0 must not break

These three are **not** `rules.design` — money and dates live in `rules.apply.guidelines`, and "a legal threshold is data, never a fixed value" lives in `rules.specs` and `rules.apply.guidelines`. M0 exercises none of them, which is exactly why each is disposed of here, before a first consumer can get it wrong.

1. **LPH legal rules as data, never Go constants.** M0 contains **zero** LPH rules. Every duration M0 does encode — access 15 min, refresh 30 d, superadmin session 8 h, reset 1 h, OTP 5 min/5 attempts, lockout 5-in-15-min — is a product/security parameter from PRD §5.1/§6.1, not an LPH deadline, majority or percentage; the 15/12 password floor is NIST SP 800-63B, not LPH. Consequently M0 creates **no `legal_rules` table and no `internal/legal` package** (proposal assumption 4: "`internal/legal` has no M0 content"); both arrive with their first consumer at M7. The rule is preserved for the future by an ADR-level convention recorded here and by the `openspec/config.yaml` M0 coverage carve-out that removes `internal/legal/...` from the 80 % gate rather than failing CI on an empty package.
2. **Money as `numeric` in the DB, decimal strings in the API, never `float64`.** No M0 table or endpoint carries money. The rule is nonetheless made unbreakable at M0, before the first money column exists, by two mechanical guards installed in WU-0/WU-3: the `sqlc.yaml` type override mapping `numeric`/`decimal` → `github.com/shopspring/decimal.Decimal` (so a generated struct can never surface a float), and a Semgrep rule + golangci `forbidigo` pattern rejecting `float64`/`float32` in any type under `internal/http/dto`, `internal/domain` or `internal/db`, and rejecting `double precision`/`real` in any `migrations/**.sql`. Both are CI-blocking from M0.
3. **Timestamps UTC as `timestamptz`, legal-deadline arithmetic in `Europe/Madrid`.** Every M0 column is `timestamptz`; the containers run `TZ=UTC`; `now()` is never used for security decisions in application code (an injected `Clock` is, per the proposal's TOTP risk row). M0 computes no natural-day legal deadline — all M0 expiries are absolute durations, correct in UTC. The rule still has one hard M0 consequence, but **not** the one it first appears to have: `gcr.io/distroless/static` **does** ship a zone database. Google lists `tzdata` among the image's contents, and `static/config.bzl` carries `tzdata` in `STATIC_PACKAGES` for both the debian12 and debian13 variants; `time.LoadLocation`'s documented search order (`ZONEINFO` → the system zoneinfo directories → `$GOROOT/lib/time/zoneinfo.zip` → the database embedded by importing `time/tzdata`) therefore resolves `Europe/Madrid` inside that image with no import at all. `cmd/vecingest/main.go` imports `_ "time/tzdata"` anyway for a different and stronger reason: **PRD §9 requires the zone database to travel inside the binary** ("la zona horaria va embebida en el binario con `time/tzdata`"), so M7's deadline arithmetic must not depend on what a base image happens to contain. §6.1 explicitly offers `FROM scratch` as an alternative to `distroless/static`, and a scratch image or a slimmer future base would silently break `LoadLocation` without the embed. The integration test asserting `time.LoadLocation("Europe/Madrid")` succeeds inside the built image stays — it is a regression test for the embed and for a base-image swap, not a workaround for a missing package.

### Tenant scope of every new table (required by `rules.design`)

M0 tables are **identity-scoped, not tenant-scoped** — communities and offices are M1. The mandatory scope column is `user_id` for `sessions`, `password_reset_tokens`, `user_mfa`, `otp_challenges`, and `id` for `users`; every M0 sqlc query filters on it and `make lint-scope` is seeded with that M0 mapping. Two tables are exceptions and both are stated explicitly, because "no scope column" must always be a decision and never an oversight. `audit_log` carries a nullable `community_id` **from M0** (no FK until M1) so that PRD §7.3's "toda tabla de negocio lleva `community_id` … y todos los índices compuestos empiezan por esa columna" can be satisfied later by an `ADD CONSTRAINT` + `CREATE INDEX` instead of rewriting a partitioned append-only table; its M0 rows are authentication events, for which that column is legitimately null. `append_only_relations` (D-B) has **no** scope column and never will: it is a superuser-owned database-infrastructure catalogue with one row per protected relation, not a business table, and `app_rw` holds only `SELECT` on it.

## Architecture Decisions

### D-A. Two embedded goose migration sets, two DSNs, two advisory locks

| Option | Trade-off | Decision |
|---|---|---|
| One goose provider on `MIGRATIONS_DATABASE_URL`, Go migration opens a second `BOOTSTRAP_DATABASE_URL` connection (proposal finding 5, literal) | Correct for env access, but chicken-and-egg: on a virgin database `vecingest_owner` does not exist yet, so the *first* goose connection cannot be made | rejected as the sole mechanism |
| One provider on `BOOTSTRAP_DATABASE_URL` | Every object ends up owned by the superuser, which makes PRD §10.1's restricted-role item unachievable | rejected |
| **Two embedded sets: `migrations/bootstrap/` run as the superuser, `migrations/schema/` run as the owner** | Two providers, two version tables, two lock ids; keeps research's "one goose invocation = one DSN" intact | **chosen** |

The bootstrap set's single file is still the Go migration the proposal requires (`goose.AddMigrationNoTxContext`) because it must read `APP_DB_PASSWORD` and the `MIGRATIONS_DATABASE_URL` credentials from the environment, which SQL cannot do. The proposal's "second connection" becomes a second provider — a mechanism refinement, explicitly permitted by the proposal's own risk row ("test asserting the resulting privilege matrix, not the mechanism") and by the phase brief. Both providers use `WithSessionLocker(lock.NewPostgresSessionLocker(lock.WithLockID(...)))` — goose does **not** lock by default (research §1) — with pinned, never-to-change keys: bootstrap `5432001`, schema `5432002`. Distinct keys let the two sets serialize independently; a session-level (not transaction-level) lock is required because the bootstrap and River migrations run `NO TRANSACTION`.

### D-B. Append-only enforcement: the `SECURITY DEFINER` hole is the real boundary

`REVOKE UPDATE, DELETE, TRUNCATE ON audit_log FROM app_rw, PUBLIC` is necessary and insufficient. Three additional invariants, each asserted by a migration-time `DO $$ ... RAISE EXCEPTION $$` block that fails the deploy, and each re-asserted by a Testcontainers test:

- **I1** — `app_rw` holds no `UPDATE`/`DELETE`/`TRUNCATE` on `audit_log` **or on any relation in its partition hierarchy**, walked recursively through `pg_inherits`. Confirmed against the PostgreSQL 17 manual: privileges are checked on the relation actually named in the query; a partitioned table is an inheritance hierarchy, and the documented exceptions to "the parent's state propagates" concern columns, `CHECK`/`NOT NULL` constraints and `TRUNCATE ONLY` — none of them concern privileges. A grant on a child is therefore a direct bypass of the parent's revoke.
- **I2** — no `SECURITY DEFINER` routine owned by `vecingest_owner` that can write an append-only table is `EXECUTE`-able by `app_rw` **or `PUBLIC`** (research §7; `PUBLIC` receives `EXECUTE` on new functions by default and must be revoked explicitly). Asserted by scanning `pg_proc.prosecdef` joined to `has_function_privilege`.
- **I3** — `app_rw` is not the owner of any append-only table and is not a member of `vecingest_owner` (`pg_auth_members`), because ownership is a superset of any grantable privilege.
- **I4** — the default-privilege baseline itself grants no write beyond `INSERT`. Asserted against `pg_default_acl` for `defaclrole = vecingest_owner`, `defaclobjtype = 'r'`: the ACL may contain `SELECT` and `INSERT` for `app_rw` and nothing else. This is the invariant that closes the hole described next.

#### The `ALTER DEFAULT PRIVILEGES` hole this bootstrap would otherwise open, and how it is closed

`ALTER DEFAULT PRIVILEGES` is used **only** in its schema-less, `FOR ROLE vecingest_owner` form — never to revoke, because the per-schema form is additive and cannot revoke a schema-less grant (research §7). It bites only because the creating role is exactly the `MIGRATIONS_DATABASE_URL` identity; a migration-time assertion checks `current_user = 'vecingest_owner'` before relying on it.

**An unstated dependency, now recorded: the revoke works only because the grantor is `vecingest_owner`.** A privilege carries its grantor, and `REVOKE` removes only privileges granted **directly by the revoking user**. The `REVOKE UPDATE, DELETE, TRUNCATE ... FROM app_rw, PUBLIC` in the migrations and in the D-B event-trigger guard succeeds because `ALTER DEFAULT PRIVILEGES FOR ROLE vecingest_owner` attributes the resulting grant to `vecingest_owner`, which is also the role running the revoke. If that grant ever originates from another role — a second `ALTER DEFAULT PRIVILEGES FOR ROLE <other>`, a manual `GRANT` issued by the superuser, or a table created by a role other than `vecingest_owner` — the revoke **silently succeeds while removing nothing**, because there is no matching grantor and `REVOKE` does not error on a privilege it cannot see. Two guards, both already present, are what turn that silence into noise: invariant **I4** pins the default-privilege baseline to exactly one `defaclrole = vecingest_owner` entry containing only `SELECT` and `INSERT`, and invariant **I1** asserts the *effective* privilege on every relation in the hierarchy rather than asserting that a revoke ran. I1 is the one that matters here: it checks the outcome, so a revoke that removed nothing still fails the deploy.

The naive baseline `GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES` fires on **every** table that role subsequently creates — including every `audit_log` partition created after the bootstrap. The twelve partitions pre-created by the M0 migration are covered by that migration's own explicit `REVOKE`, but the partitions M1's `partitions.maintain` job creates would be born with a live `UPDATE`/`DELETE` grant, protected only by convention. Convention does not beat a standing grant. Two mechanisms close it, and both are required:

1. **The baseline is narrowed to `GRANT SELECT, INSERT ON TABLES` (fail-closed).** A brand-new table is append-only-shaped by default; a *mutable* table's own migration adds `GRANT UPDATE, DELETE ON <table> TO app_rw` explicitly. This inverts the failure mode from "forgot to revoke ⇒ silent bypass" to "forgot to grant ⇒ the first `UPDATE` fails loudly in the integration tests". Every M0 table except `audit_log` carries that explicit grant. Invariant I4 asserts the baseline never widens again.
2. **A partition cannot be created without the revoke, structurally.** The bootstrap set (superuser DSN — `CREATE EVENT TRIGGER` requires superuser) creates a registry table `append_only_relations(relname regclass primary key)` seeded with `audit_log`, and an event trigger `vecingest_append_only_guard` on `ddl_command_end`, filtered:

```sql
CREATE EVENT TRIGGER vecingest_append_only_guard ON ddl_command_end
  WHEN TAG IN ('CREATE TABLE', 'CREATE TABLE AS', 'ALTER TABLE')
  EXECUTE FUNCTION vecingest_append_only_guard_fn();
```

   The tag list is **`'ALTER TABLE'`, not `'ALTER TABLE ... ATTACH PARTITION'`**: PostgreSQL has no such command tag. The event-trigger firing matrix lists `ALTER TABLE` as the tag, and `ATTACH PARTITION` is a subform of it; `WHEN TAG IN ('ALTER TABLE ... ATTACH PARTITION')` would simply never match and the guard would silently not fire on attach. The handler then resolves the affected relations from `pg_event_trigger_ddl_commands()` rows filtered by `object_type = 'table'`, discarding every other subcommand an `ALTER TABLE` may report.

   **What is inferred and not documented, stated as inference.** `pg_event_trigger_ddl_commands()` documents `objid` only as "OID of the object itself", and the PostgreSQL 17 event-trigger chapter contains **zero** occurrences of the word "partition". Which relation is reported for `CREATE TABLE ... PARTITION OF` and for `ALTER TABLE ... ATTACH PARTITION` is therefore **undocumented**; for the ATTACH form the plain reading of "the object itself" points at the **parent**, which on its own would leave the handler unable to reach the newly attached child OID. This is the structural guarantee the whole append-only design rests on, so the handler is written so that it does not depend on which end of the relationship is reported:

   For every reported relation `R` with `object_type = 'table'`, the handler resolves reachability and closure with PostgreSQL 17's built-in partition-introspection functions — `pg_partition_ancestors`, `pg_partition_tree`, `pg_partition_root` — rather than a hand-rolled recursive CTE over `pg_inherits`. This matters because PostgreSQL 17 supports sub-partitioning ("Partitions may themselves be defined as partitioned tables, resulting in sub-partitioning"): a single-level `pg_inherits` join only reaches one level and misses a grandchild partition, whereas `pg_partition_ancestors(rel)` and `pg_partition_tree(rel)` already walk their respective directions **transitively**, at every level, leaf and intermediate alike — matching I1's "walked recursively" — and are less error-prone than reimplementing that recursion by hand:

   - **Reachability (upward, transitive)**: `R` is in scope if `R` itself is registered in `append_only_relations`, or any row returned by `pg_partition_ancestors(R)` is registered — this reaches a registered relation from a descendant at *any* depth of sub-partitioning, not only the immediate parent (covers the case where a child, at any depth, is reported).
   - **Closure (downward, transitive)**: once `R` is in scope, the handler computes `root := pg_partition_root(R)` and revokes on every relation returned by `pg_partition_tree(root)` — the entire tree below the registered root, at every level, not only `R`'s immediate children (covers the case where the parent, at any depth, is reported, which is the plain reading for ATTACH).

   `REVOKE UPDATE, DELETE, TRUNCATE ON <rel> FROM app_rw, PUBLIC` is executed for each relation in that transitive closure, gated as described below.

   **The downward leg is load-bearing, not merely defensive, and this is now recorded rather than assumed.** PostgreSQL commit 43231423 feeds an attached partition's `ObjectAddress` into the `ALTER TABLE` **subcommand** collection only, which is reachable by a C deparse extension and not through `pg_event_trigger_ddl_commands()`'s scalar `objid` column. So for `ATTACH PARTITION` that column very likely reports the **parent** — `R` is very likely `audit_log` itself (or an intermediate sub-partitioned ancestor), already registered directly — and the upward leg never has to fire for this command; it is the downward `pg_partition_tree(root)` walk that actually reaches the newly attached child. The upward leg stays necessary for `CREATE TABLE ... PARTITION OF`, where the plain reading of "the object itself" points at the new child instead.

   **The revoke is gated, not unconditional.** PostgreSQL 17's `REVOKE` documentation: "When a non-owner of an object attempts to REVOKE privileges on the object, the command will fail outright if the user has no privileges whatsoever on the object." An unconditional fan-out over the closure would therefore hard-fail the whole DDL statement the instant any relation in the tree is owned by a role the invoker holds nothing on — the fail-closed direction, and probably unreachable given this design's own ownership model, but not a risk worth carrying. Each `REVOKE` is preceded by `has_table_privilege('app_rw', rel, 'UPDATE')` (equivalently `DELETE`/`TRUNCATE`, since the fail-closed baseline narrowing above grants and revokes the three together) and is skipped when it is already false — this both closes that edge and removes the redundant re-revoke on partitions the fail-closed default-privilege baseline already left without the grant in the first place, which `pg_partition_tree(root)` would otherwise revisit on every subsequent DDL event in the tree.

   **This fan-out does not cost table locks.** A validation pass checked whether revoking over the whole closure would take `AccessExclusiveLock` on every partition, and refuted it against the PostgreSQL 17 source: `objectNamesToOids` resolves the `REVOKE` target with `NoLock`, and `ExecGrant_Relation` opens only `pg_class` and `pg_attribute` with `RowExclusiveLock`. A revoke takes no lock on the target relation itself, so the fan-out costs catalog row updates and cache invalidations, not table locks — roughly 120 row updates a decade in with monthly partitions. **The mechanism is verified by test, not by documentation**: two RED integration tests are required and neither may be dropped — one for `CREATE TABLE ... PARTITION OF audit_log` and one for `ALTER TABLE audit_log ATTACH PARTITION`, each asserting that `app_rw` cannot `UPDATE`/`DELETE`/`TRUNCATE` the resulting child **with no explicit revoke anywhere in the test**. If the ATTACH test fails on the pinned image, the guard is wrong and the migration must not ship; the invariant assertions of I1 are the deploy-time backstop that turns a wrong guard into a failed deploy rather than a silent bypass.

   The function is deliberately **`SECURITY INVOKER`**: migrations run as `vecingest_owner`, which owns `audit_log` and may therefore revoke on its own children, and a superuser-run `CREATE TABLE` can revoke anything — so no `SECURITY DEFINER` routine is introduced and I2's surface does not grow. **Stated as inference**: the PostgreSQL 17 event-trigger chapter never states what role a `SECURITY INVOKER` event-trigger function executes as. The chain above holds by inference from the general `SECURITY INVOKER` rule (the function executes with the privileges of the user that calls it) applied to the user running the DDL; the documentation does not say so for event triggers specifically. The same two integration tests that prove the revoke also prove this inference, because a wrong invoker role produces a permission error inside the trigger and fails the DDL loudly.

   `EXECUTE` on the trigger function is revoked from `PUBLIC` anyway; an event-trigger function cannot be called directly from SQL, but the revoke costs nothing and keeps the I2 scan clean. M1's `partitions.maintain` inherits the guarantee without knowing it exists, and if `pg_partman` turns out to be available (D-C) its `part_config.inherit_privileges = true` becomes a third, redundant layer rather than the only one.

Every future migration that creates an append-only table still carries its own explicit `REVOKE` and registers the table in `append_only_relations` — but that is now defence in depth on top of two mechanisms that hold when a developer forgets, rather than the only thing standing between `app_rw` and a mutable audit trail.

### D-C. `audit_log` partitioning: declarative now, `pg_partman` when the maintenance job exists

PRD §7.3 (line 491): "Las tablas de alto volumen (`notifications`, `audit_log`, …) se crean **particionadas por mes** desde M0 con `pg_partman`". PRD §8.1 pins `image: postgres:17-alpine`, which ships core + contrib only; `pg_partman` is a third-party extension not present in that image, so `CREATE EXTENSION pg_partman` would fail on the stack the PRD itself specifies. This is an internal PRD tension, not a design divergence: the **partitioned-by-month shape is honoured exactly**, and only the maintenance tool is deferred. M0 creates `audit_log` as a native `PARTITION BY RANGE (created_at)` table with twelve months of partitions pre-created by the migration; the `partitions.maintain` job (§7.8) is M1+ scope anyway. The RED check that settles it at implementation time is `SELECT 1 FROM pg_available_extensions WHERE name = 'pg_partman'` against the pinned image; if it is present, the migration adopts `partman.create_parent(...)` with `part_config.inherit_privileges = true` so children can never carry more privilege than the parent, and I1 still applies.

### D-D. Token model

| Token | Form | Lifetime | Storage | Rotation / revocation |
|---|---|---|---|---|
| Access | JWT HS256, `kid` header, claims `sub` (user id), `sid` (**= `family_id`**), `sa` (is_superadmin), `iat`, `exp`, `jti` | 15 min (PRD §5.1) | never persisted; web keeps it in memory, native in `expo-secure-store` | verified against `JWT_SECRET`, or `JWT_SECRET_PREVIOUS` selected by `kid` during rotation (§7.7) |
| Refresh | 32 opaque bytes from `crypto/rand`, base64url — **not** a JWT | 30 d; **8 h for `superadmin`** (§6.1) | `sessions.refresh_token_hash bytea` = SHA-256 of the raw token, `UNIQUE`; the raw value never touches the database or a log | single use; rotation inserts a new `sessions` row with the same `family_id` and stamps `revoked_at` on the old one |
| CSRF | `base64url(nonce ‖ exp) + "." + base64url(HMAC-SHA256(k, "vecingest/csrf/v1" ‖ nonce ‖ family_id ‖ session_id ‖ exp))` | `exp` = the issuing `sessions` row's `expires_at`, so it can never outlive its refresh generation | **response body only** — never a cookie; the web client holds it in memory next to the access token, and echoes it in `X-CSRF-Token` | reissued on every login and every refresh; bound to `session_id`, so rotation invalidates the previous token by construction |

`k = HKDF-SHA256(JWT_REFRESH_SECRET, info="vecingest/csrf/v1")`. **Rationale**: M0 refresh tokens are opaque random values, so `JWT_REFRESH_SECRET` (PRD §8.2) would otherwise have no consumer; deriving from it keeps §8.2's variable list unchanged — adding a stack variable would diverge from the PRD — and keeps the CSRF key independent of `ENCRYPTION_KEY`, whose rotation is a data re-encryption event.

Session lineage: `sessions.family_id` **is** the logical session; each row is one refresh-token generation. That fits PRD §7.3's `sessions` columns exactly and needs no extra table. Presenting a refresh token whose hash matches a row with `revoked_at IS NOT NULL` is reuse: `UPDATE sessions SET revoked_at = now() WHERE family_id = $1 AND revoked_at IS NULL` kills the whole family, writes `audit_log`, and publishes the revocation. Session **listing** is `GROUP BY family_id` taking the newest live row; **revocation** is the same family-wide update.

**Immediate rejection of an already-issued access token.** A revoked family must stop an access token that is still cryptographically valid for up to 15 minutes. The auth middleware, after JWT verification, consults the `Cache` revocation set keyed by `sid` (TTL = access-token lifetime, so an entry is only ever needed for as long as a token can live). Revocations propagate to every replica through Postgres `LISTEN/NOTIFY` on channel `session_revoked`, published in the same transaction as the `UPDATE`. Fail-closed rule: if the `LISTEN` connection has been down longer than the access-token TTL, or the process booted less than one TTL ago, the middleware falls back to an indexed `sessions` lookup by `sid` — correctness over latency. Alternatives rejected: querying `sessions` on every request (one guaranteed Postgres round-trip on the hot path, against §10.1's p95 `GET /v1/me` < 50 ms) and shortening the access token (changes a PRD-fixed lifetime).

### D-E. CSRF: hand-rolled signed double-submit, layered with stdlib `CrossOriginProtection`

`gorilla/csrf` is ruled out (CVE GO-2025-3884: `TrustedOrigins` trusts the HTTP variant of an HTTPS host; toolkit discontinued 2026-05-01). `net/http.CrossOriginProtection` (Go 1.25) reinforces the `Origin`/`Sec-Fetch-Site` leg and is transparent to a native client that sends neither, but issues **no token**, so it cannot satisfy D4 alone.

**There is exactly one cookie, and the CSRF token is not it.** The refresh cookie keeps PRD §5.1's attributes verbatim: `HttpOnly; Secure; SameSite=Strict; Path=/v1/auth/refresh; Domain=api.DOMAIN`. A second, JS-readable CSRF *cookie* was the first pass's design and **cannot work**: the web client is served from `app.DOMAIN` (§8: `app.DOMAIN → web`, `api.DOMAIN → api`), so `document.cookie` on `app.DOMAIN` can never read a cookie scoped to `Domain=api.DOMAIN`, and `Path=/v1/auth/refresh` narrows it further. Widening the cookie to `Domain=DOMAIN` to make it readable would contradict §5.1's explicit "no se comparte con `DOMAIN` ni con otros subdominios" and would reintroduce precisely the sibling-subdomain cookie-injection weakness OWASP names for naive double-submit (research §6).

**That same path narrowing dictates where every cookie-reading endpoint may live.** RFC 6265 §5.1.4 path-match: the browser attaches a cookie only when the request path *equals* the cookie-path, or when the cookie-path is a prefix of the request path and the first character of the request path not contained in the cookie-path is `/`. With cookie-path `/v1/auth/refresh`:

- `/v1/auth/refresh` — equal ⇒ match.
- `/v1/auth/refresh/csrf` — the cookie-path is a prefix, the cookie-path does not end in `/`, and the first uncovered character of the request path is `/` ⇒ **match**. This is a subpath under the cookie's path and the cookie *is* sent.
- `/v1/auth/csrf` and `/v1/auth/logout` — the cookie-path is not a prefix ⇒ **no match, no cookie**. An endpoint at either path that tries to read the refresh cookie finds nothing and can only ever return `401`.

**The token travels in the response body instead**, held in memory by the web client exactly as the access token already is (§5.1: "el access token vive solo en memoria"):

- `POST /v1/auth/login`, `POST /v1/auth/superadmin/login` and `POST /v1/auth/refresh` return `csrf_token` in the JSON body **only on the cookie transport** — a body-path (native) caller never receives one, because it never has ambient authority to protect.
- **`GET /v1/auth/refresh/csrf`** restores it after a page reload, which is the one case a memory-only token cannot survive on its own. The path is deliberate and not cosmetic: it is a subpath beneath `Path=/v1/auth/refresh` under the RFC 6265 rule stated above, so the refresh cookie is actually attached. The earlier draft placed this endpoint at `/v1/auth/csrf`, which is *not* a subpath: the cookie would never have been sent, the `sessions` lookup would never have found a row, and the endpoint would have returned `401` unconditionally. It is a safe method with no side effect: it reads the refresh cookie, looks up the live `sessions` row, and mints a token bound to that exact row. It rotates nothing. A cross-site attacker can make the browser *send* that request but cannot *read* its response — CORS emits `Access-Control-Allow-Credentials: true` only for `https://app.DOMAIN`, and `*` is illegal together with credentials, so no other origin is ever granted read access. Rate-limited at 60 req/min per IP; `401` when the cookie is missing, expired or revoked. This endpoint is an addition to PRD §7.4's list, which the PRD introduces as "endpoints principales", not an exhaustive contract.
- **Response headers on `GET /v1/auth/refresh/csrf`** (all three are required, not optional hardening):
  - `Cache-Control: no-store` — the body is a per-session secret; RFC 9111 §7.3 warns explicitly about caches storing credentials.
  - `Vary: Origin` — `Access-Control-Allow-Origin` is computed *from* the request `Origin`, so without `Vary` a shared cache may serve the `https://app.DOMAIN` variant, carrying `Access-Control-Allow-Credentials: true`, to a request from another origin. That is a cache-mediated CORS bypass, and it defeats the CORS argument above.
  - `Cross-Origin-Resource-Policy: same-origin` — this is what closes the no-CORS subresource class (`<script src>`, `<img>`, `<link>`), where the request is sent with cookies and the `onload`/`onerror` oracle leaks whether it succeeded. `X-Content-Type-Options: nosniff` does **not** close that class: `nosniff` blocks *execution* of a script-destination response served without a JavaScript MIME type, but it does not stop the request being made, does not stop the cookies being attached, and does not stop the response arriving — the load/error oracle survives it. `nosniff` remains set globally (D-H step 5) for its own reasons; it is not the control being relied on here.
- The custom `X-CSRF-Token` header is itself a second independent leg: a cross-origin request carrying it forces a CORS preflight that a cross-site form post cannot produce (OWASP, research §6).

**`POST /v1/auth/logout` has the same path problem and is solved differently.** `/v1/auth/logout` is not under `Path=/v1/auth/refresh`, so the refresh cookie is not attached and logout can never identify the session from it. Logout is therefore authenticated by the **access token**, not by the cookie: it requires `Authorization: Bearer <access>`, takes `sid` (= `family_id`, D-D) from the verified claims, and runs the same family-wide `UPDATE sessions SET revoked_at = now() WHERE family_id = $sid AND user_id = $sub AND revoked_at IS NULL`, writes `audit.Append` and publishes `session_revoked`. Consequences, each deliberate:

- **No CSRF token is required on logout**, and requiring one would be theatre: the request carries no ambient authority at all (no cookie is sent to that path), and a cross-site attacker cannot read the in-memory access token, so it cannot forge the request. The `X-CSRF-Token` header is accepted and ignored if present.
- **The cookie is still cleared**, because setting a cookie is not path-matched the way sending one is: the response emits `Set-Cookie` with the identical `Name`, `Domain=api.DOMAIN`, `Path=/v1/auth/refresh`, `HttpOnly; Secure; SameSite=Strict` and `Max-Age=0`, which deletes the stored cookie regardless of the request path.
- **An expired access token cannot log out.** That is not a gap: the family is already unusable for API calls, the client discards its in-memory state, and the two remaining exits are refresh-then-logout, or `DELETE /v1/me/sessions/{family_id}` — which is the same operation reached through a token the caller still holds.
- **Native clients** use the identical Bearer-based logout; there is no body transport for logout, so the cookie-XOR-body rule below applies only to `/v1/auth/refresh`.

Validation on `/v1/auth/refresh`, cookie path: header present → constant-time HMAC verify → `family_id` in the token equals the family of the presented refresh cookie → `session_id` in the token equals the exact `sessions` row that cookie hashes to → `exp` not passed. Any failure is `403 AUTH_CSRF_INVALID`. The `session_id` binding is what makes rotation mandatory rather than conventional: a token minted for generation *N* is dead the instant generation *N+1* exists, so a stolen token's replay window is bounded by the refresh token's own single-use window.

**Native-client exemption without a hole.** The exemption is by *transport*, never by User-Agent, header sniffing or an allowlist. PRD §5.1: "El endpoint `/auth/refresh` acepta el refresh token por cookie (web) o por body (móvil), **nunca ambos** en la misma petición."

- refresh cookie present → cookie path → CSRF token **required**;
- no cookie, refresh token in the JSON body → body path → no CSRF token, because there is no ambient authority to abuse: a cross-site attacker cannot supply a body token it does not know;
- both present → `400 AUTH_AMBIGUOUS_TOKEN_TRANSPORT`. This last rule is what closes the downgrade hole — a browser attacker cannot force the body path.

### D-F. Where the conditional password rule lives, given Zod is generated

The rule ("15 characters without 2FA, 12 with TOTP active; the error states which rule applies") cannot be a static `minLength` in OpenAPI, and it **must not** be evaluated client-side: whether an account has TOTP active is exactly the fact a client-side conditional would leak. One authoritative definition, two projections:

- **Authority**: `internal/domain/auth.PasswordPolicy` in Go — the only place the two thresholds exist.
- **Static projection**: the huma request struct tags the password field `minLength:12` (the absolute floor). `openapi-zod-client` generates `z.string().min(12)`, so the obviously-short case is caught offline with no duplicated logic.
- **Dynamic projection**: the server rejects with a stable code (`AUTH_PASSWORD_TOO_SHORT_NO_MFA` / `AUTH_PASSWORD_TOO_SHORT_WITH_MFA`) plus `details: { min_length, rule }`. Per PRD §9 the app renders the message from the code and interpolates `details.min_length` — the applicable rule travels as **data**, not as a second implementation.

HIBP k-anonymity (SHA-1 5-char prefix, `Add-Padding: true`, discard count-0 rows) sits behind a `HIBPChecker` interface with a fake server in tests (no network in CI). On an HIBP transport error the check **fails open** with a WARN log and an `audit_log` entry (`action = 'auth.hibp_unavailable'`): the length floor and Argon2id still hold, and failing closed would let a third-party outage block every password reset. This is a deliberate, reversible decision and it is **recorded in `docs/security/threat-model.md` as an accepted residual risk**: an attacker who can block egress to `api.pwnedpasswords.com` can register or reset to a breached password, provided it still clears the 15/12-character floor. Compensating controls are the length floor, Argon2id, progressive lockout (D-N) and the audit entry, which makes a sustained outage visible rather than silent. The alternative — failing closed — trades a credential-quality risk for a self-inflicted availability outage on the account-recovery path, which is the worse of the two.

### D-G. Argon2id (`x/crypto/argon2` is a bare KDF)

`internal/domain/auth/password` implements the three missing layers itself: 16-byte salt from `crypto/rand`; `argon2.IDKey(pw, salt, 2, 19456, 1, 32)` (PRD §5.1 params); PHC string `$argon2id$v=19$m=19456,t=2,p=1$<salt>$<tag>` with `base64.RawStdEncoding`; verification by recompute + `crypto/subtle.ConstantTimeCompare`; a strict PHC parser that rejects unknown algorithms/versions and bounds-checks `m`/`t`/`p` (an attacker-supplied hash with `m=4 GiB` is a DoS); `Verify` returns `(ok, needsRehash)` and a login rehashes in the same transaction as `last_login_at`. Login against an unknown email runs the same Argon2 work against a fixed dummy hash, so the response reveals nothing about existence (PRD §5.1).

### D-H. Middleware order (the trusted-proxy chain is two packages, not one knob)

`httprate` performs **no** proxy-trust resolution and its own IP helpers are deprecated as spoofable. Order in `internal/http/router.go`, outermost first:

```
1  RequestID / X-Request-Id validation (§7.7: format+length checked, else server-generated)
2  middleware.ClientIPFromXFF(netip.PrefixFrom(PROXY_IP, 32))      ← chi v5.3.0; MUST precede 3,4,8
3  slog request logger (redacting handler; logs the *resolved* client IP)
4  Recoverer
5  security headers (§6 "middleware secure")
6  CORS — credentials:true only for https://app.DOMAIN
7  http.CrossOriginProtection (Go 1.25) on unsafe methods
8  Limiter middleware → httprate.LimitBy(..., key = httprate.CanonicalizeIP(middleware.GetClientIP(ctx)))
9  per-route: Bearer auth → CSRF (cookie path of /v1/auth/refresh only)
```

`CanonicalizeIP` buckets IPv6 by /64 so an attacker cannot rotate within a delegated prefix. Development-only fallback: when `APP_ENV=development` and `PROXY_IP` is unset, `ClientIPFromXFFTrustedProxies(1)` is installed instead — a hop count is strictly weaker, and `internal/config` refuses it when `APP_ENV` is `staging` or `production`. `APP_ENV` is the PRD's own variable (§8 lists it with the domain `development | staging | production`, §8.2 defaults it to `production`, and §8 scopes `vecingest seed` by it), so this fallback keys off a documented stack variable rather than an invented one. Limits per §6: 10 req/min login and reset, 300 req/min per authenticated user, 60 req/min per IP on public endpoints. The middleware-order itself is asserted by a test, because omitting step 2 silently reverts to spoofable behaviour.

### D-I. Startup configuration validation and the secrets holder

`config.Load(ctx, env, cmd) (Config, *secrets.Holder, error)`:

- `Config` is the exported, log-safe object. It holds **no** secret. It carries `DOMAIN`, `APP_URL`, `CORS_ORIGINS`, `PORT`, `APP_ENV`, `PROXY_IP`, `R2_*` non-secret fields, `SMS_PROVIDER`, `TSA_URL`, `SENTRY_DSN`, timeouts and the resolved DSN **hosts** (never the credentials).
- `secrets.Holder` holds `ENCRYPTION_KEY`, `JWT_SECRET`, `JWT_SECRET_PREVIOUS`, `JWT_REFRESH_SECRET`, `APP_DB_PASSWORD` and the three full DSNs in unexported fields, reachable only through accessor methods. It implements `slog.LogValuer` → `"[redacted]"`, `String()` → `"secrets.Holder{redacted}"`, and `MarshalJSON` → `"\"[redacted]\""`, so logging or error-reporting a config value cannot serialize a secret even by accident.
- After validation succeeds, `os.Unsetenv("ENCRYPTION_KEY")` — PRD §6.1: "la app la lee al arrancar y la borra de `process.env`". Only `ENCRYPTION_KEY` is unset, because that is the variable the PRD names; the remaining secrets live in the holder but stay in the environment, and the design records honestly that `Unsetenv` is not a memory scrub and buys nothing against an attacker who already reads process memory.
- Validation is **fail-fast and total**: every missing or malformed variable is collected and reported at once, by **name only, never by value**, then the process exits non-zero before binding a listener. `ENCRYPTION_KEY` is validated as exactly 32 base64-decoded bytes; it has a real M0 consumer (`user_mfa.totp_secret_encrypted`, AES-256-GCM with the `v1:` version prefix of §6.1). Requirements are per-subcommand, and **every one of the six subcommands has a declared set** — a subcommand with no declared set would silently inherit `serve`'s, which is how a worker ends up demanding a JWT secret it never uses:

| Subcommand | Required | Optional | Explicitly NOT required |
|---|---|---|---|
| `serve` | JWT trio, `DATABASE_URL`, `PROXY_IP`, `CORS_ORIGINS`, `ENCRYPTION_KEY`, `SMTP_URL`, `MAIL_FROM`, `APP_ENV`, `PORT` | `DATABASE_URL_READ` (D-R), `SENTRY_DSN`, `TURNSTILE_SECRET` | bootstrap/migration credentials |
| `serve --migrate`, `migrate` | the above **plus** `BOOTSTRAP_DATABASE_URL`, `MIGRATIONS_DATABASE_URL`, `APP_DB_USER`, `APP_DB_PASSWORD` | — | — |
| `worker` | `DATABASE_URL`, `APP_ENV` | `DATABASE_URL_WORKER` (D-R) | **the JWT trio, `PROXY_IP`, `CORS_ORIGINS`, `PORT`, `SMTP_URL`/`MAIL_FROM`** — the worker binds no listener, issues no token and has zero registered job workers at M0 |
| `seed` | `DATABASE_URL`, `APP_ENV` | — | `ENCRYPTION_KEY` (no MFA enrolment), SMTP, JWT |
| `bootstrap-superadmin` | `DATABASE_URL`, `ENCRYPTION_KEY` | — | `APP_ENV` — §224 makes it explicitly environment-independent |
| `health` | `PORT` for `--live`/`--ready`; `DATABASE_URL` for `--worker` | — | everything else — it must start inside a distroless container with a minimal environment |

### D-J. `slog` PII redaction needs all three legs

`ReplaceAttr` never sees the message string and `LogValuer` does not fire reliably on nested struct fields, so no single mechanism suffices:

1. **Wrapping handler** `internal/observability/slogpii.Handler` — walks the whole `slog.Record` including the **message** and nested groups. Key-based redaction (`email`, `phone`, `iban`, `id_document`, `token`, `authorization`, `password`, `refresh_token`, `csrf`, `code_hash`, `totp_secret`, `secret`) plus compiled value-pattern scanning of the message for email, Spanish phone and `ES\d{2}[A-Z0-9]{20}` IBAN, replaced with `[redacted:email]` etc.
2. **`HandlerOptions.ReplaceAttr`** on the underlying `slog.JSONHandler` as a cheap second key-based leg, catching attributes injected below the wrapper.
3. **`LogValuer` on typed PII** — `auth.Email`, `domain.Phone`, `domain.IBAN` are distinct types, never bare `string`, so redaction survives a forgetful call site.

Plus a call-site discipline rule enforced by a Semgrep pattern: no `slog.*` call whose message argument is a concatenation or `fmt.Sprintf` over a PII-typed value.

### D-K. Images and compose hardening

Multi-stage: builder `golang:1.27-bookworm`, `CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w -X main.version=…"`, BuildKit module/build cache mounts; final `gcr.io/distroless/static-debian12:nonroot`, `USER 65532:65532`, `ENTRYPOINT ["/vecingest"]`, single `linux/amd64` platform (the phase-A server is amd64; arm64 is a later, cost-free addition). No shell ⇒ migrations are `//go:embed`ed and run from `main`, and the healthcheck is `["CMD","/vecingest","health","--ready"]` exactly as PRD §8.1 specifies. `go.mod` declares `go 1.26.0` — **PRD §7.1 pins Go 1.26+**, and its River row states why: River v0.47.0's own `go.mod` declares `go 1.26.0`, and since Go 1.21 that directive is a hard minimum for consumers, not a hint. The 1.26 floor covers Go 1.25's `net/http.CrossOriginProtection` (D-E) a fortiori. CI and the image build on current stable 1.27.1. `web`/`site` build on `nginxinc/nginx-unprivileged` listening on **8080** with `user: "1000:1000"`.

**Tags (contract): `sha-<short>` and `v<semver>` are immutable and never overwritten; `latest` is a moving pointer.** Production pins `TAG` to a `v<semver>`; rollback re-points `TAG` at the previous immutable tag and never mutates `latest`.

`deploy/docker-compose.yml` carries §8.1 verbatim in shape: `read_only: true`, `security_opt: [no-new-privileges:true]`, `cap_drop: [ALL]`, `tmpfs: [/tmp]` (nginx services add `/var/cache/nginx`, `/var/run`), `user:` per service, `mem_limit` (api/worker 256m, nginx 128m, db 768m) with `GOMEMLIMIT: 200MiB`, `restart: unless-stopped`, `healthcheck`, `stop_grace_period: 30s` on `worker`, `depends_on: {condition: service_healthy}`, `networks: [internal, proxy]` with `db` reachable only on `internal`. Two corrections to the §8.1 reference copy, which the PRD itself labels a reference ("esta copia es de referencia"): the mis-indented `CLAMAV_HOST` key is dropped entirely, since ClamAV is M2 scope, and the `clamav` service stays only as an inert `profiles: [av]` stub.

### D-L. River pinned at v0.47.0 — and what that pin drags in

Research §3's gap is closed. **River v0.47.0**, published 2026-08-31, not a prerelease, verified via the GitHub API on 2026-09-04. Module path `github.com/riverqueue/river`, **no major-version suffix — it is still 0.x, so there is no API stability guarantee**: Renovate groups it but must never auto-merge it, and the pinned migration `TargetVersion` below is what keeps a dependency bump from becoming a schema change.

Two consequences of the pin, both already absorbed elsewhere in this design:

- **Toolchain.** River's `go.mod` declares `go 1.26.0` with `toolchain go1.26.6`. Since Go 1.21 the `go` directive is a hard minimum for every consumer, so this alone sets the backend floor at **Go 1.26+** — which is what PRD §7.1 now pins, and what D-K encodes in `api/go.mod`. `openspec/config.yaml` still says "Go 1.23+" in `context` and in `projects[].stack`; that is stale against the PRD and is corrected in the same edit as the coverage carve-out.
- **PostgreSQL.** River's README states no minimum PostgreSQL version; its docs say it is tested against the three most recent majors, which covers **PostgreSQL 17** (PRD §7.1, §8.1). There is no stack conflict to escalate.

What the design fixes on top of the pin:

- River's schema is provisioned at M0 (proposal assumption 3) by `migrations/schema/0000N_river.go`, an `AddMigrationNoTxContext` Go migration calling `rivermigrate` with an **explicit pinned `TargetVersion`**, never "latest", so a dependency bump can never silently change the schema.
- It must be `NoTx`: River's migration set **cannot** all run in one transaction — its migration 6 depends on migration 4's enum change, and PostgreSQL forbids creating an immutable function in the same transaction that altered a dependent enum. A `-- +goose NO TRANSACTION`-equivalent Go migration lets `rivermigrate` manage its own per-step transactions.
- River tables are **not** append-only: `GRANT SELECT, INSERT, UPDATE, DELETE ON river_job, river_leader, river_client, river_client_queue, river_queue TO app_rw` is explicit, and invariant I1's assertion excludes them by name.
- `worker` at M0 starts a River client with **zero registered workers** plus leader election, purely so the binary, the healthcheck (`/vecingest health --worker`) and the compose service are real deliverables.

### D-N. Progressive lockout: two independent counters, one alert, no enumeration channel

PRD §5.1 requires blocking after 5 failed attempts in 15 minutes **counted by email and by IP** plus an email alert to the affected user. Neither the dual keying nor the alert is expressible through the `Limiter` seam as declared, because `Limiter.Allow` counts *requests* while lockout counts *failures*. A fifth phase-A/phase-B seam is therefore declared alongside the four in the table above:

```go
type AttemptCounter interface {
    Fail(ctx context.Context, key string, window time.Duration) (count int, err error)
    Count(ctx context.Context, key string, window time.Duration) (int, error)
    Reset(ctx context.Context, key string) error
}
```

Phase A is the same in-process sliding-window store `httprate` uses; phase B is Valkey. No handler or domain service touches the implementation.

- **Dual keying.** Two independent counters per failed login: `auth:fail:email:<hex(sha256(normalized_email))>` and `auth:fail:ip:<httprate.CanonicalizeIP(resolved client IP)>`, window 15 min, threshold 5. The email is hashed into the key so neither the in-process store nor a future shared Valkey ever holds a plaintext address — the same PII discipline D-J applies to logs. The IP is the *resolved* client IP from D-H step 2, never a raw header.
- **The email counter is incremented for unknown emails too.** Skipping it for non-existent accounts would turn lockout into a user-enumeration oracle: an attacker would learn an address is registered by whether attempt six behaves differently. Both counters advance identically, the dummy-Argon2id work of D-G still runs, and the response is `429 AUTH_TOO_MANY_ATTEMPTS` in both cases.
- **A successful login resets the email counter only.** Resetting the IP counter as well would let an attacker holding one valid account clear their own IP budget between bursts.
- **"Progressive" is the escalating window.** PRD §5.1 says "bloqueo progresivo tras 5 intentos fallidos en 15 minutos" without specifying the escalation, so this design fixes it: consecutive lock cycles on the same key extend the block 15 → 30 → 60 min, capped at 60, decaying back to 15 after a clean hour. Recorded as a design choice, not a PRD quotation.
- **The alert.** `internal/mail`'s M0 scope grows from one template to two: `password_reset.html` and `login_lockout.html` (PRD §5.1 "aviso al usuario por email"). It is sent only when the email belongs to a real account — the attacker never observes the difference, since the response is identical either way — and at most once per lock window per account, deduped on `auth:lockout-alert:<hex(sha256(email))>` with the lock's TTL, so lockout cannot be weaponised into a mail bomb. Dispatch is **asynchronous** through a bounded worker pool owned by `internal/mail` (fixed size, capped queue, drained on graceful shutdown, registered in `cmd/vecingest` — PRD §9 forbids unmanaged goroutines): sending inline would add SMTP latency to the response only for existing accounts, which is a timing oracle. A lockout also writes `audit_log` (`action = 'auth.lockout'`).
- **A third key family covers the second factor, because these two do not.** `auth:fail:email:*` and `auth:fail:ip:*` count *login* failures; a TOTP or recovery-code submission happens after the first factor already succeeded and touches neither. D-P therefore registers `auth:fail:totp:<user_id>` on the same `AttemptCounter`, same window, same threshold, same escalation. It is recorded here as well as in D-P so that the counter inventory lives in one place: three key families, one interface, one store.
- **Turnstile is out of M0** (proposal: external services beyond configuration placeholders). `Count` is exposed so that M1 can require a CAPTCHA from the third failure (PRD §5.1) without touching the counter design; M0 wires `TURNSTILE_SECRET` as a validated-but-unused config field.

### D-O. `audit_log` hash chain: computed in Go, serialized by one advisory lock, continuous across partitions

PRD §6.1 requires "hash encadenado y anclaje diario del `audit_log`". The columns alone are not a design; this is the rest of it.

**What is hashed.** `hash = SHA-256(prev ‖ F(id) ‖ F(created_at) ‖ F(user_id) ‖ F(community_id) ‖ F(action) ‖ F(entity) ‖ F(entity_id) ‖ F(before) ‖ F(after) ‖ F(ip) ‖ F(request_id))`, where `prev` is the predecessor's `hash` or 32 zero bytes for the genesis row, and `F(x)` is a big-endian `uint32` length prefix followed by the canonical encoding of `x` — RFC 3339 with nanoseconds in UTC for timestamps, canonical (sorted-key, no insignificant whitespace) JSON for `before`/`after`, and a single `0xFF` byte for SQL `NULL`, which is distinct from the empty string. The length prefixes exist so that no two different field splits can produce the same byte stream; without them, moving a character from `action` to `entity` is undetectable.

**Where it is computed.** In Go, in `internal/domain/audit`, not in a database trigger. The threat PRD §6.1 names is "un operador con acceso a la BD"; a chain whose construction lives entirely inside the database is verifiable only by the same component the threat model distrusts. Computing it in the application means `audit.VerifyChain` can be run from outside against a dump. A single service function `audit.Append(ctx, tx, Entry)` is the **only** writer, and the sqlc query `InsertAuditLog` is called from nowhere else — enforced by the same Semgrep style rule that guards the money and PII rules.

**How concurrent INSERTs are serialized.** `pg_advisory_xact_lock(5432003)` is taken inside the caller's transaction, before reading the head and before inserting — the same mechanism PRD §7.7 mandates for the votes chain, which uses `pg_advisory_xact_lock` keyed by `meeting_item_id`. The audit chain is global rather than per-scope, so the key is a single pinned constant. Per-community chains were rejected: `community_id` is nullable at M0 and null for every authentication event, so a per-scope chain would leave M0's entire audit trail unchained. The lock is transaction-scoped and auto-released at COMMIT (research §2), and it is held only across one read plus one insert; M0 audit volume is authentication events, so contention is not a concern, and if it ever becomes one the answer is per-`entity` chains, not a weaker lock.

**Ordering, and why partitions do not break it.** The head is `SELECT hash, created_at, id FROM audit_log ORDER BY created_at DESC, id DESC LIMIT 1` against the **partitioned parent**, which Postgres answers with a merge append over the per-partition indexes; in practice only the newest partition is touched. Monthly partition boundaries therefore carry no semantics whatsoever: the first row of month *N+1* has `prev_hash` = the last row of month *N*. `id` is a UUIDv7 and `created_at` comes from the injected `Clock`, both assigned inside the locked section, and `Append` clamps `created_at` to the head's `created_at` if a clock skew would otherwise produce an earlier value — chain order and `(created_at, id)` sort order must never be able to disagree, because verification walks the sort order.

**What is M0 and what is deferred.** M0 ships chain *construction* plus `audit.VerifyChain(ctx, from, to)` as a domain function exercised by the integration tests. Deferred with reason: the `vecingest verify-chain` **subcommand** is a PRD §10.1 **M7** gate item scoped to `votes` and a meeting id, so building the CLI at M0 would mean designing a surface against a table that does not exist until M7; and **daily anchoring** is `chains.anchor`, a §7.8 periodic worker job, which the proposal puts out of M0 scope along with every other periodic job — it additionally needs the `chain_anchors` table (§7.3) and a live `TSA_URL` (§8, §8.2), neither of which is M0. Because the verifier already exists as a tested domain function, the M7 CLI and the M1+ anchoring job are thin wrappers over it, not a redesign.

### D-P. TOTP: monotonic step counter for replay, SHA-256 for recovery codes

PRD §5.1 names `otplib`, which is a JavaScript library and does not apply. **`github.com/pquerna/otp`** provides RFC 6238 generation and validation; this design owns only the part no library can provide, because it needs storage.

- **Secret**: 160 bits (20 bytes) from `crypto/rand`, RFC 4648 base32 without padding in the provisioning URI `otpauth://totp/Vecingest:<email>?secret=…&issuer=Vecingest&algorithm=SHA1&digits=6&period=30`. Stored as AES-256-GCM ciphertext with the `v1:` version prefix in `user_mfa.totp_secret_encrypted` — this is the real M0 consumer of `ENCRYPTION_KEY` that D-I claims.
- **Parameters**: HMAC-SHA1, 30-second period, 6 digits, ±1 step of drift tolerance. SHA-1 is the RFC 6238 default and the only algorithm every authenticator app implements; it is used here as an HMAC key-derivation primitive over a 160-bit secret, where its collision weaknesses do not apply. Time comes from the injected `Clock`, never `time.Now()`.
- **Replay protection** (the spec requirement with no schema behind it in the first pass): `user_mfa` gains `last_totp_step bigint NULL`. Verification computes the candidate codes for steps `T-1, T, T+1` with `totp.GenerateCodeCustom` and compares each with `crypto/subtle.ConstantTimeCompare` — comparing rather than calling `ValidateCustom` is what yields the *matched step number*, which the library does not return. **All three candidates are always compared and the highest matching step is the one recorded**, never the first match found: the loop must not break early. Two reasons, and both are required. Recording a lower step than the one actually presented would leave the higher step replayable; and stopping at the first match would make the comparison count depend on which step matched, which is a timing signal. A match is accepted only if `step > COALESCE(last_totp_step, -1)`, and it is committed with `UPDATE user_mfa SET last_totp_step = $step WHERE user_id = $1 AND (last_totp_step IS NULL OR last_totp_step < $step)`; zero rows affected means a concurrent request already consumed that step ⇒ reject. Monotonicity also kills the backward-drift replay (a `T-1` code presented after `T` was accepted).
- **The atomicity of that conditional update is isolation-level dependent, and the dependency is the design.** "Zero rows ⇒ a concurrent request won ⇒ reject" is a property of **READ COMMITTED** only, which is PostgreSQL's default and the level every M0 transaction runs at. Under READ COMMITTED an `UPDATE` that blocks on a concurrent writer re-evaluates its `WHERE` against the *updated* row once the other transaction commits, so the loser's predicate `last_totp_step < $step` is now false and it affects zero rows — exactly the branch this design relies on. Under **REPEATABLE READ or SERIALIZABLE** that re-evaluation does not happen: PostgreSQL raises `ERROR 40001 could not serialize access due to concurrent update` instead, the "zero rows" branch never runs, and an unhandled 40001 would surface to the client as a `500` on a *correct* rejection. This is recorded as a dependency, not as an unconditional property. Two obligations follow: `internal/db` MUST NOT raise the isolation level of the MFA verification transaction; and if it is ever raised for any reason, `mfa.VerifyTOTP` MUST classify `pgerrcode.SerializationFailure` (`40001`) as the same outcome as zero-rows — reject with `AUTH_TOTP_REPLAYED`, never a 500 — because at that point a serialization failure *is* the concurrent-consumption signal. An integration test asserts the two concurrent verifications outcome (exactly one wins) at the level actually configured.
- **Brute-force throttling: D-N's lockout does not cover TOTP, so a third counter key does.** Failed TOTP attempts do not advance `last_totp_step` — only a *successful* verification does — so the monotonic counter provides **zero** brute-force resistance within a step window: an attacker may try every one of the 10⁶ codes against the same step until the window rolls. RFC 4226 §7.3 requires throttling, and D-N's progressive lockout counts *login* failures under `auth:fail:email:*` and `auth:fail:ip:*`, which a TOTP submission on an already-authenticated first factor never touches. The `AttemptCounter` seam of D-N is therefore extended with a third key family, using the same store, window and threshold: `auth:fail:totp:<user_id>` and, independently, the existing `auth:fail:ip:<canonicalized IP>` key, window 15 min, threshold 5, with the same escalating 15/30/60-minute block. A successful TOTP verification resets the per-user key and not the IP key, mirroring D-N. Recovery-code verification failures increment the same per-user key, because a recovery code is the alternative second factor for the same account and must not be a way around the TOTP budget. No enumeration channel is opened: the first factor is already proven, so the user is known to the attacker in every case this counter covers.
- **Recovery codes**: 10 codes, 128 bits each from `crypto/rand`, rendered base32 in `xxxx-xxxx-xxxx` groups, shown exactly once. Hashed with **SHA-256**, not Argon2id, and stored hex in `recovery_codes_hashed text[]`. Rationale: a recovery code is a 128-bit uniformly random value, not a human-chosen password, so an offline attacker gains nothing from a memory-hard KDF that a 128-bit search space has already denied them — while Argon2id at `m=19456 KiB` would cost 10 × 19 MiB per verification attempt, since the submitted code must be compared against every unused entry. SHA-256 is also exactly what PRD §5.1 prescribes for the other short single-use secrets. Verification normalizes (uppercase, dashes stripped), compares against **all** entries in constant time with no early exit, and consumes the match with `UPDATE user_mfa SET recovery_codes_hashed = array_remove(recovery_codes_hashed, $2) WHERE user_id = $1 AND $2 = ANY(recovery_codes_hashed)`; zero rows affected ⇒ already used ⇒ reject.

### D-Q. `/v1/health/ready` probes the database at M0, and object storage joins by registration

The spec requires readiness to succeed only when database **and object storage** are reachable (PRD §7.7 "BD, R2"), while the proposal puts R2 out of M0 scope beyond configuration placeholders. Settled rather than left open:

`ready` walks a registry of `ReadinessCheck` values wired in `cmd/vecingest`, each with a name and a `Check(ctx) error` bounded by a 2-second timeout. At M0 exactly **one** check is registered — `postgres`, a `pool.Ping` plus `SELECT 1` on the serve pool. The `storage` check is defined against the same interface and registered at M2, when an R2 client first exists.

**Why not probe R2 at M0.** There is no R2 client and no code path that touches object storage, so a probe would either be a fabricated success or a real network call to a dependency the process does not have. The second option is actively harmful: `deploy/docker-compose.yml` gates `worker` on `api: {condition: service_healthy}`, so a Cloudflare hiccup would take down a stack that is not using Cloudflare. The readiness contract this design implements is "every dependency the process actually has is reachable" — at M0 that set is `{postgres}`, and the spec's requirement is satisfied by construction at the moment object storage becomes a dependency. Externally, `/v1/health/ready` returns `200 {"ok":true}` or `503 {"ok":false}` with **no** dependency detail (PRD §7.7 exposes only `/health/live` outward; `ready` is consumed by Docker on the internal network). `vecingest health --ready` calls `127.0.0.1:$PORT/v1/health/ready` and exits 0 or 1.

### D-R. `ServeDB`/`WorkerDB` and `ReadDB`/`WriteDB` split, in configuration, from M0

PRD §7.10 point 2 lists this under "Qué está preparado desde M0" and §7.7 says plainly "El código se escribe desde M0 con esa separación (`ServeDB` con pooler, `WorkerDB` directo)". A single pgx pool would defer a change that costs nothing now and rewrites every call site later.

This decision is PRD-anchored but was, until this pass, carried by no spec requirement. The four properties below are written as **externally observable assertions** rather than as intentions, so that the requirement covering them is testable exactly as stated and the design text and the requirement cannot drift apart:

```go
// distinct named types, not two fields of the same type — see property 3
type ReadDB  struct{ pool *pgxpool.Pool }   // Query/QueryRow/Exec only
type WriteDB struct{ pool *pgxpool.Pool }   // the only type exposing Begin/BeginTx
type Handles struct{ Write WriteDB; Read ReadDB }   // db.Handles
```

1. **Two distinct pool objects, always.** `serve` builds `ServeDB` with `Write` on `DATABASE_URL` and `Read` on the resolved read DSN. At M0 both DSNs resolve to the same node, and `Read` is nonetheless a **separate `*pgxpool.Pool` instance with its own `MaxConns`** — assertable: the two pointers differ, and `Read.Stat().MaxConns() != Write.Stat().MaxConns()` under the M0 defaults.
2. **Exec mode differs by process role, observably.** `serve`'s pools are configured with `QueryExecModeCacheDescribe` so PgBouncer transaction pooling drops in at phase B without a code change (§7.7); `worker` builds `WorkerDB` as a direct connection with pgx's default exec mode, because River and `LISTEN/NOTIFY` cannot live behind a transaction pooler (§7.7). Assertable from the built `pgxpool.Config` of each process role without opening a connection.
3. **A read handle cannot reach a transaction — at compile time, not at review time, for every caller outside `internal/db`.** `ReadDB` and `WriteDB` are distinct named types and only `WriteDB` exposes `Begin`/`BeginTx`, so passing a read handle where a transaction is started does not compile. The guarantee is scoped, not absolute: Go visibility is package-scoped and `pool` is an unexported field, so code inside `internal/db` itself can still reach `.pool.Begin(ctx)` directly regardless of which named type wraps it. That residual is harmless here because every domain service constructor — where the whole point of the split applies — lives outside `internal/db`, which is exactly where the compile-time guarantee needs to hold. Every domain service constructor takes `ReadDB` for pure reads and `WriteDB` for everything else; anything inside a transaction and every read-then-write sequence takes `WriteDB`. Refresh rotation, `audit.Append`, TOTP step consumption and every lockout counter update are `WriteDB` by construction. The previous formulation ("a `Read` handle passed into a `pgx.Tx` scope is a review blocker") is retained only as the residual case a type cannot catch — a service that takes `ReadDB` and performs a logically read-then-write sequence through two statements — and that residual is the only part left to PR review.
4. **No new stack variable.** `internal/config` recognises optional `DATABASE_URL_READ` and `DATABASE_URL_WORKER`; when unset — always, at M0 — both fall back to `DATABASE_URL`. PRD §8.2's active variable list is unchanged; `.env.example` carries them only in a commented "fase B" block, so the template documents the seam without adding a variable the PRD does not list. Assertable: with neither override set, the process starts and both handles work.

### D-S. `seed` and `worker`: the two subcommands the earlier passes named but never designed

Both appear in PRD §10's M0 row and in the §7.2 tree, and both were carried as a name only. Their configuration requirement sets are in D-I's table; their behaviour is here.

**`vecingest seed` — development fixtures, hard-gated on `APP_ENV`.** PRD §8 (line 1106) specifies the full fixture set (one despacho, two communities with viviendas, a verified company, one user per role with a known password) and gates it with **"solo si `APP_ENV != production`"**. Note the variable: the PRD uses `APP_ENV`, not `NODE_ENV`; the Go backend has no `NODE_ENV` and `internal/config` must not accept one.

- **The guard is the first thing the command does**, before opening a connection: if `APP_ENV == "production"`, print `seed refuses to run with APP_ENV=production` to stderr and exit non-zero. It is a positive equality check against the one forbidden value, and an *unset or unrecognised* `APP_ENV` is also refused rather than defaulting to permissive — §8.2 defaults `APP_ENV` to `production`, so an absent variable means production. This is the one place where fail-closed and the PRD's literal `!=` differ, and fail-closed wins.
- **M0 fixture scope is degenerate, and that is the design, not an omission.** Despachos, communities, viviendas and companies are M1 entities with no M0 tables (proposal assumption 2), so at M0 `seed` can only create what M0 has: a small fixed set of **users** with a known development password, one flagged `is_superadmin` and the rest plain. Each M1 entity is added to `seed` in the migration-set change that creates its table, so the fixture set grows with the schema rather than being written against tables that do not exist.
- **Idempotent**, like `bootstrap-superadmin`: `INSERT ... ON CONFLICT (email) DO NOTHING`, never touching an existing row's password, exit 0 on a second run.
- **It bypasses HIBP and the password policy deliberately.** The fixture password is short, well known and published in the README; running it through `PasswordPolicy` or `HIBPChecker` would either fail or make `seed` require network access in CI. Argon2id hashing still applies, so the stored shape is identical to a real account. `seed` is the **only** caller permitted to construct a `users` row without the policy check, and that exemption lives in the `seed` command package, never in `internal/domain/auth`.
- It writes `audit_log` through `audit.Append` like every other writer, so the chain has no gap after seeding.

**`vecingest worker` — a real deliverable with zero jobs.** M0 registers no job workers (D-L), so the command's purpose is to make the process, the compose service and the healthcheck real:

- Builds `WorkerDB` per D-R, starts a River client with **zero registered workers** plus leader election, and starts nothing else — no HTTP listener, no mail pool, no rate limiter.
- Graceful shutdown on `SIGTERM` within the compose `stop_grace_period: 30s`: stop the River client, wait for the leader election to release, close the pool.
- `vecingest health --worker` is the compose healthcheck and is the reason `health` needs `DATABASE_URL`: it asserts this client's own `river_client` heartbeat row is fresher than one heartbeat interval, exiting 0 or 1. A worker that has lost the database is therefore unhealthy, which is the only failure mode a jobless worker has.
- Because it registers no workers, a job enqueued by mistake would sit unclaimed; there are no producers at M0 (D-L), and an integration test asserts the `river_job` table is empty after a full E2E run, which is what turns "zero producers" from a claim into a check.

## Data Flow

### Startup and migration (multi-step, multi-actor)

```
api container        vecingest serve --migrate     Postgres
     │                        │                        │
     ├─ config.Load ─────────►│  (fail-fast; unset ENCRYPTION_KEY)
     │                        │                        │
     │        goose provider A (BOOTSTRAP_DATABASE_URL, superuser)
     │                        ├── pg_advisory_lock(5432001) ──────────►│
     │                        ├── 00001_roles.go (NoTx, reads env):    │
     │                        │     CREATE ROLE vecingest_owner LOGIN  │
     │                        │     CREATE ROLE app_rw LOGIN (APP_DB_PASSWORD)
     │                        │     CREATE EXTENSION btree_gist, pg_stat_statements
     │                        │     ALTER SCHEMA public OWNER TO vecingest_owner
     │                        │     REVOKE ALL ON SCHEMA public FROM PUBLIC
     │                        │     GRANT USAGE ON SCHEMA public TO app_rw
     │                        │     ALTER DEFAULT PRIVILEGES FOR ROLE vecingest_owner
     │                        │       GRANT SELECT,INSERT ON TABLES TO app_rw   ← fail-closed
     │                        │     CREATE TABLE append_only_relations (audit_log)
     │                        │     CREATE EVENT TRIGGER vecingest_append_only_guard
     │                        │       ON ddl_command_end  (superuser-only DDL)
     │                        │       WHEN TAG IN ('CREATE TABLE','CREATE TABLE AS',
     │                        │                    'ALTER TABLE')   ← no ATTACH tag exists
     │                        │     assert: app_rw NOT member of vecingest_owner
     │                        ◄── pg_advisory_unlock ─────────────────►│
     │                        │                        │
     │        goose provider B (MIGRATIONS_DATABASE_URL, vecingest_owner)
     │                        ├── pg_advisory_lock(5432002) ──────────►│   ← worker blocks here
     │                        ├── 0001 auth schema (+ explicit GRANT UPDATE,DELETE per table)
     │                        ├── 0002 audit_log (partitioned, 12 months)
     │                        │   + REVOKE UPDATE,DELETE,TRUNCATE FROM app_rw,PUBLIC
     │                        │   ⤷ guard fires per partition, re-revoking automatically
     │                        ├── 0003 assert I1 · I2 · I3 · I4  (RAISE EXCEPTION ⇒ deploy fails)
     │                        ├── 0004 river.go (NoTx, pinned TargetVersion) + river grants
     │                        ◄── unlock ─────────────────────────────►│
     │                        │                        │
     ├─ pgx pool as app_rw ──►│  listener: LISTEN session_revoked      │
     └─ chi router listens :3000
```

### Refresh rotation and reuse detection

```
Client (web)          api /v1/auth/refresh          sessions            Cache/NOTIFY
   │ (after a page reload only)                        │                    │
   │ GET /v1/auth/refresh/csrf ►│ subpath of Path=/v1/auth/refresh ⇒ cookie IS sent
   │                       │ cookie → live row → mint token bound to that row │
   │◄── 200 {csrf_token} ──┤ safe method, no rotation; no-store + Vary: Origin
   │   held in memory      │ + CORP: same-origin; ACAO only for https://app.DOMAIN
   │                       │                            │                    │
   │ cookie + X-CSRF-Token ►│                            │                    │
   │  (token from memory)  ├─ transport check: cookie XOR body else 400      │
   │                       ├─ CrossOriginProtection + Origin check           │
   │                       ├─ HMAC verify CSRF; family_id AND session_id must │
   │                       │  match the row the cookie hashes to             │
   │                       ├─ SHA-256(refresh) ────────►│ lookup by hash     │
   │                       │        ┌── row live ───────┤                    │
   │                       │        │  revoke old row, INSERT new row (same family_id)
   │                       │◄───────┘  issue access(15m) + refresh cookie     │
   │◄─ 200 + Set-Cookie ───┤     + new csrf_token IN THE BODY (never a cookie)│
   │   (access + csrf both kept in memory; old csrf now dead — session_id changed)
   │                       │        └── row already revoked  ⇒ REUSE         │
   │                       │           UPDATE … WHERE family_id=$1 ──────────►│
   │                       │           audit.Append (chain)  NOTIFY session_revoked
   │◄── 401 AUTH_REFRESH_REUSED                          │  every replica drops
                                                          the family's access tokens
```

### Audit-chain append (multi-step; the lock ordering is the design)

```
domain service        audit.Append(ctx, tx, Entry)      Postgres (Write handle)
   │ same tx as the change ►│                                   │
   │                        ├─ pg_advisory_xact_lock(5432003) ─►│  serializes the chain
   │                        ├─ SELECT hash, created_at, id      │  ← partitioned parent,
   │                        │  FROM audit_log                   │    merge append, so a
   │                        │  ORDER BY created_at DESC, id DESC│    month boundary is
   │                        │  LIMIT 1                          │    not a chain boundary
   │                        ├─ id = uuidv7(); created_at = Clock.Now()
   │                        │  clamp created_at ≥ head.created_at  (order must not fork)
   │                        ├─ hash = SHA-256(prev ‖ F(fields)…) │
   │                        └─ INSERT … (prev_hash, hash) ──────►│
   │◄── returns; lock released at COMMIT with the domain change ─┘
```

### Superadmin bootstrap and first login

```
Operator                     vecingest bootstrap-superadmin        users / user_mfa
   │ --email … --password-stdin ──►│                                     │
   │                               ├─ policy(15|12) + HIBP + Argon2id     │
   │                               ├─ INSERT … ON CONFLICT (email) DO NOTHING
   │                               │    0 rows ⇒ load existing, ensure is_superadmin,
   │                               │             DO NOT touch the password, exit 0
   │◄── "created" | "already exists, no change" (exit 0 both) ────────────┤
Superadmin                   POST /v1/auth/superadmin/login               │
   │ email+password ──────────────►│ non-superadmin ⇒ AUTH_INVALID_CREDENTIALS (generic)
   │                               │ no user_mfa row ⇒ 403 MFA_ENROLLMENT_REQUIRED + enrol
   │ TOTP code ───────────────────►│ verify (injected Clock) ⇒ session TTL 8h (§6.1)
```

## File Changes

| File | Action | Description |
|---|---|---|
| `go.work`, `Makefile`, `lefthook.yml`, `renovate.json`, `pnpm-workspace.yaml`, `turbo.json` | Create | Single workspace entrypoint: `make dev/gen/test/lint/test-e2e/lint-scope`, plus `make test-short` (`go test -race -short ./...`, the Docker-free fast suite) and **`make test-golden-update`** — the only sanctioned `-update` path for the audit-chain goldens |
| `api/go.mod` | Create | Module `github.com/tu-org/vecingest` (§7.2; org segment substituted at WU-0), `go 1.26.0` (D-K/D-L), `github.com/riverqueue/river v0.47.0`, `tool` directives pinning `sqlc` and the `goose` CLI |
| `api/cmd/vecingest/main.go` | Create | stdlib-`flag` subcommand dispatcher over the **six** M0 subcommands — `serve`, `worker`, `migrate`, `seed`, `bootstrap-superadmin`, `health` (no cobra: six commands with no nesting, no shell completion requirement and no dynamic help do not justify the dependency or its size against the <30 MB budget; the conclusion is unchanged from the earlier "5 commands" miscount, and would still hold at eight); imports `_ "time/tzdata"` |
| `api/cmd/vecingest/seed.go`, `worker.go` | Create | D-S: `seed`'s `APP_ENV` production guard + idempotent M0 user fixtures; `worker`'s jobless River client, leader election and graceful shutdown |
| `api/cmd/openapi-gen/main.go` | Create | Emits `api/openapi/openapi.yaml`; deliberately **not** in the shipped image |
| `api/internal/config/` | Create | `Load`, per-subcommand requirement sets, `secrets.Holder` |
| `api/internal/http/{router,middleware,handlers}/` | Create | chi chain D-H, huma handlers, `dto` in/out structs (`DisallowUnknownFields`) |
| `api/internal/domain/auth/` | Create | `password` (Argon2id+policy+HIBP), `token` (access/refresh/CSRF), `session`, `mfa` (TOTP + step replay + recovery codes, D-P), `lockout` (D-N) |
| `api/internal/domain/audit/` | Create | `Append` (the only `audit_log` writer) + `VerifyChain`, D-O |
| `api/internal/db/` | Create | `sqlc.yaml`, `queries/*.sql`, generated `*.sql.go` (pgx/v5), `Handles{Write,Read}` per process role (D-R) |
| `api/internal/observability/slogpii/` | Create | Wrapping redaction handler |
| `api/internal/platform/{limiter,cache,queue,attempts}/` | Create | Phase-A implementations behind the five interfaces (`attempts` = `AttemptCounter`, D-N) |
| `api/internal/health/` | Create | `ReadinessCheck` registry; `postgres` check registered at M0, `storage` at M2 (D-Q) |
| `api/internal/mail/` | Create | `Mailer` interface + go-mail SMTP sender + bounded async dispatch pool + `html/template` templates `password_reset.html` and `login_lockout.html` (D-N); fake in tests |
| `api/migrations/bootstrap/`, `api/migrations/schema/` | Create | Two embedded goose sets (D-A) |
| `api/test/` | Create | Testcontainers harness, privilege-matrix and e2e suites |
| `api/Dockerfile`, `app/Dockerfile.web`, `site/Dockerfile` | Create | D-K |
| `deploy/docker-compose.yml`, `docker-compose.dev.yml`, `.env.example` | Create | §8.1/§8.2 with the two corrections in D-K; `.env.example` adds a commented "fase B" block for `DATABASE_URL_READ`/`DATABASE_URL_WORKER` (D-R) without changing §8.2's active list |
| `packages/shared/` | Create | Generated TS client + Zod + stable error codes; never hand-edited |
| `app/` | Create | Expo skeleton, `(auth)/login.tsx`, RN Paper theme, `expo-secure-store` |
| `.github/workflows/{ci,security,deploy}.yml`, `PULL_REQUEST_TEMPLATE.md` | Create | D-M |
| `docs/security/{threat-model.md,gates/M0.md}` | Create | §6.1 table + the two-checkpoint gate |
| `openspec/config.yaml` | Modify | M0 coverage carve-out excluding `internal/legal/...` (proposal assumption 4), and the stale "Go 1.23+" in `context` and `projects[].stack` corrected to Go 1.26+ per PRD §7.1 (D-L) |

## Interfaces / Contracts

### Schema — exact DDL shape (owner-run set)

```sql
-- users  (scope column: id / user_id downstream)
id                 uuid        PRIMARY KEY,            -- UUIDv7, generated in Go (§7.3)
email              text        NOT NULL,               -- normalized lower+trim in Go; UNIQUE INDEX
password_hash      text        NOT NULL,               -- PHC string
name               text        NOT NULL,
phone              text        NULL,
phone_verified_at  timestamptz NULL,
locale             text        NOT NULL DEFAULT 'es',
avatar_key         text        NULL,
id_document_encrypted bytea    NULL,                   -- AES-256-GCM, 'v1:' prefix
notification_prefs jsonb       NOT NULL DEFAULT '{}'::jsonb,
is_superadmin      boolean     NOT NULL DEFAULT false,
last_login_at      timestamptz NULL,
created_at         timestamptz NOT NULL DEFAULT now(),
updated_at         timestamptz NOT NULL DEFAULT now(),
deleted_at         timestamptz NULL
-- CREATE UNIQUE INDEX users_email_key ON users (email);          -- no citext: keeps §7.3's
-- CREATE INDEX users_superadmin_idx ON users (id) WHERE is_superadmin;  -- extension list intact

-- sessions  (scope column: user_id; PRD §7.3 columns exactly)
id uuid PK, user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
family_id uuid NOT NULL, refresh_token_hash bytea NOT NULL,      -- sha256, 32 bytes
device_name text NULL, platform text NOT NULL CHECK (platform IN ('ios','android','web')),
ip inet NULL, last_used_at timestamptz NULL, expires_at timestamptz NOT NULL,
revoked_at timestamptz NULL, created_at/updated_at timestamptz NOT NULL DEFAULT now()
-- UNIQUE (refresh_token_hash); INDEX (user_id)  [§7.3 mandatory]; INDEX (family_id);
-- INDEX (expires_at) WHERE revoked_at IS NULL;

-- password_reset_tokens: id uuid PK, user_id FK, token_hash bytea NOT NULL UNIQUE,
--   expires_at timestamptz NOT NULL, used_at timestamptz NULL, requested_ip inet, created_at
--   INDEX (user_id, created_at DESC);                    -- a user's recent requests
--   INDEX (expires_at) WHERE used_at IS NULL;            -- M1 purge job, live rows only
-- user_mfa: user_id uuid PK REFERENCES users(id) ON DELETE CASCADE,
--   totp_secret_encrypted bytea NOT NULL, enabled_at timestamptz NULL,
--   last_totp_step bigint NULL,                          -- D-P replay protection
--   recovery_codes_hashed text[] NOT NULL DEFAULT '{}', created_at, updated_at
--   no secondary index: the PK is the only access path, and last_totp_step is only
--   ever read and written through that same key.
-- otp_challenges: id uuid PK, user_id FK, purpose text CHECK (purpose IN
--   ('phone_verify','vote','sign_minutes','sensitive_action')), code_hash bytea NOT NULL,
--   channel text CHECK (channel IN ('sms','totp')), expires_at timestamptz NOT NULL,
--   attempts int NOT NULL DEFAULT 0, verified_at timestamptz NULL, created_at
--   INDEX (user_id, purpose, created_at DESC);            -- the only lookup path
--   INDEX (expires_at) WHERE verified_at IS NULL;         -- M1 purge job
--   deliberately NO index on code_hash: indexing it invites an equality-lookup code
--   path that skips the user scope; the code is compared in constant time after the
--   scoped lookup, never used to find the row.
--   [provisioned at M0; first writer is M1 phone verification]

-- audit_log  APPEND-ONLY, PARTITION BY RANGE (created_at), 12 months pre-created
id uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
user_id uuid NULL, community_id uuid NULL,          -- tenant column from M0; FK added in M1
action text NOT NULL, entity text NOT NULL, entity_id uuid NULL,
before jsonb NULL, after jsonb NULL, ip inet NULL, request_id text NULL,
prev_hash bytea NULL, hash bytea NOT NULL,
PRIMARY KEY (created_at, id)                         -- partition key must be in the PK
-- INDEX (community_id, created_at DESC); INDEX (user_id, created_at DESC);
-- INDEX (entity, entity_id, created_at DESC);
-- REVOKE UPDATE, DELETE, TRUNCATE ON audit_log FROM app_rw, PUBLIC;  (+ every partition)

-- append_only_relations  (bootstrap set, superuser-owned; D-B mechanism 2)
--   relname regclass PRIMARY KEY   -- seeded with 'audit_log'; votes/time_entries join at M7/M8
--   read by the ddl_command_end event trigger that re-revokes on every new partition
```

`sqlc.yaml` overrides: `uuid`→`github.com/google/uuid.UUID`, `timestamptz`→`time.Time`, `inet`→`netip.Addr`, `jsonb`→`json.RawMessage`, **`numeric`→`shopspring/decimal.Decimal`** (the money guard of rule 2, installed before any money column exists).

### Go seams

```go
type Limiter interface { Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) }
type Cache   interface { Get(ctx, key) ([]byte, bool); SetTTL(ctx, key, val, ttl) error; Delete(ctx, key) error }
type Queue   interface { EnqueueTx(ctx context.Context, tx pgx.Tx, job Job) error }   // no M0 producer
type Mailer  interface { Send(ctx context.Context, msg Message) error }
type HIBPChecker interface { Pwned(ctx context.Context, password string) (bool, error) }
type Clock   interface { Now() time.Time }                                            // never time.Now() in domain

// D-N: failures, not requests — Limiter cannot express lockout
type AttemptCounter interface {
    Fail(ctx context.Context, key string, window time.Duration) (int, error)
    Count(ctx context.Context, key string, window time.Duration) (int, error)
    Reset(ctx context.Context, key string) error
}

// D-Q: readiness by registration; M0 registers only "postgres", M2 adds "storage"
type ReadinessCheck interface { Name() string; Check(ctx context.Context) error }

// D-R: the phase-B split, present from M0 with both DSNs resolving to one node.
// Distinct named types: only WriteDB exposes Begin, so a read handle in a
// transaction scope is a compile error rather than a review finding, for
// every caller outside internal/db (pool is unexported).
type ReadDB  struct{ pool *pgxpool.Pool }
type WriteDB struct{ pool *pgxpool.Pool }   // sole owner of Begin/BeginTx
type Handles struct{ Write WriteDB; Read ReadDB }
```

### `GET /v1/me` — M0 contract

```go
type MeResponse struct {
    ID           string `json:"id"            format:"uuid"`
    Email        string `json:"email"         format:"email"`
    IsSuperadmin bool   `json:"is_superadmin"`
}
```

No role. No memberships. PRD §7.4 line 795 reads `GET /v1/me  # usuario + membresías`, and that endpoint list is **not** milestone-tagged; the memberships half is unimplementable at M0 because offices, communities and `unit_members` are M1 (proposal assumption 2). This is a deferral, not a divergence: adding `memberships` at M1 is an additive OpenAPI change and therefore non-breaking for the generated client.

**M0 endpoint set**: `POST /v1/auth/login`, `POST /v1/auth/refresh`, **`GET /v1/auth/refresh/csrf`** (D-E — under the refresh cookie's path so the cookie is actually attached), `POST /v1/auth/logout` (Bearer-authenticated, `sid`-scoped, D-E), `POST /v1/auth/forgot-password`, `POST /v1/auth/reset-password`, `POST /v1/auth/superadmin/login`, `GET /v1/me`, **`GET /v1/me/sessions`**, **`DELETE /v1/me/sessions/:id`**, `GET /v1/health/live`, `GET /v1/health/ready`.

The two session endpoints are in M0. The first pass excluded them; that was wrong. PRD §5.1 states "El usuario puede cerrar sesiones remotas desde su perfil", §7.4 lists both, and M0 already builds the entire mechanism they expose — family lineage, the listing query, family-wide revocation and `LISTEN/NOTIFY` propagation. Shipping the mechanism with no way to reach it buys nothing and leaves a required capability untestable end to end.

```go
type SessionSummary struct {
    ID         string `json:"id"           format:"uuid"`  // family_id — the logical session
    DeviceName string `json:"device_name,omitempty"`
    Platform   string `json:"platform"     enum:"ios,android,web"`
    LastUsedAt string `json:"last_used_at" format:"date-time"`
    CreatedAt  string `json:"created_at"   format:"date-time"`
    Current    bool   `json:"current"`                     // the family of the calling access token's sid
}
```

`GET /v1/me/sessions` returns the live families of the caller: `GROUP BY family_id`, newest non-revoked, non-expired row per family, ordered `last_used_at DESC`. Raw or hashed refresh tokens, IPs and `refresh_token_hash` never appear in the output struct (PRD §6.1 mass-assignment control: output structs carry no `*_hash` field).

`DELETE /v1/me/sessions/:id` — **`:id` is the `family_id`**, because the family is what the UI lists and what revocation actually operates on. It runs the same family-wide `UPDATE sessions SET revoked_at = now() WHERE family_id = $1 AND user_id = $2 AND revoked_at IS NULL`, writes `audit.Append`, and publishes `session_revoked`, so an access token issued under that family stops working within the D-D fail-closed bound rather than at its 15-minute expiry. Deleting one's own current family is allowed and is exactly `logout`. A family belonging to another user, or one that does not exist, returns **`404`, never `403`** — a 403 would confirm that someone else's session id exists.

### Expo login screen

Two themes from the machine-readable `colors` blocks of `docs/design/vecingest-light.md` and `vecingest-dark.md`, wired to `PaperProvider` and switched by `useColorScheme()`. **Both themes ship at M0 because the proposal's scope says so and a complete dark design system already exists in the repository** — `docs/design/README.md` does *not* mandate dark mode, and the first pass attributed the requirement to it incorrectly. What that README does impose, and what this screen must satisfy, is its list of non-negotiable rules: no gradients and no drop shadows, one single interactive accent, one primary button per screen, Spanish sentence-case copy with no exclamation marks and verb-first buttons, minimum 11 px type and 44 px touch targets, tabular figures for numeric values, and text on a coloured background using the darkest tone of that same family rather than black.

Interactive accent is single: `#185FA5` light, `#85B7EB` dark. Form per the light system's field pattern: label 13px/500 above the field, 44 px field height, 8 px radius, border `#D6DAD5` → `#185FA5` on focus, error text below in `#A32D2D`, exactly one primary button. React Hook Form + `zodResolver` over the **generated** `packages/shared` schema; error strings resolved by stable code, never by server text. Tokens go to `expo-secure-store`.

### D-M. CI topology

| Workflow | Jobs |
|---|---|
| `ci.yml` (PR + push `main`) | **go**: gofumpt check, golangci-lint (gosec, errcheck, sqlclosecheck, forbidigo float ban), `go build ./...`, a `sudo systemctl stop docker` step immediately before the test step — `ubuntu-latest` ships Docker installed and running by default, so without this step an unguarded Testcontainers acquisition in the fast job would simply succeed against a live daemon and the job would only get slower, not fail; this step is what makes "no Docker daemon available" true rather than merely claimed — then **`go test -race -short ./...`**, which is what enforces the skill's skippability rule mechanically rather than by convention now that the daemon is actually absent: a unit test that silently acquires a container fails the fast job instead of slowing it down; coverage ≥ 80 % on `internal/domain/...` only at M0, and a clean `git diff` over `**/testdata/*.golden` so a golden can only change through `make test-golden-update` · **gen**: Go + Node in one job, runs `make gen` then **`git diff --exit-code`** (the dirty-diff gate) · **ts**: `pnpm install --frozen-lockfile`, turbo lint/typecheck/test · **e2e**: `make test-e2e` with Testcontainers Postgres 17 — the job keeps the `e2e` name for continuity with the Makefile target, but the layer it runs is `integration_layer` in `openspec/config.yaml`'s taxonomy, which correctly declares `e2e_layer: false` for `api/`; no config change is implied · **scope**: `make lint-scope` |
| `security.yml` | gitleaks, govulncheck, gosec, Semgrep (`p/owasp-top-ten` + Go + TS), `pnpm audit --audit-level=high`, Trivy fs — all blocking; a deliberately-failing probe PR is the §10.1 evidence |
| `deploy.yml` (push `main`, tags) | Build + push GHCR `sha-<short>` and `latest`, plus `v<semver>` on a tag; Trivy image scan blocking on HIGH/CRITICAL; advisory image-size report against the 30 MB budget; then the Portainer stack redeploy webhook |

`make gen` order is fixed and reproducible: `sqlc generate` → `go run ./cmd/openapi-gen` → `openapi-typescript` → `openapi-zod-client` → `gofumpt -w`. Generator versions are pinned (Go tools via `go.mod` `tool` directives, JS tools via `package.json` + the lockfile); an unpinned generator makes the dirty-diff gate flap. The gate is wired at WU-5 (once real auth structs exist) and enforced from WU-11, resolving the chicken-and-egg the proposal flags.

## Testing Strategy

Strict TDD from WU-2 onward; WU-0/8/9 are setup and configuration verified by a green pipeline and `docker inspect`, exactly as the proposal's discipline table states.

Two cross-cutting rules from the project's `go-testing` skill bind every row below and are stated once here rather than repeated per row.

**Skippability.** Every test that starts a container, runs an external command or executes a built binary MUST begin with `if testing.Short() { t.Skip("integration: requires Docker/Postgres") }`. That covers **all seven** slow rows — the privilege-matrix/migration row, the audit-chain row, the readiness row that stops a Testcontainers Postgres mid-test, the DB-handles row (D-R), the subcommands row (D-S), the E2E row, and the startup row that builds the distroless image and probes it — not only the first, which was the earlier passes' error. No test is left unguarded; the count was simply stale. The consequence is a real contract, not a formality: `go test -short ./...` MUST pass with no Docker daemon available, and CI's fast unit job runs exactly that, so a unit test that silently acquires a container fails the fast job instead of slowing it down.

**Golden files.** The audit-chain golden hashes are the only golden artifacts at M0. They are regenerated **only** through the repo's `-update` path and never hand-edited: `internal/domain/audit` declares `var update = flag.Bool("update", false, "regenerate golden files")`, the test writes `testdata/*.golden` when it is set and compares otherwise, and the workspace exposes `make test-golden-update` (= `cd api && go test ./internal/domain/audit/... -run TestAuditChainGolden -update`). The documented loop is: run with `-update`, inspect the diff, then rerun **without** `-update` and commit both the code and the regenerated files. Goldens must be deterministic — the chain test uses a fixed `Clock`, fixed UUIDv7 values from a seeded source and fixed field inputs, because a golden hash over `time.Now()` would regenerate to a new value on every run and destroy the diff's meaning. CI runs the suite without `-update`; a dirty `testdata/` diff after the run is a failure, which is what stops a golden from being "updated" by an accidental commit of a run artifact.

**Vocabulary: this design has three layers, `openspec/config.yaml` declares two.** The config sets `e2e_layer: false` for `api/` and names `make test-e2e` as the **integration** command, while the rows below add a distinct "E2E" layer and D-M adds an `e2e` CI job running that same command. The config is right about the taxonomy and this design keeps its own row names, so the drift is resolved by naming rather than by adding a layer: `make test-e2e` remains **one** command running the whole `api/test/` suite — Testcontainers Postgres, real migrations, real handlers — and `e2e_layer: false` for `api/` stays correct, because a true E2E layer in this project's vocabulary means a device-driving Maestro flow, which is declared on `app/` only. The rows named "E2E" below are **in-process HTTP integration tests** against a huma test API and a real Postgres: no browser, no device, no deployed stack. The CI job keeps its `e2e` name for continuity with `make test-e2e`; D-M's table now says so explicitly. Nothing in `openspec/config.yaml` changes for this.

| Layer | What to test | Approach |
|---|---|---|
| Unit | Argon2id PHC encode/parse round-trip, tampered tag, bounds-checked params, `needsRehash`; password policy 15/12 table; CSRF forge/expiry/family-mismatch/**session-id-mismatch after rotation**; config missing-variable per subcommand; slog redaction across message, attr, nested group and `LogValuer`; trusted-proxy resolution table incl. spoofed leftmost XFF from an untrusted source | table-driven `t.Run`, `testify`, injected `Clock`, fake HIBP server, `t.TempDir()` where files are involved |
| Unit — audit chain (D-O) | Hash is stable for fixed input; a one-field change changes the hash; **field-boundary attack**: moving a character from `action` to `entity` changes the hash (proves the length prefixes work); `NULL` and `""` hash differently; `VerifyChain` accepts a good chain and pinpoints the first broken link | table-driven; golden hashes in `internal/domain/audit/testdata/*.golden`, regenerated **only** via `make test-golden-update` (`-update` flag) and re-run without it; fixed `Clock` and seeded UUIDv7 source so the goldens are deterministic |
| Unit — TOTP (D-P) | ±1 step accepted, ±2 rejected; **the same code rejected on second use**; a `T-1` code rejected after `T` was accepted; **when more than one candidate step matches, the highest is the one recorded**; the candidate loop never breaks early (assert all three comparisons ran); 160-bit secret length; recovery code single-use and constant-time comparison over all entries; **TOTP throttle**: 5 failed TOTP attempts in 15 min lock the `auth:fail:totp:<user_id>` key, a failed recovery-code attempt increments the same key, success resets the per-user key but not the IP key | injected `Clock`, fixed secret vectors, fake `AttemptCounter` |
| Unit — lockout (D-N) | 5 failures in 15 min lock by email and, independently, by IP across different emails; unknown email advances the counter identically to a known one; success resets the email counter but not the IP counter; escalating 15/30/60-min window; alert dedupe fires at most once per lock window | fake `AttemptCounter` + fake `Mailer`, injected `Clock` |
| Integration (Testcontainers PG 17) | Migrations up→down→up; **privilege matrix**: `app_rw` UPDATE/DELETE/TRUNCATE on `audit_log` *and* on a child partition all fail, not owner, not member of owner; invariants I1/I2/I3/**I4** assertions fire on a deliberately-planted `SECURITY DEFINER` function and on a deliberately-widened default-privilege baseline; **two independent partition-guard RED tests, both required** (D-B): (a) `CREATE TABLE … PARTITION OF audit_log` and (b) `CREATE TABLE … (LIKE audit_log)` followed by `ALTER TABLE audit_log ATTACH PARTITION …`, each as `vecingest_owner`, each asserting `app_rw` cannot UPDATE/DELETE/TRUNCATE the resulting child **with no explicit revoke anywhere in the test** — (b) is what actually proves which relation `pg_event_trigger_ddl_commands()` reports for ATTACH, which the documentation does not state; a `REVOKE` issued by a role other than the grantor removes nothing, and I1 still fails the deploy; two concurrent goose providers serialize on the advisory lock and converge to the same version; River v0.47.0 migrate at the pinned target; **`river_job` is empty after the full suite** (D-S: zero producers at M0); every sqlc query | real Postgres, no mocks; **`testing.Short()` skip guard** |
| Integration — audit chain (D-O) | 50 concurrent `Append` calls from 10 goroutines produce a chain with no gap and no fork, every `prev_hash` equal to its predecessor's `hash`; a chain that spans a **month partition boundary** verifies unbroken; an owner-connection `UPDATE` (bypassing `app_rw`'s revoke, simulating the §6.1 hostile operator) makes `VerifyChain` fail at exactly that row; TOTP step and recovery-code consumption are atomic under two concurrent verifications at **READ COMMITTED** (exactly one wins, the loser gets zero rows and a rejection, never a 500) | real Postgres, `errgroup`, advisory-lock contention exercised; **`testing.Short()` skip guard** |
| Integration — readiness (D-Q) | `/v1/health/ready` is 200 with the container up; killing the Postgres container yields non-2xx within the check timeout and no dependency detail in the body; `vecingest health --ready` exits 1 in that state | Testcontainers with the DB stopped mid-test; **`testing.Short()` skip guard** (it stops a container and runs the built binary) |
| Integration — DB handles (D-R) | `serve` yields two distinct `*pgxpool.Pool` instances with different `MaxConns`; `serve` pools carry `QueryExecModeCacheDescribe` and `worker`'s do not; both processes start with neither `DATABASE_URL_READ` nor `DATABASE_URL_WORKER` set; a compile-time assertion (a `_test` file that must fail to build, checked by `go vet`/a build-tagged negative test) that `ReadDB` exposes no `Begin` | real Postgres for the start path, `pgxpool.Config` inspection for the rest; **`testing.Short()` skip guard** |
| Integration — subcommands (D-S) | `seed` with `APP_ENV=production` exits non-zero and touches no row; with `APP_ENV` unset it also refuses (fail-closed); with `APP_ENV=development` it creates the fixture users and a second run changes nothing; `worker` starts with zero registered workers, acquires leadership, and shuts down cleanly on `SIGTERM` inside 30 s; `vecingest health --worker` exits 0 while it runs and 1 after the DB is stopped | real Postgres, real built binary, signal delivery; **`testing.Short()` skip guard** |
| E2E — in-process HTTP (huma test API + real PG; **not** a device or browser layer, see the vocabulary note above) | login → refresh rotation → reuse ⇒ family revoked ⇒ a still-unexpired access token is rejected; CSRF: foreign `Origin` → 403, valid cookie without token → 403, **a token from the previous refresh generation → 403**; **`GET /v1/auth/refresh/csrf` actually receives the refresh cookie** (the RFC 6265 subpath property, asserted through a real cookie jar that stores `Path=/v1/auth/refresh` — a request to `/v1/auth/csrf` must send **no** cookie, which is the regression test for the rejected path), restores a usable token after the client drops it (reload simulation), returns 401 once the family is revoked, and its response carries `Cache-Control: no-store`, `Vary: Origin` and `Cross-Origin-Resource-Policy: same-origin`; **`POST /v1/auth/logout` succeeds with a Bearer token and no cookie, revokes the whole `sid` family, emits a `Max-Age=0` `Set-Cookie` on `Path=/v1/auth/refresh`, and is rejected without a Bearer token**; body-path refresh from a native client succeeds without a CSRF token; cookie+body together → 400; superadmin TOTP on the separate route, non-superadmin rejected generically; reset flow single-use + 1 h expiry; `/v1/me` shape; **`GET /v1/me/sessions` lists families and never leaks a hash, `DELETE /v1/me/sessions/:id` kills the family and invalidates its live access token, another user's family id → 404**; 429 from the limiter, from login lockout and from the TOTP throttle | `go test -race`, deterministic clock; **`testing.Short()` skip guard** (real Postgres) |
| Frontend | Login screen renders and validates in light **and** dark, Zod resolver bound to the generated schema, token written to `expo-secure-store` | React Native Testing Library |
| Startup | Process refuses to start with a missing variable and names it without printing values, **per subcommand** — `worker` must start with no JWT secret, `health` with only `PORT`, `seed` with no `ENCRYPTION_KEY` (D-I's table, asserted row by row); `time.LoadLocation("Europe/Madrid")` succeeds inside the built distroless image | binary-level test; **`testing.Short()` skip guard** on the rows that build the image and execute the binary (the pure `config.Load` table stays a unit test and needs none) |

## Threat Matrix

The skill's matrix targets shell/VCS/PR-automation boundaries. This design introduces none of them in the product: the binary spawns no subprocess, runs no shell (there is none in `distroless/static`), and performs no git or PR automation. Every row is explicit `N/A`.

| Boundary | Applicability | Reason |
|---|---|---|
| Documentation-like paths | N/A | No file is classified or executed by content or extension at M0; the only uploads (M2) do not exist yet |
| Git repository selection | N/A | The product invokes no `git`; CI is declarative GitHub Actions, not composed commands |
| Commit state | N/A | No commit is created by any code path in this design |
| Push state | N/A | No push is performed by any code path |
| PR commands | N/A | No PR automation; `deploy.yml` publishes images and calls one Portainer webhook |

The adversarial boundaries this design *does* introduce are HTTP routing and database DDL/privilege state, and their cases are carried as first-class tests rather than in this table: spoofed `X-Forwarded-For`; cross-origin refresh; missing, forged, expired or previous-generation CSRF token; cross-site read of `GET /v1/auth/refresh/csrf`, including the cache-mediated variant a missing `Vary: Origin` would open and the no-CORS subresource oracle `Cross-Origin-Resource-Policy` closes; both-transport refresh; refresh reuse; logout without a Bearer token; lockout as a user-enumeration oracle; TOTP code replay and TOTP brute force within one step window; recovery-code reuse; direct DML against an `audit_log` child partition; a partition created without an explicit revoke, through **both** `PARTITION OF` and `ATTACH PARTITION`; a widened default-privilege baseline; a revoke issued by a role that is not the grantor; `seed` run against production; and a hostile-operator `UPDATE` against a chained `audit_log` row.

## Migration / Rollout

Greenfield: no data, no prior release. `goose down` to zero or dropping the database is a complete rollback; the bootstrap set's `down` drops the `vecingest_append_only_guard` event trigger and `append_only_relations`, resets the default-privilege baseline, then reassigns ownership before dropping `app_rw` and `vecingest_owner` — in that order, because dropping the roles first would leave an event trigger referencing a nonexistent grantee. Deployment rollback re-points `TAG` at the previous immutable `v<semver>` — never by mutating `latest`. Expand/contract (PRD §9) begins at the first real user account. `bootstrap-superadmin` is idempotent, so re-running after a rollback neither duplicates nor errors and never resets an existing password.

## Open Questions

- [x] ~~River release and minimum PostgreSQL major~~ — **RESOLVED**: v0.47.0 (2026-08-31, not a prerelease), module `github.com/riverqueue/river`, still 0.x with no API stability guarantee; its `go.mod` `go 1.26.0` sets the Go 1.26+ floor; no stated minimum PostgreSQL, tested against the three most recent majors, so PostgreSQL 17 is covered. Verified via the GitHub API on 2026-09-04. See D-L.
- [x] ~~Go toolchain floor~~ — **RESOLVED**: Go 1.26+ per PRD §7.1, which also covers Go 1.25's `net/http.CrossOriginProtection`. See D-K.
- [ ] **Three claims this design holds by inference rather than by documentation, each closed by a named test rather than left silent.** (a) Which relation `pg_event_trigger_ddl_commands()` reports for `CREATE TABLE ... PARTITION OF` and for `ALTER TABLE ... ATTACH PARTITION`: `objid` is documented only as "OID of the object itself" and the PostgreSQL 17 event-trigger chapter never mentions partitions at all; for ATTACH the plain reading points at the parent. D-B's handler is written to be correct under either reading by revoking over the bidirectional `pg_inherits` closure, and the two partition-guard integration tests are what actually establish the behaviour. (b) What role a `SECURITY INVOKER` event-trigger function executes as — never stated for event triggers; inferred from the general rule, and proven by the same two tests, since a wrong invoker fails the DDL loudly. (c) The TOTP conditional update's "zero rows ⇒ reject" branch, which is a **READ COMMITTED** property; at REPEATABLE READ or SERIALIZABLE PostgreSQL raises `40001` instead and that error must be mapped to the same rejection. None of the three blocks the design; all three are recorded so they are not mistaken for documented facts.
- [ ] `pg_partman` presence in `postgres:17-alpine` — settled at implementation time by `SELECT 1 FROM pg_available_extensions WHERE name = 'pg_partman'`; D-C is correct either way, but the M1 `partitions.maintain` job needs either a db image carrying the extension or a Go periodic job that pre-creates partitions.
- [ ] The GitHub org segment of the module path (`github.com/tu-org/vecingest` in PRD §7.2) must be substituted with the real org at WU-0; changing it later rewrites every import.
- [ ] Expo SDK version to pin and freeze until M4 (§7.1) — an operator decision at WU-10, not a design one.
- [ ] Three deferrals recorded here so they are not rediscovered as gaps: `vecingest verify-chain` is an M7 gate item and `chains.anchor` is an M1+ periodic job, so M0 ships chain construction plus a tested `VerifyChain` domain function and neither the CLI nor TSA anchoring (D-O); `/v1/health/ready` registers only the `postgres` check until an R2 client exists at M2 (D-Q); Turnstile from the third failed attempt is M1, with `Count` already exposed for it (D-N). None of the three is an open question — each is a decision with a stated reason — but each is a place where the spec text is broader than M0's deliverable.
