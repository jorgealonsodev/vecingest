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
jest.mock("expo-router", () => ({
  ...jest.requireActual("expo-router"),
  useRouter: () => ({ replace: mockReplace }),
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

    const invitationLink = screen.getByTestId("portal-invitation-link");
    expect(invitationLink.props.accessibilityState?.disabled).toBe(true);
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
