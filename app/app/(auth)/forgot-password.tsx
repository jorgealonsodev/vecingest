import Head from "expo-router/head";
import { useColorScheme } from "react-native";
import { PaperProvider } from "react-native-paper";
import { ForgotPasswordScreen } from "../../src/screens/ForgotPasswordScreen";
import { pickTheme } from "../../src/theme";

/**
 * Forgot-password route. Same `useColorScheme()` → `pickTheme()` →
 * `PaperProvider` wiring as `login.tsx`.
 *
 * Web `<title>` via `expo-router/head`'s `<Head>` — see `login.tsx`'s
 * comment on why `Stack.Screen options.title` does not work here.
 */
export default function ForgotPassword() {
  const scheme = useColorScheme();

  return (
    <PaperProvider theme={pickTheme(scheme)}>
      <Head>
        <title>Recuperar contraseña · Vecingest</title>
      </Head>
      <ForgotPasswordScreen />
    </PaperProvider>
  );
}
