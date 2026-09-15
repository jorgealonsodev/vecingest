import { render, screen, waitFor } from "@testing-library/react-native";
import Portal from "../../app/portal";
import { apiClient } from "../auth/api";
import { clearSession, setSession } from "../auth/session";

/**
 * Smoke test for the real route, mirroring `LoginScreen.route.test.tsx` —
 * see that file's comment for why route tests live here and not under
 * `app/app/`.
 */
jest.mock("../auth/api", () => ({
  apiClient: { GET: jest.fn(), POST: jest.fn() },
}));

const mockReplace = jest.fn();
jest.mock("expo-router", () => ({
  ...jest.requireActual("expo-router"),
  useRouter: () => ({ replace: mockReplace }),
}));

describe("Portal route", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearSession();
  });

  it("renders the authenticated portal for a real session", async () => {
    setSession({ accessToken: "a-valid-access-token", csrfToken: null });
    (apiClient.GET as jest.Mock).mockResolvedValue({
      data: { id: "user-1", email: "vecino@example.com", is_superadmin: false },
      error: undefined,
    });

    await render(<Portal />);

    await waitFor(() => {
      expect(screen.getByTestId("portal-screen")).toBeTruthy();
    });
    expect(screen.getByText("Conectado como vecino@example.com")).toBeTruthy();
  });

  it("redirects to /(auth)/login instead of rendering blank when reached with no session", async () => {
    // No setSession() call — same as a fresh page load or a deep link.
    await render(<Portal />);

    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalledWith("/(auth)/login");
    });

    expect(apiClient.GET).not.toHaveBeenCalled();
    expect(screen.queryByTestId("portal-screen")).toBeNull();
  });
});
