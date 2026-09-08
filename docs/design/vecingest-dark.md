---
name: Vecingest Dark Mode
colors:
  surface: '#131312'
  surface-dim: '#131312'
  surface-bright: '#393937'
  surface-container-lowest: '#0e0e0d'
  surface-container-low: '#1b1c1a'
  surface-container: '#20201e'
  surface-container-high: '#2a2a28'
  surface-container-highest: '#353533'
  on-surface: '#e5e2df'
  on-surface-variant: '#c1c7d0'
  inverse-surface: '#e5e2df'
  inverse-on-surface: '#30302e'
  outline: '#8b919a'
  outline-variant: '#41474f'
  surface-tint: '#9acbff'
  primary: '#a9d2ff'
  on-primary: '#003355'
  primary-container: '#85b7eb'
  on-primary-container: '#014876'
  inverse-primary: '#2b6291'
  secondary: '#c9c6bd'
  on-secondary: '#31312a'
  secondary-container: '#474740'
  on-secondary-container: '#b7b5ac'
  tertiary: '#fec46f'
  on-tertiary: '#442b00'
  tertiary-container: '#e0a957'
  on-tertiary-container: '#603e00'
  error: '#ffb4ab'
  on-error: '#690005'
  error-container: '#93000a'
  on-error-container: '#ffdad6'
  primary-fixed: '#cfe5ff'
  primary-fixed-dim: '#9acbff'
  on-primary-fixed: '#001d34'
  on-primary-fixed-variant: '#054a77'
  secondary-fixed: '#e5e2d9'
  secondary-fixed-dim: '#c9c6bd'
  on-secondary-fixed: '#1c1c16'
  on-secondary-fixed-variant: '#474740'
  tertiary-fixed: '#ffddb2'
  tertiary-fixed-dim: '#f6bc68'
  on-tertiary-fixed: '#291800'
  on-tertiary-fixed-variant: '#624000'
  background: '#131312'
  on-background: '#e5e2df'
  surface-variant: '#353533'
typography:
  display-lg:
    fontFamily: Bricolage Grotesque
    fontSize: 32px
    fontWeight: '600'
    lineHeight: 40px
    letterSpacing: -0.02em
  headline-lg:
    fontFamily: Bricolage Grotesque
    fontSize: 24px
    fontWeight: '600'
    lineHeight: 32px
    letterSpacing: -0.015em
  headline-md:
    fontFamily: Bricolage Grotesque
    fontSize: 20px
    fontWeight: '600'
    lineHeight: 28px
    letterSpacing: -0.01em
  headline-sm:
    fontFamily: Bricolage Grotesque
    fontSize: 16px
    fontWeight: '600'
    lineHeight: 24px
    letterSpacing: -0.005em
  body-lg:
    fontFamily: IBM Plex Sans
    fontSize: 16px
    fontWeight: '400'
    lineHeight: 24px
  body-md:
    fontFamily: IBM Plex Sans
    fontSize: 14px
    fontWeight: '400'
    lineHeight: 20px
  body-sm:
    fontFamily: IBM Plex Sans
    fontSize: 12px
    fontWeight: '400'
    lineHeight: 16px
  label-lg:
    fontFamily: IBM Plex Sans
    fontSize: 14px
    fontWeight: '500'
    lineHeight: 20px
    letterSpacing: 0.01em
  label-md:
    fontFamily: IBM Plex Sans
    fontSize: 12px
    fontWeight: '500'
    lineHeight: 16px
    letterSpacing: 0.02em
  label-sm:
    fontFamily: IBM Plex Sans
    fontSize: 11px
    fontWeight: '600'
    lineHeight: 14px
    letterSpacing: 0.04em
  code-num:
    fontFamily: IBM Plex Sans
    fontSize: 14px
    fontWeight: '500'
    lineHeight: 20px
rounded:
  sm: 0.125rem
  DEFAULT: 0.25rem
  md: 0.375rem
  lg: 0.5rem
  xl: 0.75rem
  full: 9999px
