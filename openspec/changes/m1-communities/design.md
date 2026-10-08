---
change: m1-communities
phase: design
project: vecingest
date: 2026-09-15
authority: PRD_go.md §3, §5.1, §5.2, §7.3 (lines 490–544), §7.4 (lines 807–827), §7.7 (line 943), §10, §10.1 (lines 1576–1583)
binds: >
  openspec/changes/m1-communities/proposal.md (Decisions 1–4, scope, risks),
  openspec/changes/m1-communities/research.md (C1–C9 and its "Gaps and cautions"),
  openspec/changes/m1-communities/exploration.md,
  openspec/changes/m0-foundation/design.md (D-B append-only baseline, D-D/D-N interface seams, D-H middleware chain)
verification: >
  Every huma fact in this document was read from the pinned module cache at
  /home/jorge/go/pkg/mod/github.com/danielgtaylor/huma/v2@v2.39.1/ and is cited file:line.
  Nothing here is taken from huma's `main` branch or from the documentation site.
  Research finding C8 is CORRECTED by this document, not merely restated.
size_note: >
  This artifact exceeds the generic 800-word design budget, following the m0-foundation
  precedent and its own size_note. The phase brief requires a verified framework-surface
  report with quotations, a four-rung enforcement design, a six-table schema with the
  tenant-scope column of every table and query, an invitation flow with a sequence diagram,
  and the binding `rules.design` interface disclosure. Compressing any of those below the
  budget would reproduce exactly the failure this change exists to correct: a document
  promising a guarantee the code does not deliver.
---

# Design: M1 — Communities, units, invitations, role portals

## Technical Approach

M1 adds six tenant tables, one authorization package, and roughly twenty operations to the
M0 `chi` + `huma` surface without changing its layering: `internal/http` (transport, huma
handlers) → `internal/domain/...` (policy) → `internal/db` (sqlc). The one genuinely new
architectural element is `api/internal/authz` plus its `scoped` registration wrapper, which
is where PRD §7.7's intent gets a mechanism that the source actually supports.

The four-rung ladder from proposal Decision 1 is built as: a typed registration wrapper that
is one-directionally compile-checked (rung 1), a fail-closed boot assertion over two
independent registration surfaces (rung 2), a generated permission matrix (rung 3), and a PR
checklist item (rung 4). Rung 2 is the backstop and is designed below in the most detail,
because it is the only rung that closes the case rung 1 provably cannot.

## Verified huma v2.39.1 surface

This section exists because research C8 assumed an iteration API and its own "Gaps and
cautions" recorded that the assumption was never checked. It has now been checked.

