import {
  ACCENT_DARK,
  ACCENT_LIGHT,
  darkTheme,
  lightTheme,
  pickTheme,
} from "./index";

/**
 * Pure unit coverage for the `useColorScheme()`-driven theme switch used by
 * `app/(auth)/login.tsx` (design.md "Expo login screen": "switched by
 * useColorScheme()"). Kept dependency-free from any native module so the
 * switching logic itself is exercised deterministically.
 */
describe("pickTheme", () => {
  it("picks the dark theme, with its own accent, for 'dark'", () => {
    expect(pickTheme("dark")).toBe(darkTheme);
    expect(darkTheme.colors.primary).toBe(ACCENT_DARK);
  });

  it("picks the light theme, with its own accent, for 'light'", () => {
    expect(pickTheme("light")).toBe(lightTheme);
    expect(lightTheme.colors.primary).toBe(ACCENT_LIGHT);
  });

  it("falls back to the light theme when there is no device preference", () => {
    expect(pickTheme(null)).toBe(lightTheme);
    expect(pickTheme(undefined)).toBe(lightTheme);
  });

  it("keeps light and dark as two distinct accents — no other blue in the interface", () => {
    expect(lightTheme.colors.primary).not.toBe(darkTheme.colors.primary);
  });

  /**
   * Pins the brand tokens against the PROSE of docs/design/vecingest-light.md,
   * corroborated by the "vecingest WEB" system (docs/design/vecingest-web.md),
   * whose frontmatter and prose agree on all four.
   *
   * These exact values were once "corrected" to the frontmatter's competing
   * MD3 palette (#1b1c18, #727782, #c1c6d2, #ba1a1a) on the belief that they
   * had been invented. They had not. This test exists so the same mistake
   * fails here instead of reaching a screen.
   */
  it("keeps the brand tokens the prose specifies, not the frontmatter's MD3 palette", () => {
    expect(lightTheme.colors.onBackground).toBe("#0f2a4a");
    expect(lightTheme.colors.onSurface).toBe("#0f2a4a");
    expect(lightTheme.colors.onSurfaceVariant).toBe("#5f6b7a");
    expect(lightTheme.colors.outline).toBe("#d6dad5");
    expect(lightTheme.colors.error).toBe("#a32d2d");
  });
});