spacing:
  space-2xs: 0.125rem
  space-xs: 0.25rem
  space-sm: 0.5rem
  space-md: 0.75rem
  space-base: 1rem
  space-lg: 1.5rem
  space-xl: 2rem
  space-2xl: 3rem
  gutter-mobile: 1rem
  gutter-desktop: 1.5rem
  margin-mobile: 1rem
  margin-desktop: 2rem
---

## Brand & Style

This design system establishes a specialized dark environment tailored for community management, accounting, and institutional compliance under the Spanish Horizontal Property Law (*Ley de Propiedad Horizontal - LPH*). The aesthetic is grounded in banking precision, institutional restraint, and operational clarity. It avoids neon light emissions, playful decorative flourishes, or expressive ambient illumination.

The visual language draws on Swiss corporate typography combined with architectural rigidity: crisp 1px structural framing, matte mineral surfaces, distinct hierarchical values, and functional color coding. It conveys immutability, meticulous bookkeeping, and unambiguous administrative authority during nocturnal operations or high-density auditing sessions.

## Colors

The palette operates on low-luminance warm mineral charcoal bases paired with high-legibility parchment text, restrained steel borders, and deliberate slate-blue operational accents.

### Core Canvas & Structure
- **Canvas Base (`#2C2C2A`):** Deep charcoal slate background. Provides the structural foundation without harsh pure-black eye strain.
- **Surface / Container (`#444441`):** Muted warm stone for cards, modular panels, data tables, and modal dialogues.
- **Structural Line / Border (`#5F5E5A`):** Crisp 1px boundary marker used across dividers, form fields, headers, and grid partitions.

### Typography & Readability
- **Primary Readout (`#F1EFE8`):** Off-white bone parchment. High legibility for financial figures, quotas, and resolutions without clinical blue glare.
- **Secondary Readout (`#B4B2A9`):** Neutralized stone grey for field labels, metadata, IBAN numbers, and legal citations.

### Accents & Semantic States
- **Primary Operational (`#85B7EB`):** Controlled cerulean accent. Reserved for primary buttons, active tabs, selected states, and focused interactive inputs.
- **Success / Quota Cleared:** Container background `#1C3829` paired with text `#A3E5B9` and border `#2D5940`. Used for balance confirmations, quorum attainment, and paid receipts.
- **Urgent / Impago / Destructive:** Container background `#3E1F1F` paired with text `#FCA5A5` and border `#6B3232`. Used for formal notices, default debts (*impagos*), and destructive actions.
- **Warning / Pending Audit:** Container background `#3E2E18` paired with text `#FCD34D` and border `#6B5125`. Used for pending convocations, unverified balances, and legal deadlines.

## Typography

The type system blends the structured mechanical authority of **Bricolage Grotesque** for navigational landmarks, section headlines, and balance figures with the legibility of **IBM Plex Sans** for tabular ledgers, legal clauses, minute summaries, and forms.

Tabular figures (`tnum`) and lining figures (`lnum`) are mandatory across financial ledgers, coefficient decimals (*cuotas de participación*), and property indices. Headlines avoid high font-weight bloat, prioritizing compact tracking and clarity.

## Layout & Spacing

A compact, density-tolerant 12-column grid is enforced for desktop management views, collapsing to a single-column layout on mobile devices. Spacing scales along a disciplined 4px/8px rhythm.

- **Desktop (>= 1024px):** 12-column layout, 24px gutters, max layout container width 1440px. Accounting tables and LPH quota breakdowns expand dynamically across available space while retaining standard column alignment.
- **Tablet (768px - 1023px):** 8-column layout, 16px gutters, 24px outer margins. Split-screen panels (e.g., owner balance vs. registered notifications) stack or shift into tabbed overlays.
- **Mobile (< 768px):** 4-column layout, 16px gutters, 16px margins. Data grids transition into bordered card stacks.