| # | Fact, as read from the pinned module | Source | Consequence for this design |
|---|---|---|---|
| V1 | `func Register[I, O any](api API, op Operation, handler func(context.Context, *I) (*O, error))` | `huma.go:779` | Research **C1 holds**. huma owns the call site and passes a plain `context.Context`. PRD §7.7's "a handler does not compile without it" is unachievable at the handler boundary, exactly as the proposal states. |
| V2 | **There is no `Operations()` method on `*OpenAPI`.** The only mutator is `func (o *OpenAPI) AddOperation(op *Operation)` | `openapi.go:1509` | Research C8's "enumerate `api.OpenAPI()`" must be implemented as a walk, not a call. **Recorded here so nobody tries `Operations()` again.** |
| V3 | `Paths map[string]*PathItem` on `OpenAPI`; `PathItem` carries eight `*Operation` fields (`Get` `openapi.go:1057`, `Put` `:1060`, `Post` `:1063`, `Delete` `:1066`, plus `Head`, `Options`, `Patch`, `Trace`) | `openapi.go:1463`, `:1042`–`:1080` | Enumeration walks `Paths` and those eight fields. huma's own duplicate-operation-ID check does precisely this at `openapi.go:1519-1528`; the assertion copies that loop rather than inventing one. |
| V4 | `OnAddOperation []AddOpFunc` with `type AddOpFunc func(oapi *OpenAPI, op *Operation)`; every callback is fired at the end of `AddOperation`; `huma.Register` always routes through `oapi.AddOperation(&op)` | `openapi.go:1502`, `:1437`, `:1558-1560`, `huma.go:817` | A registration-time hook the research never found. Usable as an eager trigger. Its own doc comment names the escape: *"You may bypass this by directly writing to the `Paths` map instead."* (`openapi.go:1500-1501`) — so it is a hook, not a sealed boundary. |
| V5 | `Metadata map[string]any` and `Middlewares Middlewares`, both exported, both `yaml:"-"`, both on the `*Operation` held in `Paths` | `openapi.go:896`, `:901` | `Metadata` is a marker channel that never reaches the published `openapi.yaml`. This answers research's gap "*whether `Operation.Middlewares` is reachable through any exported introspection hook*" — **it is reachable.** |
| V6 | `type Middlewares []func(ctx Context, next func(Context))`; `Register` composes `api.Middlewares().Handler(op.Middlewares.Handler(...))` | `chain.go:5`, `huma.go:881` | …but reachable is not usable. Go func values are not comparable, so no assertion can prove "this operation carries `RequireMembership`". Worse, **group middleware never lands in `op.Middlewares`**: it is applied from `api.Middlewares()`, so the repo's existing `authGroup.UseMiddleware(bearerAuthAndRateLimit(d))` (`api/internal/http/api/api.go:68`) is structurally invisible in the OpenAPI model. Middleware introspection is therefore rejected as an enforcement mechanism, with a reason rather than a shrug. |
| V7 | **`Hidden: true` produces a served route that `api.OpenAPI()` cannot see.** `huma.Register` documents the operation only via `} else if !op.Hidden { oapi.AddOperation(&op) }`; `Group.DocumentOperation` repeats the guard (`if op.Hidden { return }`); the OpenAPI marshaller states "Operations marked `Hidden` are never added to `Paths`". Routing is unconditional: `a.Handle(&op, ...)`, and the chi adapter does `a.router.MethodFunc(op.Method, op.Path, ...)` | `huma.go:813-818`, `group.go:106-108`, `openapi.go:1570`, `huma.go:881`, `adapters/humachi/humachi.go:167-170` | **This is a blind spot neither the research nor the proposal names.** Proposal Decision 1 calls `api.OpenAPI()` "huma's own single source of truth for registered routes". Corrected: it is the single source of truth for *documented* routes. One struct field defeats an assertion that reads only the OpenAPI model. Design response: check A3 below. |
| V8 | `PrefixModifier` shallow-copies the operation and rewrites `modified.Path = prefix + modified.Path`; `Group.DocumentOperation` and `groupAdapter.Handle` each run the modifier chain independently | `group.go:28-33`, `:101-112`, `:45-49` | A registry keyed by the path `scoped.Register` saw can hold the **pre-prefix** path while `Paths` holds the post-prefix one — a built-in drift source for the two-structure approach. A marker carried inside `Operation` cannot desynchronise this way, because the shallow copy carries the `Metadata` map header with it. |

Research's "Gaps and cautions" is now **partly closed**: the enumeration surface is answered
(V2, V3), `Middlewares` reachability is answered and then rejected on a second ground (V5,
V6), and a gap the research did not know it had is added (V7). The remaining caution stands
unchanged and is inherited as a risk: huma v2 is under active development, and a huma
**major** upgrade invalidates this seam. The "no prior art for a huma registration-time
wrapper" caution also stands — what follows is inference validated by the source, not
attested practice.

## Architecture Decisions

### D-1. Rung 1 — `authz.Membership` and the typed `scoped` constructors

| Option | Trade-off | Decision |
|---|---|---|
| Handler signature required by plain `huma.Register` | Foreclosed by V1 | rejected on evidence |
| Context lookup inside the handler body (`authz.FromContext(ctx)`) | Zero plumbing, but a handler that forgets the lookup compiles and serves | rejected — this is the exact drift §7.7 exists to prevent |
| **Wrapper with an unconstructible `Membership` parameter plus a tenant-declaring input constraint** | One generic wrapper per scope kind; handlers gain a third parameter | **chosen** |

