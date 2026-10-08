import { render, screen } from "@testing-library/react-native";
import Invitation from "../../app/(auth)/invitation";

/**
 * Smoke test for the real route, mirroring `LoginScreen.route.test.tsx` —
 * see that file's comment for why route tests live here and not under
 * `app/app/(auth)/`.
 */
it("renders the invitation route with its code field and primary action", async () => {
  await render(<Invitation />);

  expect(screen.getByTestId("invitation-code-input")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Continuar" })).toBeTruthy();
});
