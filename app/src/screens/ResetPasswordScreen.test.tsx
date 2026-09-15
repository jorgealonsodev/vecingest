import {
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react-native";
import { PaperProvider } from "react-native-paper";
import { apiClient } from "../auth/api";
import { lightTheme } from "../theme";
import { ResetPasswordScreen } from "./ResetPasswordScreen";

jest.mock("../auth/api", () => ({
  apiClient: { POST: jest.fn() },
}));

function renderScreen(token?: string) {
  return render(
    <PaperProvider theme={lightTheme}>
      <ResetPasswordScreen token={token} />
    </PaperProvider>,
  );
}

describe("ResetPasswordScreen", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("renders an invalid-link state and never calls the API when the token param is missing", async () => {
    await renderScreen(undefined);

    expect(screen.getByTestId("reset-password-invalid-link")).toBeTruthy();
    expect(screen.queryByTestId("reset-password-new-password")).toBeNull();
    expect(apiClient.POST).not.toHaveBeenCalled();
  });

  it("blocks submission when the passwords do not match, before any network call", async () => {
    await renderScreen("a-valid-reset-token");

    await fireEvent.changeText(
      screen.getByTestId("reset-password-new-password"),
      "a-strong-password",
    );
    await fireEvent.changeText(
      screen.getByTestId("reset-password-confirm-password"),
      "a-different-password",
    );
    await fireEvent.press(screen.getByTestId("reset-password-submit"));

    await waitFor(() => {
      expect(
        screen.getByTestId("reset-password-confirm-password-error"),
      ).toHaveTextContent("Las contraseñas no coinciden.");
    });
    expect(apiClient.POST).not.toHaveBeenCalled();
  });

  it("submits token + new_password only (never confirm_password) and shows the success state", async () => {
    (apiClient.POST as jest.Mock).mockResolvedValue({
      data: { accepted: true },
      error: undefined,
    });

    await renderScreen("a-valid-reset-token");

    await fireEvent.changeText(
      screen.getByTestId("reset-password-new-password"),
      "a-strong-password",
    );
    await fireEvent.changeText(
      screen.getByTestId("reset-password-confirm-password"),
      "a-strong-password",
    );
    await fireEvent.press(screen.getByTestId("reset-password-submit"));

    await waitFor(() => {
      expect(screen.getByTestId("reset-password-success")).toBeTruthy();
    });
    expect(apiClient.POST).toHaveBeenCalledWith(
      "/v1/auth/reset-password",
      expect.objectContaining({
        body: {
          token: "a-valid-reset-token",
          new_password: "a-strong-password",
        },
      }),
    );
  });

  it("maps AUTH_RESET_TOKEN_INVALID to the Spanish expired-link message", async () => {
    (apiClient.POST as jest.Mock).mockResolvedValue({
      data: undefined,
      error: { code: "AUTH_RESET_TOKEN_INVALID", message: "" },
    });

    await renderScreen("a-valid-reset-token");

    await fireEvent.changeText(
      screen.getByTestId("reset-password-new-password"),
      "a-strong-password",
    );
    await fireEvent.changeText(
      screen.getByTestId("reset-password-confirm-password"),
      "a-strong-password",
    );
    await fireEvent.press(screen.getByTestId("reset-password-submit"));

    await waitFor(() => {
      expect(
        screen.getByTestId("reset-password-server-error"),
      ).toHaveTextContent(
        "El enlace no es válido o ha caducado. Solicita uno nuevo.",
      );
    });
  });

  it("maps AUTH_PASSWORD_BREACHED to the Spanish leaked-password message", async () => {
    (apiClient.POST as jest.Mock).mockResolvedValue({
      data: undefined,
      error: { code: "AUTH_PASSWORD_BREACHED", message: "" },
    });

    await renderScreen("a-valid-reset-token");

    await fireEvent.changeText(
      screen.getByTestId("reset-password-new-password"),
      "a-strong-password",
    );
    await fireEvent.changeText(
      screen.getByTestId("reset-password-confirm-password"),
      "a-strong-password",
    );
    await fireEvent.press(screen.getByTestId("reset-password-submit"));

    await waitFor(() => {
      expect(
        screen.getByTestId("reset-password-server-error"),
      ).toHaveTextContent(
        "Esta contraseña aparece en filtraciones conocidas. Elige otra.",
      );
    });
  });
});
