import Head from "expo-router/head";
import { useColorScheme } from "react-native";
import { PaperProvider } from "react-native-paper";
import { InvitationScreen } from "../../src/screens/InvitationScreen";
import { pickTheme } from "../../src/theme";

/**
 * Invitation-code route, reached from `login-invitation-link` and
 * `portal-invitation-link`. Same `useColorScheme()` → `pickTheme()` →
 * `PaperProvider` wiring and `<Head>` title as `login.tsx`. It takes no
 * route params on purpose: the short code is a credential and must never
 * travel in a URL (see `InvitationScreen`'s header comment).
 */
export default function Invitation() {
  const scheme = useColorScheme();

  return (
    <PaperProvider theme={pickTheme(scheme)}>
      <Head>
        <title>Código de invitación · Vecingest</title>
      </Head>
      <InvitationScreen />
    </PaperProvider>
  );
}
