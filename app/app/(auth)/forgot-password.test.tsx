import { render, screen } from "@testing-library/react-native";
import ForgotPassword from "./forgot-password";

/**
 * Smoke test for the real route, mirroring `login.test.tsx`.
 */
it("renders the forgot-password route with its primary action", async () => {
  await render(<ForgotPassword />);

  expect(screen.getByRole("button", { name: "Enviar enlace" })).toBeTruthy();
});
