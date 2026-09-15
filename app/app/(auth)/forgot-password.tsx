import { useColorScheme } from "react-native";
import { PaperProvider } from "react-native-paper";
import { ForgotPasswordScreen } from "../../src/screens/ForgotPasswordScreen";
import { pickTheme } from "../../src/theme";

/**
 * Forgot-password route. Same `useColorScheme()` → `pickTheme()` →
 * `PaperProvider` wiring as `login.tsx`.
 */
export default function ForgotPassword() {
  const scheme = useColorScheme();

  return (
    <PaperProvider theme={pickTheme(scheme)}>
      <ForgotPasswordScreen />
    </PaperProvider>
  );
}
