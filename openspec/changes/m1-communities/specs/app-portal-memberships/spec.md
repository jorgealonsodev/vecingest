# App Portal Memberships Specification

## Purpose

Turns `PortalScreen.tsx`'s already-built empty state into "portales vacíos
por rol": populated branch, context selector, invitation-code entry, per
`docs/funcionalidad/app-movil-2.md` ("Elegir dónde entrar") and
`consola-web.md`'s M1 section. No new screen; `GET /v1/me`'s `memberships`
array is the only new wire.

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

The `portal-invitation-link` entry point MUST be enabled (no longer
"Próximamente") and MUST call `POST /v1/invitations/preview` with the
entered code before navigating to the accept-invitation screen.

#### Scenario: Valid short code shows preview before account creation

- GIVEN a user on the portal with no memberships
- WHEN they enter a valid, unexpired short code at `portal-invitation-link`
- THEN the app shows the community/unit/role preview before any account
  is created

#### Scenario: Invalid short code shows a generic error

- WHEN a user enters an invalid or expired short code
- THEN the app shows an error that does not reveal whether any invitation
  ever existed for that code
