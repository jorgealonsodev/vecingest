import {
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react-native";
import { PaperProvider } from "react-native-paper";
import { apiClient } from "../auth/api";
import { persistRefreshToken } from "../auth/secureTokens";
import { clearSession, getSession, setSession } from "../auth/session";
import { lightTheme } from "../theme";
import { InvitationScreen } from "./InvitationScreen";

jest.mock("../auth/api", () => ({
  apiClient: { POST: jest.fn() },
}));

jest.mock("../auth/secureTokens", () => ({
  persistRefreshToken: jest.fn().mockResolvedValue(undefined),
}));

const mockReplace = jest.fn();
const mockBack = jest.fn();
jest.mock("expo-router", () => ({
  ...jest.requireActual("expo-router"),
  useRouter: () => ({
    replace: mockReplace,
    back: mockBack,
    canGoBack: () => true,
  }),
}));

const PREVIEW = {
  community_id: "community-1",
  community_name: "C/ Mayor 12, Irun",
  role: "owner",
  unit_id: "unit-1",
  expires_at: "2026-10-20T10:00:00Z",
};

function renderScreen() {
  return render(
    <PaperProvider theme={lightTheme}>
      <InvitationScreen />
    </PaperProvider>,
  );
}

function mockPostResponses(
  responses: Record<string, Array<{ data?: unknown; error?: unknown }>>,
) {
  (apiClient.POST as jest.Mock).mockImplementation((path: string) => {
    const queue = responses[path];
    if (!queue || queue.length === 0) {
      throw new Error(`unexpected POST ${path}`);
    }
    return Promise.resolve(queue.shift());
  });
}

async function enterCodeAndContinue(code: string) {
  await fireEvent.changeText(screen.getByTestId("invitation-code-input"), code);
  await fireEvent.press(screen.getByTestId("invitation-code-submit"));
}

async function reachAcceptForm() {
  await enterCodeAndContinue("ab12cd34");
  await waitFor(() => {
    expect(screen.getByTestId("invitation-preview")).toBeTruthy();
  });
  await fireEvent.press(screen.getByTestId("invitation-preview-continue"));
  await fireEvent.changeText(
    screen.getByTestId("invitation-accept-name"),
    "Ane Etxeberria",
  );
  await fireEvent.changeText(
    screen.getByTestId("invitation-accept-password"),
    "a-strong-password",
  );
  await fireEvent.press(screen.getByTestId("invitation-accept-consent"));
}

describe("InvitationScreen — code entry and preview (Invitation-Code Entry Point Enabled)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearSession();
  });

  it("keeps Continuar disabled until the code has 8 alphanumeric characters, uppercased", async () => {
    await renderScreen();

    const input = screen.getByTestId("invitation-code-input");
    await fireEvent.changeText(input, "ab-12 cd");
    expect(
      screen.getByTestId("invitation-code-submit").props.accessibilityState
        ?.disabled,
    ).toBe(true);

    await fireEvent.changeText(input, "ab-12 cd34");
    expect(screen.getByTestId("invitation-code-input").props.value).toBe(
      "AB12CD34",
    );
    expect(
      screen.getByTestId("invitation-code-submit").props.accessibilityState
        ?.disabled,
    ).toBe(false);
  });

  it("shows the preview for a valid code before any account action", async () => {
    mockPostResponses({ "/v1/invitations/preview": [{ data: PREVIEW }] });

    await renderScreen();
    await enterCodeAndContinue("ab12cd34");

    await waitFor(() => {
      expect(screen.getByTestId("invitation-preview")).toBeTruthy();
    });
    expect(apiClient.POST).toHaveBeenCalledTimes(1);
    expect(apiClient.POST).toHaveBeenCalledWith("/v1/invitations/preview", {
      body: { short_code: "AB12CD34" },
    });
    expect(screen.getByText("C/ Mayor 12, Irun")).toBeTruthy();
    expect(screen.getByText("Propietario")).toBeTruthy();
    expect(screen.getByText("Válida hasta el 20/10/2026")).toBeTruthy();
    // Preview first: no account form, no accept call yet.
    expect(screen.queryByTestId("invitation-accept-form")).toBeNull();
  });

  it("shows one generic error for an invalid or expired code", async () => {
    mockPostResponses({
      "/v1/invitations/preview": [
        { error: { code: "NOT_FOUND", message: "invitation not found" } },
      ],
    });

    await renderScreen();
    await enterCodeAndContinue("ZZZZ9999");

    await waitFor(() => {
      expect(screen.getByTestId("invitation-code-error")).toHaveTextContent(
        "El código no es válido o ha caducado.",
      );
    });
    expect(screen.queryByTestId("invitation-preview")).toBeNull();
  });

  it("'No soy yo' backs out without any accept call", async () => {
    mockPostResponses({ "/v1/invitations/preview": [{ data: PREVIEW }] });

    await renderScreen();
    await enterCodeAndContinue("ab12cd34");
    await waitFor(() => {
      expect(screen.getByTestId("invitation-preview")).toBeTruthy();
    });

    await fireEvent.press(screen.getByTestId("invitation-preview-not-me"));

    expect(mockBack).toHaveBeenCalled();
    expect(apiClient.POST).toHaveBeenCalledTimes(1);
  });
});