```go
package authz

// Membership has no exported field and no exported constructor. Only this
// package's resolvers build one, and the zero value fails every accessor.
type Membership struct {
	userID uuid.UUID
	scope  Scope // {kind: community|office, id}
	role   Role  // admin | admin_staff | owner | tenant
	valid  bool
}

func (m Membership) UserID() uuid.UUID      // accessors only
func (m Membership) CommunityID() uuid.UUID // panics on the zero value — unreachable by construction

// Inputs declare their own tenant id. A community-scoped operation whose input
// cannot answer this is a COMPILE error, not a review finding.
type CommunityScoped interface{ ScopeCommunityID() uuid.UUID }
type OfficeScoped    interface{ ScopeOfficeID() uuid.UUID }
```

```go
package scoped

// Community registers a community-scoped operation. PI is the pointer-receiver
// constraint that forces *I to declare its tenant id.
func Community[I any, O any, PI interface {
	*I
	authz.CommunityScoped
}](api huma.API, op huma.Operation, roles []authz.Role,
	handler func(context.Context, PI, authz.Membership) (*O, error))

func Office[I any, O any, PI interface{ *I; authz.OfficeScoped }](...)

// Self covers operations scoped to the caller's own membership set and no path
// resource: GET /v1/communities, GET /v1/me. authz.Memberships is likewise
// unconstructible outside authz.
func Self[I any, O any](api huma.API, op huma.Operation,
	handler func(context.Context, *I, authz.Memberships) (*O, error))
```

This is a **refinement of the proposal's single `scoped.Register[I,O](api, op, scope, roles..., handler)`**, forced by the source: the wrapper must obtain the tenant id from the typed input (huma parses path params into `*I`), and a single function cannot express three different input constraints. The scope argument becomes the constructor name and the roles stay a parameter.

**What rung 1 guarantees, stated one-directionally and no further.** `func(context.Context, *I, authz.Membership) (*O, error)` is not assignable to `func(context.Context, *I) (*O, error)`, so a scoped handler **cannot** be passed to plain `huma.Register` — that is a compile error. A handler outside `authz` also cannot fabricate a `Membership`. The converse is **not** prevented: a newly written two-argument handler can still be registered on a scoped path through plain `huma.Register`. That residual case is precisely what rung 2 closes, and nothing in this design pretends otherwise.

### D-2. Rung 2 — registration-time fail-closed, marker-based, over two surfaces

