---
change: m1-communities
phase: propose
project: vecingest
date: 2026-09-15
authority: PRD_go.md §3, §5.1, §5.2, §7.3, §7.4, §7.7, §10, §10.1
binds: openspec/changes/m1-communities/exploration.md (Engram sdd/m1-communities/explore, obs 6048)
research: openspec/changes/m1-communities/research.md (Engram sdd/m1-communities/research, obs 6065) — written after this proposal, from the same handoff it quotes
---

# Proposal: M1 — Communities, units, invitations, role portals

## Intent

M0 shipped authentication against a six-table schema with no tenant concept at all. M1 (PRD §10) introduces the first multi-tenant data — `offices`, `communities`, `units`, memberships and `invitations` — and with it the first opportunity to get tenant isolation wrong. Its functional criterion is one sentence: *"Un admin crea una comunidad e invita a un vecino que entra en su portal"*. Its security criterion is the §10.1 "M1 — Aislamiento" gate: tenant isolation proven by tests, 2FA for admins, anti-abuse on public forms, authenticated email.

M1 is where authorization becomes load-bearing. Every later milestone (incidents, documents, receipts, votes) reads through the mechanism this change builds. Getting the enforcement shape right now costs one wrapper type; retrofitting it across M2–M8 costs every handler.

## Decision 1 — What tenant-isolation guarantee M1 actually ships

**PRD §7.7 promises something Go cannot deliver as written**, and this is verified fact, not preference:

- huma **v2.39.1** (pinned in `api/go.mod`) declares `func Register[I, O any](api API, op Operation, handler func(context.Context, *I) (*O, error))`. huma owns the call site and always passes a plain `context.Context`. No handler signature can be made to *require* an authorized-identity type at that boundary.
- huma's own documented authorization middleware **fails open**: it reads `ctx.Operation().Security` and calls `next(ctx)` unchecked when that is empty. Unusable as the guarantee.
- `chi.Walk` cannot see huma-level `Security`: humachi registers each operation as one opaque closure. A chi-walking audit moves the drift hole, it does not close it.
- Render's published generic-token pattern *is* a genuine compile-time guarantee — but at the service/repository-method boundary, not at the HTTP handler.

So M1 ships a **layered guarantee**, and states plainly where each rung sits:

| Rung | Mechanism | What it actually guarantees |
|---|---|---|
| **Compile-time** (one direction) | `scoped.Register[I,O](api, op, scope, roles..., func(ctx, *I, authz.Membership) (*O, error))`. `authz.Membership` has no exported constructor and no zero-value that passes validation | A scoped handler **cannot** be handed to plain `huma.Register` (signature mismatch), and cannot fabricate its own membership |
| **Registration-time, fail-closed** | `scoped.Register` records every operation ID it registers; `serve` compares `api.OpenAPI()`'s operation set against that registry plus an explicit public-route allowlist and **refuses to start** on any mismatch | Closes the residual case the compile check cannot: a *new* two-argument handler registered on a scoped path through plain `huma.Register`. `api.OpenAPI()` is huma's own single source of truth for registered routes, so there is no second list to drift against |
| **Test-time** | Permission matrix, endpoint × role × own/foreign → 403/404, 100 % route coverage (§10.1); `make lint-scope` over `.sql` | The only layer that catches a route scoped to the *wrong* community. Mandatory regardless of the rungs above |
| **Review-time** | §9 DoD point 10 PR checklist | Whatever all of the above misses |

**The PRD must be corrected.** §7.7's *"un handler sin ella no compila para rutas con ámbito (interfaz obligatoria)"* is not achievable and must be rewritten to: *"los handlers con ámbito se registran exclusivamente mediante `scoped.Register`, que exige `Membership` como argumento explícito; el arranque de `serve` falla si alguna operación de `api.OpenAPI()` bajo una ruta con ámbito no pasó por ese registro"*. This project has already been burned by a spec promising what the code does not do — the `otp_challenges` channel check reached production today. Writing the PRD correction is an M1 deliverable, not a footnote.

## Decision 2 — Role and membership model

The minimal shape that serves M1's screens, no M8 pre-building:

