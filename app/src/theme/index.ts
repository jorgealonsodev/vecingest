import { MD3DarkTheme, MD3LightTheme, type MD3Theme } from "react-native-paper";

/**
 * Theme tokens for the Expo app, following the BRAND values in the PROSE of
 * `docs/design/vecingest-light.md` and `vecingest-dark.md` — NOT their YAML
 * frontmatter.
 *
 * Read that distinction before changing anything here, because it has already
 * cost one wrong change. Those two documents contradict themselves: the
 * frontmatter carries `on-background: '#1b1c18'` and `outline: '#727782'`,
 * while the prose of the very same file specifies `#0F2A4A`, `#5F6B7A`,
 * `#D6DAD5` and `#A32D2D`. The frontmatter is Stitch's auto-generated MD3
 * palette; the prose is the brand.
 *
 * The prose wins, and the "vecingest WEB" Stitch project
 * (`projects/14138416730329310203`, mirrored in `docs/design/vecingest-web.md`)
 * settles it: its frontmatter AND its prose both carry `on-background:
 * '#0F2A4A'`, `outline: '#5F6B7A'`, `outline-variant: '#D6DAD5'` and
 * `error-text: '#A32D2D'`, with no internal disagreement. That project governs
 * `app.vecingest.app`, which is this app's own web export.
 *
 * Deliberate override: `docs/design/README.md`'s non-negotiable rule is a
 * SINGLE interactive accent per theme (`#185FA5` light, `#85B7EB` dark) with
 * "no other blue appears in the interface". The frontmatter's own `primary`
 * token (`#024883` light, `#a9d2ff` dark) is a *different* blue than that
 * accent, so using it verbatim for `theme.colors.primary` would put a second
 * blue on screen the moment any Paper component reads `colors.primary`
 * (default button color, focus rings, active tab, etc.). We therefore
 * override `primary`/`onPrimary`/`primaryContainer`/`onPrimaryContainer` to
 * the accent pairing instead of the frontmatter's `primary`/`on-primary`
 * pair, and keep every other MD3 token as the PROSE defines it.
 */

export const ACCENT_LIGHT = "#185FA5";
export const ACCENT_DARK = "#85B7EB";

const lightColors = {
  ...MD3LightTheme.colors,
  primary: ACCENT_LIGHT,
  onPrimary: "#ffffff",
  primaryContainer: "#2b609c",
  onPrimaryContainer: "#c4daff",
  secondary: "#535f6e",
  onSecondary: "#ffffff",
  secondaryContainer: "#d4e1f3",
  onSecondaryContainer: "#586473",
  error: "#a32d2d",
  onError: "#ffffff",
  errorContainer: "#fcebeb",
  onErrorContainer: "#a32d2d",
  background: "#f1efe8",
  onBackground: "#0f2a4a",
  surface: "#ffffff",
  onSurface: "#0f2a4a",
  surfaceVariant: "#e4e2dc",
  onSurfaceVariant: "#5f6b7a",
  outline: "#d6dad5",
  outlineVariant: "#d6dad5",
};

const darkColors = {
  ...MD3DarkTheme.colors,
  primary: ACCENT_DARK,
  onPrimary: "#014876",
  primaryContainer: "#014876",
  onPrimaryContainer: "#85b7eb",
  secondary: "#c9c6bd",
  onSecondary: "#31312a",
  secondaryContainer: "#474740",
  onSecondaryContainer: "#b7b5ac",
  error: "#ffb4ab",
  onError: "#690005",
  errorContainer: "#93000a",
  onErrorContainer: "#ffdad6",
  background: "#131312",
  onBackground: "#e5e2df",
  surface: "#1b1c1a",
  onSurface: "#e5e2df",
  surfaceVariant: "#353533",
  onSurfaceVariant: "#c1c7d0",
  outline: "#41474f",
  outlineVariant: "#41474f",
};

const fontConfig = {
  headlineLarge: {
    fontFamily: "BricolageGrotesque_600SemiBold",
    fontSize: 28,
    fontWeight: "600" as const,
    lineHeight: 34,
  },
  bodyMedium: {
    fontFamily: "IBMPlexSans_400Regular",
    fontSize: 16,
    fontWeight: "400" as const,
    lineHeight: 24,
  },
  labelMedium: {
    fontFamily: "IBMPlexSans_500Medium",
    fontSize: 13,
    fontWeight: "500" as const,
    lineHeight: 18,
  },
  labelSmall: {
    fontFamily: "IBMPlexSans_500Medium",
    fontSize: 11,
    fontWeight: "500" as const,
    lineHeight: 14,
  },
};

export const lightTheme: MD3Theme = {
  ...MD3LightTheme,
  colors: lightColors,
  fonts: {
    ...MD3LightTheme.fonts,
    headlineLarge: {
      ...MD3LightTheme.fonts.headlineLarge,
      ...fontConfig.headlineLarge,
    },
    bodyMedium: { ...MD3LightTheme.fonts.bodyMedium, ...fontConfig.bodyMedium },
    labelMedium: {
      ...MD3LightTheme.fonts.labelMedium,
      ...fontConfig.labelMedium,
    },
    labelSmall: { ...MD3LightTheme.fonts.labelSmall, ...fontConfig.labelSmall },
  },
};

