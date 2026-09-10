import { ACCENT_DARK, ACCENT_LIGHT, darkTheme, lightTheme, pickTheme } from "./index";

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

  it("pins background/surface to the exact docs/design frontmatter hex values", () => {
    // docs/design/vecingest-light.md `colors:` frontmatter.
    expect(lightTheme.colors.background).toBe("#fbf9f2");
    expect(lightTheme.colors.surface).toBe("#fbf9f2");

    // docs/design/vecingest-dark.md `colors:` frontmatter.
    expect(darkTheme.colors.background).toBe("#131312");
    expect(darkTheme.colors.surface).toBe("#131312");
  });
});
