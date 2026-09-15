import { render, screen } from "@testing-library/react-native";

jest.mock("expo-router", () => ({
  ...jest.requireActual("expo-router"),
  useLocalSearchParams: () => ({ token: "a-valid-reset-token" }),
}));

import ResetPassword from "../../app/(auth)/reset-password";

/**
 * Smoke test for the real route, mirroring `LoginScreen.route.test.tsx` —
 * see that file's comment for why route tests live here and not under
 * `app/app/(auth)/`. `token` is mocked here because this is a deep-link
 * param the route reads from `useLocalSearchParams` — the missing-token
 * state is covered by `ResetPasswordScreen.test.tsx`.
 */
it("renders the reset-password route with its primary action when a token is present", async () => {
  await render(<ResetPassword />);

  expect(
    screen.getByRole("button", { name: "Guardar contraseña" }),
  ).toBeTruthy();
});
