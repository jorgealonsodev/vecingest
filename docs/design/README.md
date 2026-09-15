# Design source of truth

The UI of this project is derived from **two** Stitch projects. There is no single "source of truth" project anymore: each one governs a different surface.

| Item | `VecinGest APP` (mobile) | `VecinGest WEB` (web) |
|---|---|---|
| Stitch project | `projects/11075381530582947267` | `projects/14138416730329310203` |
| Title returned by the Stitch API | "Vecingest APP" | "VecinGest WEB" |
| Screens | 79 (mostly `MOBILE`, 390px wide) | 48 (`DESKTOP`, 1280–2732px wide) |
| Design system name | "Vecingest" (light) / "Horizontalia Dark Mode" (dark, legacy name) | "Vecingest Institutional Engine" |
| Governs | Native mobile app screens | `vecingest.app` (Astro `site/` package) and `app.vecingest.app` (the Expo app's web export / operational console) |
| Fonts | Bricolage Grotesque (headlines) · IBM Plex Sans (body / labels) | same |

### Project rename (2026-09-15)

Both Stitch projects were renamed on 2026-09-15 specifically to remove WEB/APP ambiguity:

- `projects/14138416730329310203` — "VecinGest Design System Screens" → **"VecinGest WEB"**.
- `projects/11075381530582947267` — "Vecingest Design System" → **"Vecingest APP"**.

If you find either old name in a commit message, an earlier draft of this doc, or anywhere else in this repo's history, map it forward: `Vecingest Design System` / `VecinGest Design System Screens` are the pre-rename names of the APP and WEB projects respectively.

## Which system governs what

The web design system's own `design.md` is explicit about the split — quoted verbatim from its "Layout & Spacing" section:

> - **Public Marketing Surface (`vecingest.app`):** Fixed central content constraint of 1120px with 24px horizontal gutters. Structural sections are delineated by 1px solid dividers (`#D6DAD5`) paired with 72px vertical spacing blocks.
> - **Operational Console (`app.vecingest.app`):** Persistent, non-collapsible 240px left-hand navigation sidebar anchored to the viewport height. The main canvas receives 24px internal padding. High-density data tables conform to fixed 48px row heights.

So: `vecingest.app` (the marketing site, the Astro `site/` package) and `app.vecingest.app` (the operational console, the Expo app's web export) are both governed by the **web** design system (`vecingest-web.md`). The native mobile app screens come from the separate **APP** project (`vecingest-light.md` / `vecingest-dark.md`).

The web system is materially richer than the app one: it carries a `spacing:` block (content widths, sidebar width, row heights) and explicit layout/breakpoint rules that the app system never had.

## Files

- [`vecingest-light.md`](./vecingest-light.md) — canonical `design.md` of the APP project's light system. Palette, type scale, spacing, and every mobile component pattern. **See the warning below about its frontmatter.**
- [`vecingest-dark.md`](./vecingest-dark.md) — `design.md` of the APP project's dark system.
- [`stitch-screens.md`](./stitch-screens.md) — index of the APP project's 79 screens with their Stitch IDs.
- [`vecingest-web.md`](./vecingest-web.md) — canonical `design.md` of the WEB project ("Vecingest Institutional Engine"). Governs both `vecingest.app` and `app.vecingest.app`.
- [`stitch-screens-web.md`](./stitch-screens-web.md) — index of the WEB project's 48 screens with their Stitch IDs.

## Warning: `vecingest-light.md` frontmatter disagrees with its own prose

`vecingest-light.md`'s YAML frontmatter and the Spanish prose in the same file **do not agree** on core neutral/text tokens:

| Token | Frontmatter (`vecingest-light.md`) | Prose (`vecingest-light.md`, "Paleta de color") |
|---|---|---|
| Main text / `on-background` | `#1b1c18` | `#0F2A4A` ("Texto principal") |
| Secondary text / `outline` | `#727782` | `#5F6B7A` ("Texto secundario y descriptivo") |
| Borders / `outline-variant` | `#c1c6d2` | `#D6DAD5` ("Líneas divisorias y bordes") |
| Error text | *(frontmatter has no `error-text` key; only Material `error: '#ba1a1a'`)* | `#A32D2D` ("Error") |

The WEB project's `design.md` corroborates the **prose** values, and it agrees with itself: its frontmatter carries `on-background: '#0F2A4A'`, `outline: '#5F6B7A'`, `outline-variant: '#D6DAD5'`, `error-text: '#A32D2D'` — and its own prose ("Colors" section: "deep corporate navy typography (`#0F2A4A`)", secondary `#5F6B7A`, dividers `#D6DAD5`, error `#A32D2D`) uses the exact same values, with no internal conflict.

**Treat `vecingest-light.md`'s frontmatter as the unreliable half of that file.** This drift has already caused at least one wrong change in this repo. Do not edit `vecingest-light.md` to "fix" this — this note is documentation only, per the scope of this task; a correction to that file is a separate, deliberate change.

## Non-negotiable rules

Taken from the light `design.md` (APP) and corroborated by the WEB `design.md`; they apply to every screen built in this repo, mobile or web.

- No gradients, no drop shadows (`box-shadow: none`), no decorative illustrations, no emojis.
- One single interactive accent: `#185FA5`. No other blue appears in the UI.
- One primary button per screen/view.
- All UI copy in Spanish, sentence case, no exclamation marks, buttons start with a verb.
- Minimum font size 11px; minimum touch target 44px (mobile) / control height 44px (web).
- Tabular figures (`font-variant-numeric: tabular-nums`) for quotas, amounts, percentages, and balances.
- Text on a colored background uses the darkest tone of that same family, never black.

## Working with Stitch

The `mcp__stitch__*` tools reach the live projects. `list_projects` and `list_screens` outputs exceed the MCP token limit — parse the saved JSON result file instead of reading it inline (or, when possible, work screen-by-screen from the `screenInstances` ids returned by `get_project`). Use `get_screen` for a single screen's HTML.

## Product domain

Spanish *Ley de Propiedad Horizontal* (LPH): community management for three roles — vecino (owner), administrador de fincas, and empresa de servicios. Covers incidencias, avisos, documentos, recibos, juntas y votaciones, zonas comunes, fichaje, and facturación.

## Naming

The product was renamed from **Horizontalia** to **Vecingest**. The copies in this folder use the current name; upstream Stitch still carries the old one in the APP project's dark system `displayName` ("Horizontalia Dark Mode"), in its `design.md` frontmatter, and in the APP light system's `design.md` body (its H1 and the "Producto" bullet), even though the light frontmatter `name` is already `Vecingest`. The WEB project's `design.md` (`name: Vecingest Institutional Engine`) does not carry this drift — it is consistently named in both frontmatter and prose. Treat Vecingest as the product name in all new work, and expect the APP project's drift when pulling content back from Stitch.