- `offices`, `office_members (role: admin | admin_staff)` — required because `admin` scope is "communities whose `office_id` matches one of my `office_members`" (§3).
- `communities`, `units`, `unit_members (role: owner | tenant)` — the acceptance criterion itself.
- `unit_members` is created with `tenure`, `board_role`, `board_from`, `board_to`, `valid_from`, `valid_to`, `notification_address`, `electronic_notifications_consent_at`, `consent_text_version` — cheap nullable columns, verbatim from §7.3, no later `ALTER`. **`is_payer` and `iban_encrypted` are deferred to M5**, where the versioned-key gate that makes an encrypted column safe actually exists; creating an encryption column with no key management is a liability, not a saving.
- **`board_role` is created but never assigned in M1**: no endpoint, no UI. Its assignment screens depend on M5/M7 data.
- **No `companies` / `company_members`**: no M1 screen or criterion touches them (see Decision 4).
- `GET /v1/me` returns a heterogeneous `memberships` array with a `scope` discriminator (`office` | `community`). Company memberships join the same array in M8 with no breaking change — so M1 ships no dead `companies` field.
- **First-admin bootstrap**: `superadmin` creates the office and its first `admin` user directly (§3: office creation is a commercial relationship, not open registration); that user sets a password through M0's existing forgot-password flow. No new invitation type. `POST /v1/offices/me/members` adds an *existing* account as `admin_staff`; full office-staff invitation is deferred.

## Decision 3 — Invitation flow

**One contract**, reconciling the three incompatible provisional shapes in the inventory:

| Route | Purpose |
|---|---|
| `POST /v1/communities/:id/invitations` | Create. Returns the 8-char short code **once**; only hashes are stored. Sends email when `email` is present |
| `GET /v1/communities/:id/invitations` | List with status |
| `POST /v1/invitations/:id/resend` | §5.1's "puede reenviarse"; increments `sent_count`, rate-limited |
| `DELETE /v1/invitations/:id` | Revoke → `status = revoked` |
| `POST /v1/invitations/preview` | Body `{token}` **or** `{short_code}`. Returns community / unit / role for the confirm screen. **POST, not GET** — a `GET ...?token=` puts a credential in access logs, proxies and `Referer`; the PRD stores these hashed precisely because they are credentials. This rejects `GET /v1/invitations/resolve?token=` (`app-movil-3.md`) and renames `POST /v1/invitations/validate` (`app-movil-1.md`) |
| `POST /v1/auth/accept-invitation` | §7.4's named route, `{token}` or `{short_code}` + `{name, password, phone?, consent}`. No account → create `user` + `unit_member` in one transaction; existing account → link the membership, never duplicate. Responses never reveal whether the email exists (§5.1) |

Preview is **not** a `dry_run` flag on accept: the two have different rate-limit and lockout semantics, and conflating them makes the attempt counter ambiguous.

- **`invitations.status` becomes an explicit column** (`pending | accepted | revoked | blocked`), maintained transactionally — consistent with `incidents.status`, `meetings.status`, `announcements.status`, and the only way to represent `blocked`, which no timestamp derives. `expires_at` stays authoritative for expiry: `expired` is derived at read time and swept by a periodic job. Recorded as a deliberate §7.3 deviation to write back into the PRD.
- **Lockout, read honestly**: a per-invitation `failed_attempts` counter cannot stop short-code enumeration, because a wrong guess matches no invitation row. §10.1's "bloqueo tras 10 códigos fallidos" is therefore implemented as an **IP + device scoped attempt counter** on `preview` and `accept` (behind the existing `Limiter` interface), with `invitations.failed_attempts` retained for attempts against a resolved invitation. Both are gate evidence; only the first actually closes the hole.
- 14-day expiry, single use, SHA-256 `token_hash` / `short_code_hash` (both unique), 8 characters from an unambiguous alphabet (no `O/0/I/1`) via `crypto/rand`.
- **M1 does not depend on production SMTP.** The short-code path is a first-class, PRD-mandated alternative ("para entregar en papel"), so the acceptance criterion is satisfiable without a single mail leaving the server. The invitation template and `AsyncMailer` are tested against the existing `LogMailer`/fake-SMTP seam. The "correo autenticado" gate item (SPF/DKIM/DMARC) is **infra evidence, Checkpoint B**, following M0's D2 split — it never blocks Checkpoint A work.

## Decision 4 — Turnstile vs. company registration

**M1 ships Turnstile on the public forms that exist at M1** — login after the third failure and forgot-password (§5.1) — behind a `CaptchaVerifier` interface, plus per-IP limits. **M1 does not ship `POST /v1/auth/register-company` or its form.**

