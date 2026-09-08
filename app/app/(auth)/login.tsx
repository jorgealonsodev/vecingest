import { useColorScheme } from "react-native";
import { PaperProvider } from "react-native-paper";
import { pickTheme } from "../../src/theme";
import { LoginScreen } from "../../src/screens/LoginScreen";

/**
 * Login route. PaperProvider theme wiring lives here, switched by
 * `useColorScheme()` — both `vecingest-light.md` and `vecingest-dark.md`
 * ship at M0 because both design systems already exist and the app follows
 * the device theme (design.md "Expo login screen"). `pickTheme` carries the
 * actual light/dark selection so it is unit-testable without mocking the
 * native `Appearance` module (see `src/theme/pickTheme.test.ts`).
 */
export default function Login() {
  const scheme = useColorScheme();

  return (
    <PaperProvider theme={pickTheme(scheme)}>
      <LoginScreen />
    </PaperProvider>
  );
}
