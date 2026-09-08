import * as SecureStore from "expo-secure-store";
import { fireEvent, render, screen, waitFor } from "@testing-library/react-native";
import { PaperProvider } from "react-native-paper";
import { LoginScreen } from "./LoginScreen";
import { lightTheme } from "../theme";
import { apiClient } from "../auth/api";
import { getSession, clearSession } from "../auth/session";

// `require` here is Jest's real Node CommonJS global; the app has no
// `@types/node` dependency (React Native's own ambient types are enough for
// shipped code), so it is typed loosely for these two structural checks only.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
declare const require: { resolve: (id: string) => string } & ((id: string) => any);

jest.mock("../auth/api", () => ({
  apiClient: { POST: jest.fn() },
}));

function renderLoginScreen() {
  return render(
    <PaperProvider theme={lightTheme}>
      <LoginScreen />
    </PaperProvider>,
  );
}

describe("LoginScreen — generated-schema validation (app-login-ui: Generated-Schema Form Validation)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearSession();
  });

  it("blocks submission on an empty password, using the generated schema, before any network call", async () => {
    await renderLoginScreen();

    await fireEvent.changeText(screen.getByTestId("login-email"), "vecino@example.com");
    // password left empty — schemas.LoginRequest requires password.min(12)
    await fireEvent.press(screen.getByTestId("login-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("login-password-error")).toBeTruthy();
    });

    expect(apiClient.POST).not.toHaveBeenCalled();
  });

  it("declares no hand-written validation schema — it resolves against schemas.LoginRequest", () => {
    // eslint-disable-next-line @typescript-eslint/no-var-requires
    const source: string = require("fs").readFileSync(
      require.resolve("./LoginScreen.tsx"),
      "utf8",
    );
    expect(source).toMatch(/resolver:\s*zodResolver\(schemas\.LoginRequest\)/);
    expect(source).not.toMatch(/z\.object\(/);
  });
});

describe("LoginScreen — secure token storage (app-login-ui: Secure Token Storage)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearSession();
  });

  it("persists the refresh token through expo-secure-store on a successful login, never AsyncStorage", async () => {
    (apiClient.POST as jest.Mock).mockResolvedValue({
      data: {
        access_token: "access-token-value",
        csrf_token: "csrf-token-value",
        expires_in: 900,
        refresh_token: "refresh-token-value",
      },
      error: undefined,
    });

    await renderLoginScreen();

    await fireEvent.changeText(screen.getByTestId("login-email"), "vecino@example.com");
    await fireEvent.changeText(screen.getByTestId("login-password"), "a-strong-password");

    await fireEvent.press(screen.getByTestId("login-submit"));

    await waitFor(() => {
      expect(SecureStore.setItemAsync).toHaveBeenCalledWith(
        "vecingest.refresh_token",
        "refresh-token-value",
      );
    });

    // Access token and CSRF token stay in memory only — never written to
    // expo-secure-store.
    expect(SecureStore.setItemAsync).not.toHaveBeenCalledWith(
      expect.anything(),
      "access-token-value",
    );
    expect(getSession()).toEqual({
      accessToken: "access-token-value",
      csrfToken: "csrf-token-value",
    });

    // The app never depends on AsyncStorage for token persistence.
    expect(() => require.resolve("@react-native-async-storage/async-storage")).toThrow();
  });
});
