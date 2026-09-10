import { MD3DarkTheme, MD3LightTheme, type MD3Theme } from "react-native-paper";

/**
 * Theme tokens for the Expo app, built from the machine-readable `colors`
 * frontmatter blocks of `docs/design/vecingest-light.md` and
 * `vecingest-dark.md` (design.md "Expo login screen" section).
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
 * pair, and keep every other MD3 token as the frontmatter defines it.
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
  error: "#ba1a1a",
  onError: "#ffffff",
  errorContainer: "#ffdad6",
  onErrorContainer: "#93000a",
  background: "#fbf9f2",
  onBackground: "#1b1c18",
  surface: "#fbf9f2",
  onSurface: "#1b1c18",
  surfaceVariant: "#e4e2dc",
  onSurfaceVariant: "#424751",
  outline: "#727782",
  outlineVariant: "#c1c6d2",
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
  surface: "#131312",
  onSurface: "#e5e2df",
  surfaceVariant: "#353533",
  onSurfaceVariant: "#c1c7d0",
  outline: "#8b919a",
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
    headlineLarge: { ...MD3LightTheme.fonts.headlineLarge, ...fontConfig.headlineLarge },
    bodyMedium: { ...MD3LightTheme.fonts.bodyMedium, ...fontConfig.bodyMedium },
    labelMedium: { ...MD3LightTheme.fonts.labelMedium, ...fontConfig.labelMedium },
    labelSmall: { ...MD3LightTheme.fonts.labelSmall, ...fontConfig.labelSmall },
  },
};

export const darkTheme: MD3Theme = {
  ...MD3DarkTheme,
  colors: darkColors,
  fonts: {
    ...MD3DarkTheme.fonts,
    headlineLarge: { ...MD3DarkTheme.fonts.headlineLarge, ...fontConfig.headlineLarge },
    bodyMedium: { ...MD3DarkTheme.fonts.bodyMedium, ...fontConfig.bodyMedium },
    labelMedium: { ...MD3DarkTheme.fonts.labelMedium, ...fontConfig.labelMedium },
    labelSmall: { ...MD3DarkTheme.fonts.labelSmall, ...fontConfig.labelSmall },
  },
};

/** Minimum type size and touch target from `docs/design/README.md`. */
export const MIN_FONT_SIZE = 11;
export const MIN_TOUCH_TARGET = 44;

/**
 * Picks the theme for the device's color scheme. `login.tsx` wires this to
 * `useColorScheme()`; extracted as a pure function so the light/dark
 * selection is unit-testable without mocking the native `Appearance` module.
 * No preference (`null`/`undefined`) falls back to light.
 */
export function pickTheme(scheme: string | null | undefined): MD3Theme {
  return scheme === "dark" ? darkTheme : lightTheme;
}
