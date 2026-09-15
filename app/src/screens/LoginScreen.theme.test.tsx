import {
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react-native";
import { StyleSheet } from "react-native";
import { PaperProvider } from "react-native-paper";
import {
  ACCENT_DARK,
  ACCENT_LIGHT,
  darkTheme,
  INSTITUTIONAL_PANEL_BACKGROUND,
  INSTITUTIONAL_PANEL_FOREGROUND,
  lightTheme,
  MIN_TOUCH_TARGET,
  WEB_SURFACE,
} from "../theme";
import { LoginScreen } from "./LoginScreen";

/** Flattens an RN `style` prop (array or object) down to a plain object. */
function flatStyle(element: {
  props: { style?: unknown };
}): Record<string, unknown> {
  return StyleSheet.flatten(element.props.style) as Record<string, unknown>;
}

// `LoginScreen` reads `useWindowDimensions()` (via `Dimensions.get('window')`
// under the hood) to pick the mobile/desktop split. Mocking the hook's own
// module — rather than spying on the `react-native` barrel export — is the
// reliable way to control it in this RN version, since the barrel re-exports
// it through a non-configurable lazy getter.
jest.mock("react-native/Libraries/Utilities/useWindowDimensions", () => ({
  __esModule: true,
  default: jest.fn(() => ({
    width: 750,
    height: 1334,
    scale: 2,
    fontScale: 1,
  })),
}));

// Ambient type for this internal react-native module path lives in
// `../types/react-native-internal.d.ts` (react-native ships no types for it).
import useWindowDimensions from "react-native/Libraries/Utilities/useWindowDimensions";

function setWindowWidth(width: number): void {
  (useWindowDimensions as jest.Mock).mockReturnValue({
    width,
    height: 900,
    scale: 1,
    fontScale: 1,
  });
}

afterEach(() => {
  setWindowWidth(750);
});

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
    expect(screen.getByRole("button", { name: "Entrar" })).toBeTruthy();
    expect(screen.queryByText(/¡/)).toBeNull();

    // The single interactive accent for light mode (docs/design/README.md:
    // "no other blue appears in the interface").
    const submit = screen.getByTestId("login-submit-container");
    expect(submit).toHaveStyle({ backgroundColor: ACCENT_LIGHT });
    expect(submit).not.toHaveStyle({ backgroundColor: ACCENT_DARK });

    // The screen paints the theme's own background — not left transparent
    // for the browser/OS default grey to show through.
    expect(screen.getByTestId("login-screen")).toHaveStyle({
      backgroundColor: lightTheme.colors.background,
    });

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

    // The screen paints the dark theme's own background, not the light
    // theme's surface leaking through.
    expect(screen.getByTestId("login-screen")).toHaveStyle({
      backgroundColor: darkTheme.colors.background,
    });
    expect(screen.getByTestId("login-screen")).not.toHaveStyle({
      backgroundColor: lightTheme.colors.background,
    });

    // WCAG contrast: on-background text against the dark background, and
    // the accent's own text pairing, both clear the 4.5:1 AA floor for
    // normal text.
    expect(
      contrastRatio(darkTheme.colors.onBackground, darkTheme.colors.background),
    ).toBeGreaterThanOrEqual(4.5);
    expect(
      contrastRatio(darkTheme.colors.onPrimary, darkTheme.colors.primary),
    ).toBeGreaterThanOrEqual(4.5);

    expect(screen.getByRole("button", { name: "Entrar" })).toBeTruthy();
  });
});

describe("LoginScreen — mobile layout content", () => {
  it("renders the kicker, subtitle, password toggle, forgot-password link, and encrypted-connection footer", async () => {
    await render(
      <PaperProvider theme={lightTheme}>
        <LoginScreen />
      </PaperProvider>,
    );

    expect(screen.getByText("Gestión integral de fincas")).toBeTruthy();
    expect(
      screen.getByText("Introduzca sus credenciales autorizadas"),
    ).toBeTruthy();
    expect(screen.getByTestId("login-password-toggle")).toBeTruthy();
    expect(screen.getByTestId("login-forgot-password-link")).toBeTruthy();
    expect(screen.getByText("Conexión cifrada de alta seguridad")).toBeTruthy();

    // Desktop-only institutional copy is absent below the desktop breakpoint
    // (the default test-environment window width).
    expect(
      screen.queryByText("La comunidad de propietarios, en orden"),
    ).toBeNull();
  });

  it("flips the password field's secureTextEntry and its accessibility label when the toggle is pressed", async () => {
    await render(
      <PaperProvider theme={lightTheme}>
        <LoginScreen />
      </PaperProvider>,
    );

    const toggle = screen.getByTestId("login-password-toggle");
    expect(screen.getByLabelText("Mostrar contraseña")).toBeTruthy();

    await fireEvent.press(toggle);

    expect(screen.getByLabelText("Ocultar contraseña")).toBeTruthy();
  });

  it("keeps the invitation-code entry point visually present but non-functional", async () => {
    await render(
      <PaperProvider theme={lightTheme}>
        <LoginScreen />
      </PaperProvider>,
    );

    const invitation = screen.getByTestId("login-invitation-link");
    expect(invitation.props.accessibilityState?.disabled).toBe(true);
    expect(screen.getByText("Próximamente")).toBeTruthy();
  });
});

