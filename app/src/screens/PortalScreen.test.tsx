import {
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react-native";
import { PaperProvider } from "react-native-paper";
import { apiClient } from "../auth/api";
import { clearRefreshToken } from "../auth/secureTokens";
import { clearSession, getSession, setSession } from "../auth/session";
import { lightTheme } from "../theme";
import { PortalScreen } from "./PortalScreen";

jest.mock("../auth/api", () => ({
  apiClient: { GET: jest.fn(), POST: jest.fn() },
}));

jest.mock("../auth/secureTokens", () => ({
  clearRefreshToken: jest.fn().mockResolvedValue(undefined),
}));

const mockReplace = jest.fn();
const mockPush = jest.fn();
jest.mock("expo-router", () => ({
  ...jest.requireActual("expo-router"),
  useRouter: () => ({ replace: mockReplace, push: mockPush }),
}));

function renderPortalScreen() {
  return render(
    <PaperProvider theme={lightTheme}>
      <PortalScreen />
    </PaperProvider>,
  );
}

describe("PortalScreen — membership rows (GET /v1/me memberships)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearSession();
    setSession({ accessToken: "a-valid-access-token", csrfToken: null });
  });

  it("renders exactly one selectable row for a single community membership", async () => {
    (apiClient.GET as jest.Mock).mockResolvedValue({
      data: {
        id: "user-1",
        email: "vecino@example.com",
        is_superadmin: false,
        memberships: [
          {
            scope: "community",
            id: "community-1",
            name: "C/ Mayor 12, Irun",
            role: "owner",
          },
        ],
      },
      error: undefined,
    });

    await renderPortalScreen();

    await waitFor(() => {
      expect(screen.getAllByTestId(/^portal-membership-/)).toHaveLength(1);
    });

    const row = screen.getByTestId("portal-membership-community-community-1");
    expect(row.props.accessibilityRole).toBe("radio");
    // A single membership leaves no choice to make, so it is preselected.
    expect(row.props.accessibilityState?.selected).toBe(true);
    expect(screen.getByText("C/ Mayor 12, Irun")).toBeTruthy();
    expect(screen.getByText("Propietario")).toBeTruthy();
    expect(screen.queryByTestId("portal-empty-state")).toBeNull();
  });

  it("keeps the unchanged empty state when the memberships array is empty", async () => {
    (apiClient.GET as jest.Mock).mockResolvedValue({
      data: {
        id: "user-1",
        email: "vecino@example.com",
        is_superadmin: false,
        memberships: [],
      },
      error: undefined,
    });

    await renderPortalScreen();

    await waitFor(() => {
      expect(screen.getByTestId("portal-empty-state")).toBeTruthy();
    });

    expect(
      screen.getByText("Todavía no perteneces a ninguna comunidad ni empresa."),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Un administrador debe crear una comunidad e invitarte para que aparezca aquí.",
      ),
    ).toBeTruthy();
    expect(screen.queryByTestId("portal-memberships")).toBeNull();
    expect(screen.queryAllByTestId(/^portal-membership-/)).toHaveLength(0);
    expect(
      screen.getByTestId("portal-primary-cta").props.accessibilityState
        ?.disabled,
    ).toBe(true);
  });
});