Rationale: company self-registration has no M1 screen, no M1 acceptance criterion, and writes to a `companies` table with a `pending → verified` superadmin workflow that nothing in M1 reads. Shipping a public, unscreened write path purely to tick a checklist line is worse security, not better. §10.1's item is therefore read as *"Turnstile is installed on every public form present at this milestone; the company-registration and contact forms inherit it when they ship"*, recorded as a scoped item in `docs/security/gates/M1.md` with the deferral named, and added as a **blocking precondition** on the milestone that introduces `register-company`.

## Scope

### In Scope

- Migrations + sqlc: `offices`, `office_members`, `communities`, `units`, `unit_members`, `invitations`; `lintscope` `tenantColumns` extended and the M0 `audit_log` exception retired.
- `authz` package: `Membership`, membership resolution by route resource, `scoped.Register` wrapper, operation registry, fail-closed boot assertion in `serve`.
- Endpoints: offices (superadmin create, `GET /v1/offices/me`, members), communities (`GET`/`POST`/`GET :id`/`PATCH :id`), units (`GET`/`POST`/`PATCH`, members list/patch/delete), CSV import (`template`, `import?dry_run=`), invitations (the six routes above), `GET /v1/me` with `memberships`.
- CSV import hardening scoped to that endpoint only: formula escaping (`= + - @`) on template/export, path-traversal-safe filenames, row-by-row validation before write.
- Participation-coefficient sum check (100 ± 0.01) as a warning, never a block (§5.2).
- Non-superadmin TOTP enrol/verify/recovery HTTP endpoints wiring M0's existing `internal/domain/auth/mfa`; 2FA mandatory for `admin`/`admin_staff` — without it, no admin-scope access.
- Turnstile verifier behind an interface, on login and forgot-password.
- Permission-matrix test generated from `openapi.yaml`; `docs/security/gates/M1.md`; PRD §7.7 and §7.3 corrections.
- App: `PortalScreen`'s populated branch, context selector, invitation-code entry (`portal-invitation-link`) wired to preview + accept.

### Out of Scope

- The "Comunidad, resumen" aggregating dashboard (reserve-fund compliance, quorum, balance — M5/M7 data); M1 builds the ficha's own fields only.
- `board_role` assignment endpoints and the "Junta directiva" tab.
- `companies`, `company_members`, `office_providers`, `POST /v1/auth/register-company`, the public contact form.
- `is_payer` / `iban_encrypted` (M5), `unit_transfers`, `push_tokens` (M2), sub-community aggregated voting.
- Office-staff invitation flow for users with no account.
- Incidents, announcements, documents, directory (M2/M3).
- Real SMTP credentials and DNS records — Checkpoint B evidence, not code.

## Capabilities

M0 is **not archived**, so `openspec/specs/` is empty and the two modified capabilities' current text lives in `openspec/changes/m0-foundation/specs/`. The spec phase must delta against that location.

### New Capabilities

- `authz-membership`: `Membership`, scope resolution, `scoped.Register`, fail-closed boot assertion, permission-matrix contract.
- `office-management`: offices, office members, superadmin office creation, first-admin bootstrap.
- `community-management`: communities CRUD scoped by membership, legal fields, `parent_community_id`.
- `unit-management`: units, unit members, roles, coefficient validation, consent capture.
- `unit-csv-import`: template, dry-run validation, formula-injection and path-traversal hardening.
- `invitations`: create/list/resend/revoke/preview/accept, explicit status, expiry, single use, hashed storage, enumeration lockout.
- `public-form-protection`: `CaptchaVerifier` (Turnstile) + per-IP limits on existing public forms.
- `app-portal-memberships`: populated portal, context selector, invitation-code entry.

### Modified Capabilities