describe("LoginScreen — desktop split layout", () => {
  it("renders the institutional left panel at and above the desktop breakpoint", async () => {
    setWindowWidth(1280);

    try {
      await render(
        <PaperProvider theme={lightTheme}>
          <LoginScreen />
        </PaperProvider>,
      );

      expect(
        screen.getByText("La comunidad de propietarios, en orden"),
      ).toBeTruthy();
    } finally {
      setWindowWidth(750);
    }
  });

  it("does not render the desktop split below the breakpoint", async () => {
    setWindowWidth(800);

    try {
      await render(
        <PaperProvider theme={lightTheme}>
          <LoginScreen />
        </PaperProvider>,
      );

      expect(
        screen.queryByText("La comunidad de propietarios, en orden"),
      ).toBeNull();
    } finally {
      setWindowWidth(750);
    }
  });
});

describe("LoginScreen — institutional panel (WEB design system, no dark mode)", () => {
  it.each([
    ["light", lightTheme],
    ["dark", darkTheme],
  ] as const)(
    "keeps the institutional panel's fixed navy background and white heading text in %s mode — it is a brand surface, not a theme token that should invert",
    async (_label, theme) => {
      setWindowWidth(1280);

      try {
        await render(
          <PaperProvider theme={theme}>
            <LoginScreen />
          </PaperProvider>,
        );

        const panel = screen.getByTestId("login-institutional-panel");
        const panelBackground = flatStyle(panel).backgroundColor;
        // Scoped to the panel itself: the desktop split also renders the
        // "Vecingest" wordmark in `AuthHeader` (the WEB system's 64px
        // header), so an unscoped query would match both.
        const headingColor = flatStyle(
          within(panel).getByText("Vecingest"),
        ).color;

        // The panel is the WEB design system's institutional brand surface
        // (docs/design/vecingest-web.md), which has no dark mode at all. It
        // must render the same literal colors regardless of the active
        // theme — never `theme.colors.onBackground` / `onPrimary`, which
        // both invert between light and dark (this is the regression this
        // test guards against: the panel once inherited those tokens and
        // rendered as a pale beige slab with illegible text in dark mode).
        expect(panelBackground).toBe(INSTITUTIONAL_PANEL_BACKGROUND);
        expect(headingColor).toBe(INSTITUTIONAL_PANEL_FOREGROUND);

        // Specifically not the dark theme's `onBackground` (pale beige) /
        // `onPrimary` (dark blue) — the exact tokens the regression used.
        expect(panelBackground).not.toBe(darkTheme.colors.onBackground);
        expect(headingColor).not.toBe(darkTheme.colors.onPrimary);
      } finally {
        setWindowWidth(750);
      }
    },
  );
});

describe("LoginScreen — desktop card containment (WEB design system, no dark mode)", () => {
  it.each([
    ["light", lightTheme],
    ["dark", darkTheme],
  ] as const)(
    "renders the 64px header and the contained white card surface in %s mode, never the device theme's surface",
    async (_label, theme) => {
      setWindowWidth(1280);

      try {
        await render(
          <PaperProvider theme={theme}>
            <LoginScreen />
          </PaperProvider>,
        );

        // The header the previous flattened layout was missing entirely.
        expect(screen.getByTestId("auth-header")).toBeTruthy();

        // The card is a genuine containing surface, not a full-bleed panel:
        // it stays the WEB system's fixed white regardless of device theme
        // (never `darkTheme.colors.surface`, which is near-black).
        const cardBackground = flatStyle(
          screen.getByTestId("login-desktop-card"),
        ).backgroundColor;
        expect(cardBackground).toBe(WEB_SURFACE);
        expect(cardBackground).not.toBe(darkTheme.colors.surface);
      } finally {
        setWindowWidth(750);
      }
    },
  );
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
