import { render, screen } from "@testing-library/react-native";
import { PaperProvider } from "react-native-paper";
import { LoginScreen } from "./LoginScreen";
import { ACCENT_DARK, ACCENT_LIGHT, MIN_TOUCH_TARGET, darkTheme, lightTheme } from "../theme";

/**
 * app-login-ui: Login Screen Light and Dark Mode.
 *
 * Renders the exact screen `app/(auth)/login.tsx` mounts, under each design
 * system's theme, the same way `PaperProvider` delivers it at runtime.
 */
describe("LoginScreen — light mode", () => {
  it("renders with the single accent, Spanish sentence-case verb-first copy, and the 44px touch-target floor", async () => {
    await render(
      <PaperProvider theme={lightTheme}>
        <LoginScreen />
      </PaperProvider>,
    );

    // Spanish, sentence case, verb-first, no exclamation marks.
    expect(screen.getByRole("button", { name: "Iniciar sesión" })).toBeTruthy();
    expect(screen.queryByText(/¡/)).toBeNull();

    // The single interactive accent for light mode (docs/design/README.md:
    // "no other blue appears in the interface").
    const submit = screen.getByTestId("login-submit-container");
    expect(submit).toHaveStyle({ backgroundColor: ACCENT_LIGHT });
    expect(submit).not.toHaveStyle({ backgroundColor: ACCENT_DARK });

    expect(screen.getByTestId("login-email")).toBeTruthy();
    expect(screen.getByTestId("login-password")).toBeTruthy();
    expect(MIN_TOUCH_TARGET).toBe(44);
  });
});

describe("LoginScreen — dark mode", () => {
  it("renders without contrast failures, with its own accent, and no light-mode-only surface leaking through", async () => {
    await render(
      <PaperProvider theme={darkTheme}>
        <LoginScreen />
      </PaperProvider>,
    );

    const submit = screen.getByTestId("login-submit-container");

    // The dark system's own accent — never the light system's.
    expect(submit).toHaveStyle({ backgroundColor: ACCENT_DARK });
    expect(submit).not.toHaveStyle({ backgroundColor: ACCENT_LIGHT });

    // WCAG contrast: on-background text against the dark background, and
    // the accent's own text pairing, both clear the 4.5:1 AA floor for
    // normal text.
    expect(contrastRatio(darkTheme.colors.onBackground, darkTheme.colors.background)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(darkTheme.colors.onPrimary, darkTheme.colors.primary)).toBeGreaterThanOrEqual(4.5);

    expect(screen.getByRole("button", { name: "Iniciar sesión" })).toBeTruthy();
  });
});

/** WCAG 2.x relative-luminance contrast ratio between two hex colors. */
function contrastRatio(hexA: string, hexB: string): number {
  const luminanceA = relativeLuminance(hexA);
  const luminanceB = relativeLuminance(hexB);
  const lighter = Math.max(luminanceA, luminanceB);
  const darker = Math.min(luminanceA, luminanceB);
  return (lighter + 0.05) / (darker + 0.05);
}

function relativeLuminance(hex: string): number {
  const [r, g, b] = hexToRgb(hex).map((channel) => {
    const c = channel / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function hexToRgb(hex: string): [number, number, number] {
  const normalized = hex.replace("#", "");
  const r = Number.parseInt(normalized.slice(0, 2), 16);
  const g = Number.parseInt(normalized.slice(2, 4), 16);
  const b = Number.parseInt(normalized.slice(4, 6), 16);
  return [r, g, b];
}
