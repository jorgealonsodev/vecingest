import { useLocalSearchParams } from "expo-router";
import { useColorScheme } from "react-native";
import { PaperProvider } from "react-native-paper";
import { ResetPasswordScreen } from "../../src/screens/ResetPasswordScreen";
import { pickTheme } from "../../src/theme";

/**
 * Reset-password route. Same `useColorScheme()` → `pickTheme()` →
 * `PaperProvider` wiring as `login.tsx`. `token` arrives as a deep-link
 * param, e.g. `vecingest://reset-password?token=...` or, on the web export,
 * `/reset-password?token=...`.
 */
export default function ResetPassword() {
  const scheme = useColorScheme();
  const { token } = useLocalSearchParams<{ token?: string }>();

  return (
    <PaperProvider theme={pickTheme(scheme)}>
      <ResetPasswordScreen token={token} />
    </PaperProvider>
  );
}
