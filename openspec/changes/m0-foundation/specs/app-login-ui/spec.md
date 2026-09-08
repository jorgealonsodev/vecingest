# App Login UI Specification

## Purpose

The Expo login screen, built against generated OpenAPI-derived schemas,
with secure token storage, per PRD §7.1, §7.7, and `docs/design/README.md`.

## Requirements

### Requirement: Login Screen Light and Dark Mode

The Expo login screen MUST render correctly in both light and dark mode.
Both themes ship at M0 because `docs/design/` defines a complete light
system (`vecingest-light.md`) and a complete dark system
(`vecingest-dark.md`), and the app follows the device theme;
`docs/design/README.md` itself does not mandate dark mode — its
non-negotiable rules are the single interactive accent `#185FA5`, no
gradients or drop shadows, Spanish sentence-case verb-first copy, and the
11px/44px minimums. The screen MUST use the single accent color and
Spanish sentence-case, verb-first copy in both themes.

#### Scenario: Dark mode renders without contrast failures

- GIVEN the device is set to dark mode
- WHEN the login screen renders
- THEN all text meets the design system's minimum contrast and no
  light-mode-only assets leak through

### Requirement: Generated-Schema Form Validation

The login form MUST use React Hook Form with `zodResolver` against the Zod
schema generated in `packages/shared`, and MUST NOT declare a hand-written
validation schema.

#### Scenario: Invalid credentials shape rejected client-side

- GIVEN the generated login schema requires a non-empty password
- WHEN the user submits an empty password
- THEN the form blocks submission using the generated schema's validation,
  before any network call

### Requirement: Secure Token Storage

Access and refresh tokens on mobile MUST be stored in `expo-secure-store`,
never in `AsyncStorage` or plain state persisted to disk.

#### Scenario: Tokens persisted securely after login

- GIVEN a successful mobile login
- WHEN tokens are persisted
- THEN they are written through `expo-secure-store`
