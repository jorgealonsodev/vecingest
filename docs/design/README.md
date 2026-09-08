# Design source of truth

The UI of this project is derived from the Stitch project **"Vecingest Design System"**.

| Item | Value |
|---|---|
| Stitch project | `projects/11075381530582947267` |
| Screens | 79 (mostly `MOBILE`) |
| Design system — light | "Vecingest" · `assets/9af953c347bf487cbb1358104f564b1f` (v6) · primary token `#024883`, interactive accent in prose `#185FA5` |
| Design system — dark | "Horizontalia Dark Mode" in Stitch (legacy name) · `assets/35ed05bc361041659fbd1d9a021bc6ea` (v1) · primary token `#a9d2ff`, interactive accent in prose `#85B7EB` |
| Fonts | Bricolage Grotesque (headlines) · IBM Plex Sans (body / labels) |

## Files

- [`vecingest-light.md`](./vecingest-light.md) — canonical `design.md` of the light system. This is the primary spec: palette, type scale, spacing, and every component pattern.
- [`vecingest-dark.md`](./vecingest-dark.md) — `design.md` of the dark system.
- [`stitch-screens.md`](./stitch-screens.md) — index of the 79 screens with their Stitch IDs.

## Non-negotiable rules

Taken from the light `design.md`; they apply to every screen built in this repo.

- No gradients, no drop shadows (`box-shadow: none`), no decorative illustrations, no emojis.
- One single interactive accent: `#185FA5`. No other blue appears in the UI.
- One primary button per screen.
- All UI copy in Spanish, sentence case, no exclamation marks, buttons start with a verb.
- Minimum font size 11px; minimum touch target 44px.
- Tabular figures (`font-variant-numeric: tabular-nums`) for quotas, amounts, percentages, and balances.
- Text on a colored background uses the darkest tone of that same family, never black.

## Working with Stitch

The `mcp__stitch__*` tools reach the live project. `list_projects` and `list_screens` outputs exceed the MCP token limit — parse the saved JSON result file instead of reading it inline. Use `get_screen` for a single screen's HTML.

## Product domain

Spanish *Ley de Propiedad Horizontal* (LPH): community management for three roles — vecino (owner), administrador de fincas, and empresa de servicios. Covers incidencias, avisos, documentos, recibos, juntas y votaciones, zonas comunes, fichaje, and facturación.

## Naming

The product was renamed from **Horizontalia** to **Vecingest**. The copies in this folder use the current name; upstream Stitch still carries the old one in the dark system's `displayName` ("Horizontalia Dark Mode"), in its `design.md` frontmatter, and in the light system's `design.md` body (its H1 and the "Producto" bullet), even though the light frontmatter `name` is already `Vecingest`. Treat Vecingest as the product name in all new work, and expect that drift when pulling content back from Stitch.
