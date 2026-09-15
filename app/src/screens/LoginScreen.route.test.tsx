import { render, screen } from "@testing-library/react-native";
import Login from "../../app/(auth)/login";

/**
 * Smoke test for the real route: `PaperProvider` + `pickTheme` wiring
 * renders without crashing and exposes the same primary action a device
 * would show. Per-theme visual assertions (accent, copy, sizes) live in
 * `LoginScreen.theme.test.tsx` against the same `LoginScreen` this route
 * renders, under each theme explicitly (app-login-ui: Login Screen Light and
 * Dark Mode).
 *
 * Lives alongside the screen-level tests, not under `app/app/(auth)/`,
 * because Expo Router turns every file under `app/app/` into a route with
 * no automatic `*.test.tsx` exclusion in this version — a route test file
 * there ships as a real route (confirmed via `expo export -p web`).
 */
it("renders the login route with its primary action", async () => {
  await render(<Login />);

  expect(screen.getByRole("button", { name: "Entrar" })).toBeTruthy();
});