describe("InvitationScreen — accept (POST /v1/auth/accept-invitation)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearSession();
  });

  it("replaces the current session with the returned one and lands on the portal", async () => {
    setSession({ accessToken: "old-access-token", csrfToken: "old-csrf" });
    mockPostResponses({
      "/v1/invitations/preview": [{ data: PREVIEW }],
      "/v1/auth/accept-invitation": [
        {
          data: {
            access_token: "new-access-token",
            refresh_token: "new-refresh-token",
            expires_in: 900,
          },
        },
      ],
    });

    await renderScreen();
    await reachAcceptForm();
    await fireEvent.press(screen.getByTestId("invitation-accept-submit"));

    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalledWith("/portal");
    });
    expect(apiClient.POST).toHaveBeenLastCalledWith(
      "/v1/auth/accept-invitation",
      {
        body: {
          short_code: "AB12CD34",
          name: "Ane Etxeberria",
          password: "a-strong-password",
          consent: true,
          platform: expect.stringMatching(/^(ios|android|web)$/),
        },
      },
    );
    expect(getSession()).toEqual({
      accessToken: "new-access-token",
      csrfToken: null,
    });
    expect(persistRefreshToken).toHaveBeenCalledWith("new-refresh-token");
  });

  it("still lands on the portal when persisting the refresh token fails after a successful accept", async () => {
    jest
      .mocked(persistRefreshToken)
      .mockRejectedValueOnce(new Error("keychain unavailable"));
    mockPostResponses({
      "/v1/invitations/preview": [{ data: PREVIEW }],
      "/v1/auth/accept-invitation": [
        {
          data: {
            access_token: "new-access-token",
            refresh_token: "new-refresh-token",
            expires_in: 900,
          },
        },
      ],
    });

    await renderScreen();
    await reachAcceptForm();
    await fireEvent.press(screen.getByTestId("invitation-accept-submit"));

    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalledWith("/portal");
    });
    expect(getSession()).toEqual({
      accessToken: "new-access-token",
      csrfToken: null,
    });
  });

  it("asks for the TOTP code on AUTH_MFA_REQUIRED and retries with it", async () => {
    mockPostResponses({
      "/v1/invitations/preview": [{ data: PREVIEW }],
      "/v1/auth/accept-invitation": [
        { error: { code: "AUTH_MFA_REQUIRED", message: "" } },
        { data: { access_token: "new-access-token", expires_in: 900 } },
      ],
    });

    await renderScreen();
    await reachAcceptForm();
    expect(screen.queryByTestId("invitation-accept-totp")).toBeNull();
    await fireEvent.press(screen.getByTestId("invitation-accept-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("invitation-accept-totp")).toBeTruthy();
    });
    expect(mockReplace).not.toHaveBeenCalled();

    await fireEvent.changeText(
      screen.getByTestId("invitation-accept-totp"),
      "123456",
    );
    await fireEvent.press(screen.getByTestId("invitation-accept-submit"));

    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalledWith("/portal");
    });
    expect(apiClient.POST).toHaveBeenLastCalledWith(
      "/v1/auth/accept-invitation",
      {
        body: expect.objectContaining({ totp_code: "123456" }),
      },
    );
  });

  it("shows one generic error and keeps the current session on any other failure", async () => {
    setSession({ accessToken: "old-access-token", csrfToken: "old-csrf" });
    mockPostResponses({
      "/v1/invitations/preview": [{ data: PREVIEW }],
      "/v1/auth/accept-invitation": [
        { error: { code: "AUTH_INVALID_CREDENTIALS", message: "" } },
      ],
    });

    await renderScreen();
    await reachAcceptForm();
    await fireEvent.press(screen.getByTestId("invitation-accept-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("invitation-accept-error")).toHaveTextContent(
        "No se ha podido aceptar la invitación. Revisa los datos e inténtalo de nuevo.",
      );
    });
    expect(mockReplace).not.toHaveBeenCalled();
    expect(getSession()).toEqual({
      accessToken: "old-access-token",
      csrfToken: "old-csrf",
    });
  });

  it("blocks submission without consent, before any accept call", async () => {
    mockPostResponses({ "/v1/invitations/preview": [{ data: PREVIEW }] });

    await renderScreen();
    await reachAcceptForm();
    // Untick the consent reachAcceptForm ticked.
    await fireEvent.press(screen.getByTestId("invitation-accept-consent"));
    await fireEvent.press(screen.getByTestId("invitation-accept-submit"));

    await waitFor(() => {
      expect(
        screen.getByTestId("invitation-accept-consent-error"),
      ).toBeTruthy();
    });
    expect(apiClient.POST).toHaveBeenCalledTimes(1);
  });
});
