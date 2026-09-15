import { useLocalSearchParams } from "expo-router";
import Head from "expo-router/head";
import { useColorScheme } from "react-native";
import { PaperProvider } from "react-native-paper";
import { ResetPasswordScreen } from "../../src/screens/ResetPasswordScreen";
import { pickTheme } from "../../src/theme";

/**
 * Reset-password route. Same `useColorScheme()` → `pickTheme()` →
 * `PaperProvider` wiring as `login.tsx`. `token` arrives as a deep-link
 * param, e.g. `vecingest://reset-password?token=...` or, on the web export,
 * `/reset-password?token=...`.
 *
 * Web `<title>` via `expo-router/head`'s `<Head>` — see `login.tsx`'s
 * comment on why `Stack.Screen options.title` does not work here. Uses the
 * screen's "Nueva contraseña" heading even for the no-token/invalid-link
 * state: the tab identifies the route, not the transient state within it.
 */
export default function ResetPassword() {
  const scheme = useColorScheme();
  const { token } = useLocalSearchParams<{ token?: string }>();

  return (
    <PaperProvider theme={pickTheme(scheme)}>
      <Head>
        <title>Nueva contraseña · Vecingest</title>
      </Head>
      <ResetPasswordScreen token={token} />
    </PaperProvider>
  );
}
