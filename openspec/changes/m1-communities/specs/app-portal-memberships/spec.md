# App Portal Memberships Specification

## Purpose

Turns `PortalScreen.tsx`'s already-built empty state into "portales vacíos
por rol": populated branch, context selector, invitation-code entry, per
`docs/funcionalidad/app-movil-2.md` ("Elegir dónde entrar") and
`consola-web.md`'s M1 section. The only new screen is the invitation-code
flow (Stitch "Código de invitación" / "Invitación reconocida"), built on
the existing sessionless invitation endpoints; `GET /v1/me`'s
`memberships` array is the only new wire.

## Requirements

### Requirement: Portal Renders Real Membership Rows

When `GET /v1/me` returns a non-empty `memberships` array, `PortalScreen`
MUST render one selectable row per membership, showing its
scope-appropriate label (office or community name, or company name) and
role, replacing the hardcoded empty state.

#### Scenario: User with one community membership sees one row

- GIVEN a user whose `memberships` array has one `scope: community` entry
- WHEN `PortalScreen` loads
- THEN it renders exactly one selectable row for that membership

#### Scenario: User with zero memberships still sees the honest empty state

- GIVEN a user whose `memberships` array is empty
- WHEN `PortalScreen` loads
- THEN it renders the existing "Todavía no perteneces a ninguna comunidad"
  empty state, unchanged

### Requirement: Context Selector For Multiple Memberships

When a user has more than one membership, the app MUST present a context
selector listing every membership with its role and let the user pick one
to proceed. Selection MUST be resolvable client-side from the chosen
membership's scope path parameters; no new "set active context" endpoint
is required.

#### Scenario: User with three memberships sees three selectable contexts

- GIVEN a user who is `owner` in one community, `tenant` in another, and
  `admin_staff` in one office
- WHEN they reach the context selector
- THEN all three contexts are listed with their roles

### Requirement: Invitation-Code Entry Point Enabled

The `portal-invitation-link` entry point (and `LoginScreen`'s
`login-invitation-link`, which the Stitch "Iniciar sesión" design also
shows) MUST be enabled (no longer "Próximamente") and MUST open the
invitation-code screen (Stitch "Código de invitación", then "Invitación
reconocida"). The flow uses only the existing sessionless backend
endpoints, with no new endpoint:

1. The entered code (8 alphanumeric characters) is sent to
   `POST /v1/invitations/preview`.
2. A valid code shows the preview — community, unit when assigned, role
   and expiry — before any account action. The preview never shows the
   invited email: the endpoint does not return it (design D-6).
3. Confirming calls `POST /v1/auth/accept-invitation` with name, password,
   consent and platform. For an existing account the user enters that
   account's own password and, when the backend answers
   `AUTH_MFA_REQUIRED`, its TOTP code, then retries.
4. On success the returned session replaces any current one and the user
   lands on the portal.

#### Scenario: Valid short code shows preview before account creation

- GIVEN a user on the portal with no memberships
- WHEN they enter a valid, unexpired short code at `portal-invitation-link`
- THEN the app shows the community/unit/role preview before any account
  is created or linked, without the invited email

#### Scenario: Invalid short code shows a generic error

- WHEN a user enters an invalid or expired short code
- THEN the app shows an error that does not reveal whether any invitation
  ever existed for that code

#### Scenario: Accepting replaces the session and lands on the portal

- GIVEN a previewed invitation
- WHEN the user confirms with a name, a password, consent and — if the
  backend answers `AUTH_MFA_REQUIRED` — the account's TOTP code
- THEN the session returned by `POST /v1/auth/accept-invitation` replaces
  the current one and the user lands on the portal

#### Scenario: A failed accept shows a generic error

- WHEN `POST /v1/auth/accept-invitation` fails for any reason other than
  `AUTH_MFA_REQUIRED`
- THEN the app shows one generic error and keeps the current session