export const darkTheme: MD3Theme = {
  ...MD3DarkTheme,
  colors: darkColors,
  fonts: {
    ...MD3DarkTheme.fonts,
    headlineLarge: {
      ...MD3DarkTheme.fonts.headlineLarge,
      ...fontConfig.headlineLarge,
    },
    bodyMedium: { ...MD3DarkTheme.fonts.bodyMedium, ...fontConfig.bodyMedium },
    labelMedium: {
      ...MD3DarkTheme.fonts.labelMedium,
      ...fontConfig.labelMedium,
    },
    labelSmall: { ...MD3DarkTheme.fonts.labelSmall, ...fontConfig.labelSmall },
  },
};

/**
 * Institutional panel brand surface — the desktop login screen's left
 * panel from the WEB design system (`docs/design/vecingest-web.md`, Stitch
 * project `projects/14138416730329310203`, screen
 * `5f83c21f55d9493da67cc330fa8528d4`, whose left section carries
 * `bg-on-background text-on-primary`). The WEB system's "Colors" prose fixes
 * the page on a warm architectural white with "deep corporate navy
 * typography (`#0F2A4A`)" — that navy-on-white pairing IS this panel.
 *
 * The WEB design system has NO dark mode (there is not a single mention of
 * "dark" in `docs/design/vecingest-web.md`). This panel is therefore a fixed
 * BRAND surface that must not follow the device color scheme — it is not a
 * light/dark pair, just these two literal colors in both themes.
 *
 * Do not "fix" this back to `theme.colors.onBackground` /
 * `theme.colors.onPrimary`. Those MD3 tokens are text-on-surface pairs that
 * deliberately invert between the light and dark themes (`onBackground` is
 * navy in light but pale beige in dark; `onPrimary` is white in light but
 * dark blue in dark) — binding the panel to them once shipped the panel as
 * a pale beige slab with barely-legible dark-blue-on-beige text in dark
 * mode. See `LoginScreen.theme.test.tsx`'s "institutional panel" test.
 */
export const INSTITUTIONAL_PANEL_BACKGROUND = "#0f2a4a";
export const INSTITUTIONAL_PANEL_FOREGROUND = "#ffffff";

/**
 * Web-console light-only surface tokens — the desktop split of the auth
 * screens (login, forgot password, reset password) is part of the "VecinGest
 * WEB" design system (`docs/design/vecingest-web.md`, Stitch project
 * `projects/14138416730329310203`), which has NO dark mode at all (zero
 * mentions of "dark" in that document — see `docs/design/README.md`'s
 * project table). These are therefore fixed BRAND values, exactly like
 * `INSTITUTIONAL_PANEL_BACKGROUND`/`FOREGROUND` above — never
 * `theme.colors.background` / `theme.colors.surface`, which invert with the
 * device color scheme and, in dark mode, both resolve to `#131312`,
 * rendering the card invisible against the page. Mobile keeps using
 * `pickTheme()` / `useColorScheme()` against `docs/design/vecingest-dark.md`,
 * which DOES define a dark palette — do not apply these tokens there.
 */
export const WEB_PAGE_BACKGROUND = "#f4f5f2"; // background
export const WEB_SURFACE = "#ffffff"; // surface (header + card chrome)
export const WEB_BORDER = "#d6dad5"; // outline-variant
export const WEB_TEXT_PRIMARY = "#0f2a4a"; // on-background (same literal as INSTITUTIONAL_PANEL_BACKGROUND, used as text here instead of a fill)
export const WEB_HEADER_HEIGHT = 64; // header-height
export const WEB_CONTENT_MAX = 1120; // content-max

/** Minimum type size and touch target from `docs/design/README.md`. */
export const MIN_FONT_SIZE = 11;
export const MIN_TOUCH_TARGET = 44;

/**
 * Spacing scale, from `docs/design/vecingest-web.md`'s `## Layout & Spacing`
 * section / frontmatter `spacing:` block (Vecingest Institutional Engine
 * design system, the WEB Stitch project `projects/14138416730329310203`,
 * which governs this app's web export — see that document's own header).
 */
export const spacing = {
  space4: 4,
  space8: 8,
  space12: 12,
  space16: 16,
  space24: 24,
  space32: 32,
  space40: 40,
  space48: 48,
} as const;

/**
 * Desktop breakpoint, from `docs/design/vecingest-web.md`'s breakpoint table
 * (`Desktop (> 1024px)`) — the Stitch design system's `lg` breakpoint.
 */
export const BREAKPOINT_DESKTOP = 1024;

/**
 * Picks the theme for the device's color scheme. `login.tsx` wires this to
 * `useColorScheme()`; extracted as a pure function so the light/dark
 * selection is unit-testable without mocking the native `Appearance` module.
 * No preference (`null`/`undefined`) falls back to light.
 */
export function pickTheme(scheme: string | null | undefined): MD3Theme {
  return scheme === "dark" ? darkTheme : lightTheme;
}