describe("PortalScreen — context selector for multiple memberships", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearSession();
    setSession({ accessToken: "a-valid-access-token", csrfToken: null });
    (apiClient.GET as jest.Mock).mockResolvedValue({
      data: {
        id: "user-1",
        email: "vecino@example.com",
        is_superadmin: false,
        memberships: [
          {
            scope: "community",
            id: "community-1",
            name: "C/ Mayor 12, Irun",
            role: "owner",
          },
          {
            scope: "community",
            id: "community-2",
            name: "Avda. Navarra 4, Hondarribia",
            role: "tenant",
          },
          {
            scope: "office",
            id: "office-1",
            name: "Fincas Bidasoa",
            role: "admin_staff",
          },
        ],
      },
      error: undefined,
    });
  });

  it("lists every membership with its role", async () => {
    await renderPortalScreen();

    await waitFor(() => {
      expect(screen.getByTestId("portal-memberships")).toBeTruthy();
    });

    const rows = screen.getAllByTestId(/^portal-membership-/);
    expect(rows).toHaveLength(3);
    expect(
      screen.getByTestId("portal-membership-community-community-1").props
        .accessibilityLabel,
    ).toBe("Comunidad C/ Mayor 12, Irun, Propietario");
    expect(
      screen.getByTestId("portal-membership-community-community-2").props
        .accessibilityLabel,
    ).toBe("Comunidad Avda. Navarra 4, Hondarribia, Inquilino");
    expect(
      screen.getByTestId("portal-membership-office-office-1").props
        .accessibilityLabel,
    ).toBe("Despacho Fincas Bidasoa, Personal del despacho");
    expect(screen.getByText("Propietario")).toBeTruthy();
    expect(screen.getByText("Inquilino")).toBeTruthy();
    expect(screen.getByText("Personal del despacho")).toBeTruthy();
  });

  it("lets the user pick exactly one context", async () => {
    await renderPortalScreen();

    await waitFor(() => {
      expect(screen.getByTestId("portal-memberships")).toBeTruthy();
    });

    const selected = (testID: string) =>
      screen.getByTestId(testID).props.accessibilityState?.selected;

    // Nothing is preselected when there is a real choice to make.
    expect(selected("portal-membership-community-community-1")).toBe(false);
    expect(selected("portal-membership-community-community-2")).toBe(false);
    expect(selected("portal-membership-office-office-1")).toBe(false);

    await fireEvent.press(
      screen.getByTestId("portal-membership-community-community-2"),
    );

    expect(selected("portal-membership-community-community-1")).toBe(false);
    expect(selected("portal-membership-community-community-2")).toBe(true);
    expect(selected("portal-membership-office-office-1")).toBe(false);

    await fireEvent.press(
      screen.getByTestId("portal-membership-office-office-1"),
    );

    expect(selected("portal-membership-community-community-2")).toBe(false);
    expect(selected("portal-membership-office-office-1")).toBe(true);
  });
});

