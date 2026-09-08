import { render, screen } from "@testing-library/react-native";
import Login from "./login";

/**
 * Smoke test for the real route: `PaperProvider` + `pickTheme` wiring
 * renders without crashing and exposes the same primary action a device
 * would show. Per-theme visual assertions (accent, copy, sizes) live in
 * `src/screens/LoginScreen.theme.test.tsx` against the same `LoginScreen`
 * this route renders, under each theme explicitly (app-login-ui: Login
 * Screen Light and Dark Mode).
 */
it("renders the login route with its primary action", async () => {
  await render(<Login />);

  expect(screen.getByRole("button", { name: "Iniciar sesión" })).toBeTruthy();
});