## Elevation & Depth

This design system rejects deep drop shadows, diffuse glows, or neo-brutalist heavy offsets. Hierarchy is strictly structural:

1. **Base Surface (Level 0):** `#2C2C2A` for application framing, global page backgrounds, and side navigation.
2. **Elevated Surface (Level 1):** `#444441` for actionable content cards, table headers, document lists, and form panels. Every surface container must be framed with a crisp `1px solid #5F5E5A` border.
3. **Modal & Floating Menus (Level 2):** `#444441` with a hairline highlight border `1px solid #85B7EB` when active, accompanied by an ambient, low-spread drop shadow (`box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45)`).
4. **Dividers:** Internal section breaks within containers use `1px solid #5F5E5A` with zero box shadow or bevel.

## Shapes

The interface balances functional precision with modern restraint using `rounded-md` (6px) and `rounded-lg` (8px). Large pill shapes and sharp zero-radius geometry are excluded.

- **`rounded-md` (6px):** Checkboxes, radio indicators, badge tags, inline chips, status pills, and action icon containers.
- **`rounded-lg` (8px):** Input text fields, select dropdown triggers, button containers, data tables, structural cards, and modal dialogue panels.

## Components

### Buttons
- **Primary:** Background `#85B7EB`, text `#1A2836` (deep slate navy for high contrast), weight 600, border radius `rounded-lg` (8px), padding `10px 16px`. Hover: `#9EC5F0`. Active: `#6FA5E0`. Focus outline: 2px offset `#85B7EB`.
- **Secondary:** Background transparent, text `#F1EFE8`, 1px solid border `#5F5E5A`, border radius `rounded-lg`. Hover: background `#444441` with text `#F1EFE8`.
- **Destructive:** Background `#3E1F1F`, text `#FCA5A5`, 1px solid border `#6B3232`, border radius `rounded-lg`. Hover: background `#4D2626`.

### Input Fields & Selects
- Container background `#2C2C2A`, border `1px solid #5F5E5A`, text `#F1EFE8`, placeholder `#B4B2A9`, border radius `rounded-lg` (8px), vertical padding 10px, horizontal padding 12px.
- Focused state: border color `#85B7EB`, box-shadow `0 0 0 1px #85B7EB`.
- Error state: border color `#6B3232`, focus ring `#FCA5A5`.

### Cards & Ledger Blocks
- Background `#444441`, border `1px solid #5F5E5A`, border radius `rounded-lg` (8px), internal padding 16px or 24px.
- Segmented table rows inside cards: separated by `1px solid #5F5E5A`, hover state on rows `#3B3B38`.

### Status Badges & Chips
- Padding `4px 10px`, typography `label-sm` (11px, weight 600, uppercase tracking), border radius `rounded-md` (6px).
- **Cleared / Solvent:** Background `#1C3829`, text `#A3E5B9`, border `1px solid #2D5940`.
- **Delinquent / Impago:** Background `#3E1F1F`, text `#FCA5A5`, border `1px solid #6B3232`.
- **Convocation / Pending:** Background `#3E2E18`, text `#FCD34D`, border `1px solid #6B5125`.
- **Neutral / Informational:** Background `#2C2C2A`, text `#B4B2A9`, border `1px solid #5F5E5A`.

### Checkboxes & Radio Controls
- Base: 18px size, border `1.5px solid #5F5E5A`, background `#2C2C2A`, border radius `rounded-md` (checkbox) or circular (radio).
- Checked: Background `#85B7EB`, border color `#85B7EB`, check icon text color `#1A2836`.

### LPH Specific Components
- **Acta Sign-off Ledger:** Two-column verified audit row with property coefficient, owner name, vote attribution (A favor / En contra / Abstención), and signed timestamp in `code-num` typography.
- **Reserve Fund Breakdown:** Strict horizontal progress gauges framed in `#5F5E5A` with `#85B7EB` and `#A3E5B9` metric divisions.