| Option | Trade-off | Decision |
|---|---|---|
| Separate registry map written by `scoped.*`, compared against `api.OpenAPI()` at boot (the proposal's wording) | Two structures that can disagree; by V8 the registry key is the pre-prefix path while `Paths` holds the post-prefix one, so the comparison needs a path-normalisation rule that is itself a drift source | rejected |
| `op.Metadata[authz.MetadataKey]` stamped by `scoped.*`, asserted at boot by walking `Paths` (V3) | One structure: the marker rides on the same `Operation` huma stores and copies (V8), so prefixes cannot desynchronise it, and `yaml:"-"` (V5) keeps it out of the published `openapi.yaml` | **chosen** |
| `OnAddOperation` hook alone (V4) | Fails at the registration instant with the best possible error site, but its own doc admits the `Paths` bypass, and it never fires for a `Hidden` operation (V7) | **chosen as an additive second trigger, never as the only one** |
| `chi.Walk` as the authorization check | Rejected by research C7 and still rejected: humachi hides huma `Security` and per-operation middleware inside one opaque closure | rejected **for authorization**, adopted below for route *existence*, which is a different question C7 never ruled on |

`serve` calls one function after `api.New(...)` and before `ListenAndServe`, and a non-nil
error aborts startup (returned up to `main`; no `panic` outside `main`, per
`rules.apply.guidelines`):

```go
// api/internal/authz/assert.go
func AssertScopedRegistration(oapi *huma.OpenAPI, r chi.Router, allow Allowlist) error
```

Three checks, each with its own failure message naming the offending operation:

- **A1 — every scoped documented operation carries the marker.** Walk `Paths` × the eight
  method fields (V3). For any operation whose path matches a scoped pattern
  (`/v1/communities…`, `/v1/units…`, `/v1/invitations/{invitationId}`, `/v1/offices…`),
  `op.Metadata[authz.MetadataKey]` must be present, or boot fails.
- **A2 — no marker without a resolver.** Every marked operation's declared scope kind must
  have a registered resolver and a non-empty role set. This catches a marker stamped for a
  scope the resolver table cannot serve.
- **A3 — the chi route set must not exceed the documented set.** `chi.Walk` enumerates every
  route the server will actually serve. Its `(method, path)` set, after trimming chi's
  trailing `/*` mount suffix, minus the OpenAPI operation set, must be a subset of the
  allowlist. **This is the check that closes V7**: a `Hidden: true` operation is routed
  (`humachi.go:167-170`) but absent from `Paths`, so it surfaces here as an undocumented
  live route. It also catches any raw `http.Handler` mounted beside huma.

A3 uses `chi.Walk` at the only layer where it is trustworthy — *does this route exist* —
and never asks it what research C7 proved it cannot answer.

Residual holes, named rather than implied: a direct write into `oapi.Paths` (V4's documented
bypass) would forge a marker, and a handler can still be marked for the *wrong* community.
Both are greppable and reviewable; the second is caught only by rung 3.

### D-3. The allowlist is data with a reason, in one reviewed file

`api/internal/authz/allowlist.go` holds `var PublicOperations = []Entry{{Method, Path, Reason}}`.
A unit test asserts every entry has a non-empty `Reason` and that no entry's path begins with
`/v1/communities/`, `/v1/units/` or `/v1/offices/`. Its diff is a §9 checklist item
(proposal risk row). Recovery from a production false positive is an allowlist entry plus an
issue — never disabling the assertion.

### D-4. Membership resolution by route resource

One indexed resolver per scope kind, called by the `scoped.*` adapter before the handler runs:

| Route shape | Resolver | Query |
|---|---|---|
| `{communityId}` | community | `office_members` ∪ `unit_members` for that `community_id` |
| `{unitId}` | community via `units.community_id` | one join, then the community resolver's predicate |
| `{invitationId}` | community via `invitations.community_id` | same |
| `/v1/offices/me`, `{officeId}` | office | `office_members` by `(office_id, user_id)` |
| no path resource (`GET /v1/communities`, `GET /v1/me`) | `scoped.Self` | the caller's full membership set |

**Foreign resource → 404; member but insufficient role → 403.** §10.1 accepts either; 404 for
a resource in another tenant avoids confirming its existence. The distinction is asserted by
the permission matrix.

Admin scope follows PRD §3 exactly: an `admin` reaches every community whose `office_id`
matches one of their `office_members` rows. The resolver never reads a tenant from a header
or body (§7.7).

### D-5. Schema, and the tenant-scope column of every new table

Two goose migrations under `api/migrations/schema/`, both opening with `SET ROLE
vecingest_owner;` and closing with `RESET ROLE;` as `00001_auth_schema.sql` does, because M0
design D-B's fail-closed default-privilege baseline grants only `SELECT, INSERT` for objects
created by that exact role:

- `00007_m1_tenant_schema.sql` — `offices`, `office_members`, `communities`, `units`, `unit_members`
- `00008_invitations.sql` — `invitations`

**Every M1 table is mutable**, so each carries an explicit `GRANT UPDATE, DELETE ON <t> TO
app_rw;` (M0 D-B mechanism 1). **No M1 table is append-only**, none is registered in
`append_only_relations`, and no M1 code path assumes `UPDATE`/`DELETE` on an append-only
relation. M1's `audit_log` writes remain `INSERT`-only, which is what the hash-chained
append-only guarantee and the `ddl_command_end` event trigger already permit.

| Table | Tenant-scope column | Keys and indexes |
|---|---|---|
| `offices` | **`id` is itself the tenant root** — `office_id` elsewhere is an FK to it | PK `id`; `unique(cif)` |
| `office_members` | `office_id` | `unique(office_id, user_id)`; plus `office_members_user_id_idx` — a **documented exception** to §7.3's "composite indexes start with the tenant column", because `GET /v1/me` looks up by user, not by office |
| `communities` | `office_id` (tenant owner); `id` is the community tenant root | PK `id`; `communities_office_id_idx`; `parent_community_id` FK nullable |
| `units` | `community_id` | PK `id`; `unique(community_id, block, floor, door)`; `units_community_id_idx` |
| `unit_members` | **`community_id`, denormalised NOT NULL FK** | §7.3 mandates a tenant column on every business table *even when derivable by a relation*; deriving it through `units` on every query is exactly what `lint-scope` cannot verify. `unique(unit_id, user_id, role)`; `unit_members_user_id_idx` (named in §7.3's "índices imprescindibles"); partial unique `unit_members_one_president_idx ON unit_members (community_id) WHERE board_role = 'president' AND board_to IS NULL AND deleted_at IS NULL` |
| `invitations` | `community_id` | PK `id`; `unique(token_hash)`; `unique(short_code_hash)`; `invitations_community_id_status_idx` |

Columns follow §7.3 verbatim, minus the two deferrals proposal Decision 2 records
(`is_payer`, `iban_encrypted` → M5) and plus the one deviation Decision 3 records
(`invitations.status`).

**Tenant scope of every new query.** Every sqlc query in `api/internal/db/queries/` against
these six tables takes its tenant column as a bound parameter and names it in the `WHERE`
clause — `community_id` for `units`, `unit_members`, `invitations`; `office_id` for
`office_members` and `communities`; `id` for `offices`. Resolver queries are the only ones
keyed on `(tenant, user_id)` pairs, and they are the queries whose whole purpose is deciding
tenancy.

`make lint-scope` changes twice (`api/cmd/lintscope/main.go`): `tenantColumns` is already the
right three names and does not change, but the blanket **`audit_log` table exception is
retired** — M1 gives `audit_log` real per-community rows, which is the condition its own
exception note names. It is replaced by a narrower **query-level** exception map keyed
`file:queryName`, holding exactly `GetAuditLogHead` and `ListAuditLogRange` with their
reason (both intentionally read the whole hash chain across tenants). A new tenant-blind
audit query then fails CI instead of inheriting a table-wide pass.

### D-6. Invitations — status, hashing, preview, and an honestly-scoped lockout

`status text NOT NULL CHECK (status IN ('pending','accepted','revoked','blocked'))`,
maintained transactionally. `expires_at` stays authoritative for expiry: `expired` is derived
at read time and never stored, and the daily `invitations.expire` job (§7.4 job table) sweeps.
This is the recorded §7.3 deviation.

Single use is enforced by the write, not by a prior read:

```sql
UPDATE invitations SET status = 'accepted', accepted_at = now()
WHERE id = $1 AND community_id = $2 AND status = 'pending' AND expires_at > now()
RETURNING id;
```

Zero rows returned ⇒ `409`/`404`; the whole acceptance runs in one transaction with the
`users` and `unit_members` writes.

Tokens: `crypto/rand`, SHA-256 `token_hash` and `short_code_hash` (both unique), 8 characters
from an alphabet excluding `O/0/I/1`, 14-day expiry. The plaintext short code is returned
exactly once, at creation.

`POST /v1/invitations/preview` takes `{token}` **or** `{short_code}` (exactly one) in the
body — never a query string, because a `GET ...?token=` writes a credential into access logs,
proxies and `Referer`. It returns community, unit, role and `expires_at`, never the invited
email. Unresolvable, expired, revoked and blocked all return the **same** generic 404 body, so
the endpoint is not an existence oracle. It is a separate route rather than a `dry_run` flag
on accept, because the two have different lockout semantics and sharing them makes the
attempt counter ambiguous.

**Lockout, at the layer that actually closes the hole.** A wrong short code matches no
invitation row, so a per-invitation counter cannot stop enumeration. §10.1's "bloqueo tras 10
códigos fallidos" is implemented as an IP + device scoped counter over `preview` and `accept`,
through the existing `AttemptCounter` seam (`internal/domain/auth/lockout.AttemptCounter`,
`Fail`/`Count`/`Reset`): key `invite:{ip}:{deviceHash}`, where `deviceHash` is a SHA-256 of
the mandatory `X-Platform` + `X-App-Version` headers (§7.7). Threshold 10 per 15 minutes →
`429` with `Retry-After`. The device leg is **client-supplied and spoofable**; it exists only
to narrow collateral damage behind a shared NAT, and the IP leg is the load-bearing one.
`invitations.failed_attempts` is still incremented, but only when the code resolved to a real
invitation, and it is gate evidence rather than the mechanism.

### D-7. Turnstile, 2FA, and CSV import

- `CaptchaVerifier` is a new interface in `internal/domain/…` with a Turnstile HTTP
  implementation and an `AlwaysPass` test double, wired on login-after-third-failure and
  forgot-password. `TURNSTILE_SECRET` already exists in `internal/config` (`config.go:67`).
- Non-superadmin TOTP enrol/verify/recovery endpoints wire M0's existing
  `internal/domain/auth/mfa`; no new domain logic. `admin`/`admin_staff` membership resolution
  returns `403` with a distinguishable code when `user_mfa.enabled_at IS NULL`, so an admin
  without TOTP holds no admin-scope access at all.
- CSV import hardening is scoped to `POST /v1/communities/{communityId}/units/import`:
  cells beginning `=`, `+`, `-`, `@` are prefixed with `'` on template and export; uploaded
  filenames are never used as paths (the file is streamed, never written under a
  client-supplied name); `?dry_run=true` validates row by row and writes nothing.

## Data Flow

### Boot-time enforcement

```
main → serve → api.New() ──► humachi.New(chi, cfg)
                               │
        scoped.Community/Office/Self ──► stamps op.Metadata[authz.scope]
                               │              └─► huma.Register (huma.go:779)
                               │                    ├─► a.Handle → chi.MethodFunc  (always)
                               │                    └─► oapi.AddOperation          (only if !Hidden)
                               ▼
        authz.AssertScopedRegistration(oapi, chiRouter, allowlist)
              A1 Paths walk: scoped path ⇒ marker present
              A2 marker ⇒ resolver + roles exist
              A3 chi route set ∖ OpenAPI set ⊆ allowlist        ← catches Hidden
                               │
                  error ──► process exits, never serves
```

### Invitation acceptance (required sequence diagram — multi-actor, multi-step)

```
Admin        API (scoped)        DB                    Queue/Mail      Neighbour        App
  │               │               │                        │               │             │
  │ POST /v1/communities/{id}/invitations                   │               │             │
  ├──────────────►│ resolve membership (admin@office)       │               │             │
  │               ├──────────────►│ INSERT invitations      │               │             │
  │               │               │ (status=pending,        │               │             │
  │               │               │  token_hash, code_hash) │               │             │
  │               │               │ river.InsertTx(mail) ───┤ same tx       │             │
  │               │◄──────────────┤                         │               │             │
  │◄──────────────┤ 201 {short_code}  ← plaintext, once     │               │             │
  │  (paper/voice)│               │                         ├──────────────►│ email       │
  │═══════════════╪═══════════════╪═════════════════════════╪══════════════►│ short code  │
  │               │               │                         │               │             │
  │               │      POST /v1/invitations/preview {short_code}           │◄────────────┤
  │               │◄────────────────────────────────────────────────────────┼─────────────┤
  │               ├─ AttemptCounter.Count(invite:{ip}:{dev}) ──► ≥10 ⇒ 429   │             │
  │               ├──────────────►│ SELECT by short_code_hash                │             │
  │               │               │ miss/expired/revoked ⇒ same generic 404  │             │
  │               │◄──────────────┤ community, unit, role, expires_at        │             │
  │               ├────────────────────────────────────────────────────────►│ confirm UI  │
  │               │               │                         │               │             │
  │               │      POST /v1/auth/accept-invitation {short_code, name, password, consent}
  │               │◄────────────────────────────────────────────────────────┼─────────────┤
  │               ├──────────────►│ BEGIN                                    │             │
  │               │               │  SELECT ... FOR UPDATE                   │             │
  │               │               │  UPDATE invitations SET status='accepted'│             │
  │               │               │    WHERE status='pending' AND not expired│             │
  │               │               │    RETURNING id   ── 0 rows ⇒ rollback,409             │
  │               │               │  INSERT users (or link existing)         │             │
  │               │               │  INSERT unit_members (community_id, …)   │             │
  │               │               │  INSERT audit_log (before/after)         │             │
  │               │               │ COMMIT                                   │             │
  │               ├────────────────────────────────────────────────────────►│ 201 session │
  │               │      GET /v1/me → memberships[] ─────────────────────────┼────────────►│ portal
```

Responses never reveal whether the email already exists (§5.1): the account-creation and
account-linking branches return the same shape and the same status.

## Interfaces / Contracts — phase-A vs phase-B disclosure (binding `rules.design`)

| Interface | Touched by M1 | Phase A (M1 ships) | Phase B | M1 callers |
|---|---|---|---|---|
| `Limiter` | **yes** | `httprate` in-process (`internal/platform/limiter`) | `httprate-redis` / Valkey | per-IP budget on `preview`, `accept`, `resend`; Turnstile-guarded public forms |
| `AttemptCounter` (M0 D-N seam) | **yes** | `internal/platform/attempts` in-process | Valkey | invitation IP+device lockout |
| `Queue` | **yes — first producers in the project** | River on Postgres, `river.InsertTx` in the domain transaction | River → NATS JetStream | invitation email job; `invitations.expire` daily sweeper |
| `Mailer` | **yes** | `AsyncMailer` (SMTP) / `LogMailer` | unchanged | invitation template beside `RenderPasswordReset` |
| `CaptchaVerifier` (new, 6th seam) | **yes** | Turnstile HTTP client | unchanged | login-after-3rd-failure, forgot-password |
| `Cache` | **no** | in-process TTL + `LISTEN/NOTIFY` | Valkey | unchanged. **Membership lookups are deliberately not cached**: a cached membership is a stale-authorization bug, and each resolver is one indexed lookup |
| `Search` | **no** | — | — | no M1 entity is searchable |

**Confirmation that phase A is not hardcoded into callers.** No M1 handler or domain service
imports `httprate`, `river`, or an HTTP client for Turnstile. Each depends on the interface
through a field on `handlers.Deps`, and the concrete types are named in exactly one place —
`buildServeDeps` in `api/cmd/vecingest/serve.go:178-220`, the same composition root M0
established. M1 adds `Limiter`, `Attempts`, `Captcha` and `Queue` fields there and nowhere
else; the phase-B swap stays a change to that function.

## File Changes

| File | Action | Description |
|---|---|---|
| `api/migrations/schema/00007_m1_tenant_schema.sql` | Create | Five tenant tables, grants, indexes, partial unique president index; `down` drops in FK order |
| `api/migrations/schema/00008_invitations.sql` | Create | `invitations` with explicit `status`, hashed token/code, grants, indexes |
| `api/internal/authz/authz.go` | Create | `Membership`, `Memberships`, `Scope`, `Role`, `CommunityScoped`/`OfficeScoped`, `MetadataKey` |
| `api/internal/authz/resolve.go` | Create | Four resolvers (D-4), 403/404 policy, the admin-without-TOTP branch |
| `api/internal/authz/scoped/register.go` | Create | `Community`, `Office`, `Self` — marker stamping and the huma adapter |
| `api/internal/authz/assert.go` | Create | `AssertScopedRegistration` (A1/A2/A3); `OnAddOperation` eager trigger |
| `api/internal/authz/allowlist.go` | Create | Reviewed public-route allowlist, one reason per entry |
| `api/internal/authz/testdata/negative_register/` | Create | Non-compiling fixture proving rung 1's one-directional guarantee |
| `api/internal/http/api/api.go` | Modify | Register the new groups; call the boot assertion from `New`'s caller |
| `api/cmd/vecingest/serve.go` | Modify | Fail startup on assertion error; wire `Limiter`/`Attempts`/`Captcha`/`Queue` |
| `api/internal/http/handlers/{offices,communities,units,unit_members,invitations,mfa}.go` | Create | ~20 operations from §7.4 lines 807-827 |
| `api/internal/http/handlers/me.go`, `dto/me.go` | Modify | `memberships[]` with a `scope` discriminator |
| `api/internal/db/queries/*.sql` | Create | sqlc queries, each with its tenant parameter |
| `api/cmd/lintscope/main.go` | Modify | Retire the `audit_log` table exception; add the query-level exception map |
| `api/internal/mail/templates.go` | Modify | `RenderInvitation`, following `RenderPasswordReset` |
| `api/internal/platform/captcha/turnstile.go` | Create | `CaptchaVerifier` implementation + `AlwaysPass` double |
| `app/src/screens/PortalScreen.tsx` | Modify | Populated branch, context selector, enable `portal-invitation-link` |
| `docs/security/gates/M1.md` | Create | §10.1 evidence, Checkpoint A/B split |
| `PRD_go.md` | Modify | §7.7 enforcement wording (proposal's Spanish replacement); §7.3 `invitations.status` |
| `openspec/config.yaml` | Modify | Permission-matrix requirement; tenant-scope disclosure rule |

## Testing Strategy

| Layer | What to test | Approach |
|---|---|---|
| Compile | A scoped handler cannot be passed to plain `huma.Register` | `go build` over `testdata/negative_register/`, asserting a non-zero exit and the expected type-mismatch text; the test package itself stays `go vet`-clean |
| Unit | A1/A2/A3 each fail closed | Build an in-memory `humachi` API, register an operation via plain `huma.Register` on a scoped path (A1), a marker with no resolver (A2), and a `Hidden: true` operation (A3); assert `AssertScopedRegistration` returns an error naming the operation |
| Unit | Marker survives groups | Register through `huma.NewGroup` and assert the marker is still readable from `Paths` (V8) |
| Unit | Allowlist hygiene | Every entry has a reason; no entry under a scoped prefix |
| Unit | Invitation code generation, lockout counter, CSV formula escaping, coefficient sum warning | Table tests; injected `Clock`, fake `AttemptCounter` |
| Integration | Permission matrix, endpoint × role × own/foreign → 403/404, **100 % route coverage** | Generated from `api/openapi/openapi.yaml` at test time, never hand-listed; an operation neither exercised nor allowlisted fails the test |
| Integration | Tenant isolation, invitation expiry, single use, transactional accept, `serve` refusing to boot | Testcontainers Postgres (`make test-e2e`) |
| Static | `make lint-scope` green with real tenant tables and no `audit_log` table exception | CI |
| App | Populated portal branch, invitation entry | React Native Testing Library; Maestro's "accept invitation" flow is pre-release, not per PR |

Strict TDD is enabled: every behavioural task states its RED test first.

## Threat Matrix

The matrix in `references/threat-matrix.md` covers shell, git, commit/push and PR-automation
boundaries. M1 changes HTTP route registration and adds a file-upload parser; it adds no
shell command, subprocess, VCS or PR automation, so every row is explicitly `N/A` rather than
stretched.

| Boundary | Applicability | Reason |
|---|---|---|
| Documentation-like paths | **N/A** | M1 classifies no file as executable. The CSV import parses rows; it never executes, interprets, or dispatches on file type |
| Git repository selection | **N/A** | No M1 code path invokes git |
| Commit state | **N/A** | No VCS automation |
| Push state | **N/A** | No VCS automation |
| PR commands | **N/A** | No PR automation |

Two adversarial boundaries M1 *does* have are covered above rather than manufactured here,
each with its RED test already listed: CSV **formula injection** and **path traversal in
uploaded filenames** (D-7, §10.1 line 1583), and **route registration bypass** (D-2 A1/A2/A3).

## Migration / Rollout

Goose expand/contract, both migrations additive, each with a `down`. Until the first
invitation is accepted, `goose down` to the M0 revision is lossless; after it, expand/contract
only, never `DROP`/`RENAME` in the release that stops using a shape. `memberships` on
`GET /v1/me` is additive and an older client ignores it. `PortalScreen` degrades to its
already-honest empty state on an M0 app build. Deployment rollback re-points Portainer's
`TAG` at the previous GHCR image.

Checkpoint A (code, tests, CI) never waits on Checkpoint B (SPF/DKIM/DMARC, real SMTP), per
the M0 D2 precedent; the short-code path makes the acceptance criterion reachable with no
mail leaving the server.

## Open Questions

- [ ] `PATCH /v1/units/{unitId}/members/{memberId}` is listed in §7.4 as "cambiar rol **o cargo de junta**", but proposal Decision 2 defers `board_role` assignment. Design assumes the M1 DTO exposes `role` only; spec must confirm.
- [ ] `X-Platform` + `X-App-Version` is a weak device signal for the lockout's second leg. Threshold and key shape are to be measured before tightening (proposal risk row), not guessed now.
- [ ] The query-level `lintscope` exception map is a new mechanism; its key format (`file:queryName`) should be reviewed with the first two entries rather than generalised ahead of need.