- `user-profile`: `GET /v1/me` gains `memberships` (M0 assumption 2 completes here).
- `auth-mfa-totp`: TOTP enrolment/verification exposed over HTTP for non-superadmins and made mandatory for `admin`/`admin_staff`.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `api/migrations/schema/` | New | Six tenant tables + indexes, partial unique index for one sitting president per community |
| `api/internal/authz/` | New | `Membership`, resolution, `scoped.Register`, operation registry |
| `api/internal/http/handlers/` | New/Modified | Community/unit/office/invitation/MFA handlers; `me.go` + `dto/me.go` gain memberships |
| `api/internal/http/api/api.go` | Modified | Boot assertion comparing `api.OpenAPI()` against the scoped registry |
| `api/cmd/lintscope/main.go` | Modified | Real tenant columns; `audit_log` exception retired |
| `api/internal/mail/templates.go` | New | Invitation template, following `RenderPasswordReset` |
| `app/src/screens/PortalScreen.tsx` | Modified | Populated branch; invitation entry enabled |
| `docs/security/gates/M1.md` | New | §10.1 evidence, Checkpoint A/B split |
| `PRD_go.md` | Modified | §7.7 enforcement wording; §7.3 `invitations.status` |
| `openspec/config.yaml` | Modified | Permission-matrix requirement; tenant-scope disclosure rule |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| The research phase persisted nothing — `sdd-research` has no write tool and no `mem_*` tools, so its findings existed only in the handoff | Resolved | Transcribed to `research.md` and Engram `sdd/m1-communities/research` (obs 6065) after this proposal was written. Decision 1 still restates every finding with its source, and `sdd-design` must re-verify the huma v2.39.1 signature against the module cache before implementing the wrapper |
| The boot assertion's public-route allowlist becomes a silent escape hatch | Med | Allowlist is one reviewed file, each entry carries a reason, and its diff is a §9 checklist item |
| `scoped.Register`'s generic plumbing fights huma's `Register` generics | Med | `sdd-design` prototypes the wrapper against v2.39.1 before any handler is written |
| Permission-matrix test drifts from routes | Med | Generated from `openapi.yaml`, not hand-listed; 100 % route coverage asserted |
| IP/device lockout blocks a legitimate building sharing one NAT address | Med | Counter scoped to IP + device, generous threshold, admin can resend; measured before tightening |
| Explicit `invitations.status` diverges from `expires_at` | Low | `expired` derived at read time, never stored as `pending` past expiry; sweeper job + test |
| `is_payer`/`iban_encrypted` deferral forces an M5 `ALTER` | Low | Nullable additive columns; expand/contract makes this free |
| M1 gate cannot fully close without DNS/SMTP | High | Checkpoint B split (M0 D2 precedent); Checkpoint A never waits |

## Rollback Plan

Required by `rules.proposal` — this change touches personal data and permissions.

- **Database**: every migration ships a `down`. Production currently holds M0 auth data only; until the first real community exists, `goose down` to the M0 revision is lossless. **Point of no return**: the first accepted invitation — after it, expand/contract only (§9), never `DROP`/`RENAME` in the release that stops using a shape.
- **API**: `memberships` on `GET /v1/me` is additive; an older client ignores it.
- **App**: `PortalScreen` already renders an honest empty state, so an app reverted to the M0 build degrades to "no perteneces a ninguna comunidad" rather than breaking.
- **Deployment**: Portainer pins `TAG`; rollback re-points `TAG` at the previous GHCR image.
- **Enforcement**: if the boot assertion produces a false positive in production, the recovery is a reviewed allowlist entry plus an issue — never disabling the assertion.

## Dependencies

- M0 merged and deployed (auth, `Limiter`, `AsyncMailer`, `make gen`, Testcontainers CI).
- huma v2.39.1 as pinned; a huma major upgrade invalidates Decision 1's registration seam.
- Cloudflare Turnstile site/secret keys (test keys suffice for Checkpoint A).
- DNS control for SPF/DKIM/DMARC and a real SMTP provider — **Checkpoint B only**.
- Stitch M1 screens in `docs/funcionalidad/consola-web.md` and `app-movil-*.md` as the UI contract.

## Success Criteria

- [ ] An admin creates a community, adds a unit, invites a neighbour, and that neighbour accepts by short code and sees the community in their portal — end to end against real Postgres.
- [ ] `serve` refuses to start when a scoped operation is registered without `scoped.Register`; a test proves it.
- [ ] A scoped handler cannot be compiled against plain `huma.Register`; a `go vet`-clean negative compilation test proves it.
- [ ] Permission-matrix test green at 100 % route coverage; foreign resource → 403/404.
- [ ] `make lint-scope` green with real tenant tables and no `audit_log` exception.
- [ ] Invitation expiry, single use and enumeration lockout proven by test.
- [ ] `admin`/`admin_staff` cannot reach the admin portal without TOTP.
- [ ] CSV import rejects formula injection and path traversal; `dry_run` reports errors row by row without writing.
- [ ] `PRD_go.md` §7.7 and §7.3 corrected, and the correction referenced in `docs/security/gates/M1.md`.
- [ ] Every Checkpoint A item in `docs/security/gates/M1.md` green with archived evidence; Checkpoint B items archived once DNS and SMTP exist.
