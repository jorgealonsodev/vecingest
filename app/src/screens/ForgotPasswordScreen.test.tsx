import {
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react-native";
import { PaperProvider } from "react-native-paper";
import { apiClient } from "../auth/api";
import { lightTheme } from "../theme";
import { ForgotPasswordScreen } from "./ForgotPasswordScreen";

jest.mock("../auth/api", () => ({
  apiClient: { POST: jest.fn() },
}));

function renderScreen() {
  return render(
    <PaperProvider theme={lightTheme}>
      <ForgotPasswordScreen />
    </PaperProvider>,
  );
}

describe("ForgotPasswordScreen", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("blocks submission on an invalid email, before any network call", async () => {
    await renderScreen();

    await fireEvent.changeText(
      screen.getByTestId("forgot-password-email"),
      "not-an-email",
    );
    await fireEvent.press(screen.getByTestId("forgot-password-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("forgot-password-email-error")).toBeTruthy();
    });
    expect(apiClient.POST).not.toHaveBeenCalled();
  });

  it("shows the non-committal confirmation on success, never revealing whether the email exists", async () => {
    (apiClient.POST as jest.Mock).mockResolvedValue({
      data: { accepted: true },
      error: undefined,
    });

    await renderScreen();

    await fireEvent.changeText(
      screen.getByTestId("forgot-password-email"),
      "vecino@example.com",
    );
    await fireEvent.press(screen.getByTestId("forgot-password-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("forgot-password-confirmation")).toBeTruthy();
    });
    expect(apiClient.POST).toHaveBeenCalledWith(
      "/v1/auth/forgot-password",
      expect.objectContaining({ body: { email: "vecino@example.com" } }),
    );
  });

  it("maps AUTH_TOO_MANY_ATTEMPTS to the Spanish rate-limit message", async () => {
    (apiClient.POST as jest.Mock).mockResolvedValue({
      data: undefined,
      error: { code: "AUTH_TOO_MANY_ATTEMPTS", message: "" },
    });

    await renderScreen();

    await fireEvent.changeText(
      screen.getByTestId("forgot-password-email"),
      "vecino@example.com",
    );
    await fireEvent.press(screen.getByTestId("forgot-password-submit"));

    await waitFor(() => {
      expect(
        screen.getByTestId("forgot-password-server-error"),
      ).toHaveTextContent(
        "Se ha bloqueado el acceso temporalmente por demasiados intentos.",
      );
    });
  });

  it("falls back to the generic message for an unknown error code", async () => {
    (apiClient.POST as jest.Mock).mockResolvedValue({
      data: undefined,
      error: { code: "SOMETHING_ELSE", message: "" },
    });

    await renderScreen();

    await fireEvent.changeText(
      screen.getByTestId("forgot-password-email"),
      "vecino@example.com",
    );
    await fireEvent.press(screen.getByTestId("forgot-password-submit"));

    await waitFor(() => {
      expect(
        screen.getByTestId("forgot-password-server-error"),
      ).toHaveTextContent(
        "No se ha podido procesar la solicitud. Inténtalo de nuevo.",
      );
    });
  });
});
