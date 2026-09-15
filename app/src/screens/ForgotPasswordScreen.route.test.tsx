import { render, screen } from "@testing-library/react-native";
import ForgotPassword from "../../app/(auth)/forgot-password";

/**
 * Smoke test for the real route, mirroring `LoginScreen.route.test.tsx` —
 * see that file's comment for why route tests live here and not under
 * `app/app/(auth)/`.
 */
it("renders the forgot-password route with its primary action", async () => {
  await render(<ForgotPassword />);

  expect(screen.getByRole("button", { name: "Enviar enlace" })).toBeTruthy();
});