describe("PortalScreen — proving the session works (GET /v1/me)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearSession();
  });

  it("redirects instead of rendering when there is no in-memory access token", async () => {
    // No setSession() call — a fresh page load / cold start has no token.
    await renderPortalScreen();

    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalledWith("/(auth)/login");
    });

    expect(apiClient.GET).not.toHaveBeenCalled();
    expect(screen.queryByTestId("portal-screen")).toBeNull();
    expect(screen.queryByTestId("portal-greeting")).toBeNull();
  });

  it("greets the authenticated user with the real GET /v1/me response", async () => {
    setSession({ accessToken: "a-valid-access-token", csrfToken: null });
    (apiClient.GET as jest.Mock).mockResolvedValue({
      data: {
        id: "user-1",
        email: "jorge@vecingest.xdev.es",
        is_superadmin: true,
        memberships: [],
      },
      error: undefined,
    });

    await renderPortalScreen();

    await waitFor(() => {
      expect(screen.getByTestId("portal-greeting")).toBeTruthy();
    });

    expect(apiClient.GET).toHaveBeenCalledWith(
      "/v1/me",
      expect.objectContaining({
        headers: { Authorization: "Bearer a-valid-access-token" },
      }),
    );
    expect(
      screen.getByText("Conectado como jorge@vecingest.xdev.es"),
    ).toBeTruthy();
    expect(screen.getByTestId("portal-superadmin-badge")).toBeTruthy();
  });

  it("renders the empty state honestly instead of a fabricated portal list", async () => {
    setSession({ accessToken: "a-valid-access-token", csrfToken: null });
    (apiClient.GET as jest.Mock).mockResolvedValue({
      data: {
        id: "user-1",
        email: "vecino@example.com",
        is_superadmin: false,
        memberships: [],
      },
      error: undefined,
    });

    await renderPortalScreen();

    await waitFor(() => {
      expect(screen.getByTestId("portal-empty-state")).toBeTruthy();
    });

    expect(screen.queryByTestId("portal-superadmin-badge")).toBeNull();

    const primaryCta = screen.getByTestId("portal-primary-cta");
    expect(primaryCta.props.accessibilityState?.disabled).toBe(true);

    // Invitation-Code Entry Point Enabled: no longer "Próximamente".
    const invitationLink = screen.getByTestId("portal-invitation-link");
    expect(invitationLink.props.accessibilityState?.disabled).toBe(false);
    expect(screen.queryByText("Próximamente")).toBeNull();
    await fireEvent.press(invitationLink);
    expect(mockPush).toHaveBeenCalledWith("/(auth)/invitation");
  });

  it("shows a retry affordance when GET /v1/me fails for a reason other than an expired session", async () => {
    setSession({ accessToken: "a-valid-access-token", csrfToken: null });
    (apiClient.GET as jest.Mock).mockResolvedValue({
      data: undefined,
      error: { code: "INTERNAL_ERROR", message: "boom" },
    });

    await renderPortalScreen();

    await waitFor(() => {
      expect(screen.getByTestId("portal-error")).toBeTruthy();
    });

    (apiClient.GET as jest.Mock).mockResolvedValue({
      data: {
        id: "user-1",
        email: "vecino@example.com",
        is_superadmin: false,
        memberships: [],
      },
      error: undefined,
    });

    await fireEvent.press(screen.getByTestId("portal-retry"));

    await waitFor(() => {
      expect(screen.getByTestId("portal-empty-state")).toBeTruthy();
    });
  });

  it("clears the session and redirects when GET /v1/me reports an expired session", async () => {
    setSession({ accessToken: "a-stale-access-token", csrfToken: null });
    (apiClient.GET as jest.Mock).mockResolvedValue({
      data: undefined,
      error: { code: "AUTH_UNAUTHORIZED", message: "expired" },
    });

    await renderPortalScreen();

    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalledWith("/(auth)/login");
    });

    expect(screen.queryByTestId("portal-screen")).toBeNull();
    expect(getSession().accessToken).toBeNull();
  });

  it("signs out through POST /v1/auth/logout, clears local state, and returns to login", async () => {
    setSession({ accessToken: "a-valid-access-token", csrfToken: "csrf" });
    (apiClient.GET as jest.Mock).mockResolvedValue({
      data: {
        id: "user-1",
        email: "vecino@example.com",
        is_superadmin: false,
        memberships: [],
      },
      error: undefined,
    });
    (apiClient.POST as jest.Mock).mockResolvedValue({
      data: undefined,
      error: undefined,
    });

    await renderPortalScreen();

    await waitFor(() => {
      expect(screen.getByTestId("portal-sign-out")).toBeTruthy();
    });

    await fireEvent.press(screen.getByTestId("portal-sign-out"));

    await waitFor(() => {
      expect(apiClient.POST).toHaveBeenCalledWith(
        "/v1/auth/logout",
        expect.objectContaining({
          headers: { Authorization: "Bearer a-valid-access-token" },
        }),
      );
    });

    expect(getSession()).toEqual({ accessToken: null, csrfToken: null });
    expect(clearRefreshToken).toHaveBeenCalled();
    expect(mockReplace).toHaveBeenCalledWith("/(auth)/login");
  });

  it("still clears the session and navigates to login when clearRefreshToken rejects (web has no expo-secure-store)", async () => {
    // `expo-secure-store`'s web module (`ExpoSecureStore.web.ts`) exports
    // `{}`, so `clearRefreshToken` throws there — sign-out must not get
    // stuck behind that native-only cleanup step.
    (clearRefreshToken as jest.Mock).mockRejectedValueOnce(
      new Error("ExpoSecureStore.deleteValueWithKeyAsync is not a function"),
    );
    setSession({ accessToken: "a-valid-access-token", csrfToken: "csrf" });
    (apiClient.GET as jest.Mock).mockResolvedValue({
      data: {
        id: "user-1",
        email: "vecino@example.com",
        is_superadmin: false,
        memberships: [],
      },
      error: undefined,
    });
    (apiClient.POST as jest.Mock).mockResolvedValue({
      data: undefined,
      error: undefined,
    });

    await renderPortalScreen();

    await waitFor(() => {
      expect(screen.getByTestId("portal-sign-out")).toBeTruthy();
    });

    await fireEvent.press(screen.getByTestId("portal-sign-out"));

    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalledWith("/(auth)/login");
    });

    expect(getSession()).toEqual({ accessToken: null, csrfToken: null });
  });
});
