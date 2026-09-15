import Head from "expo-router/head";
import { useColorScheme } from "react-native";
import { PaperProvider } from "react-native-paper";
import { PortalScreen } from "../src/screens/PortalScreen";
import { pickTheme } from "../src/theme";

/**
 * First post-login destination — see `PortalScreen.tsx`'s header comment
 * for what it renders and why, including its own redirect-to-login guard:
 * that guard (an in-memory access token check, then `GET /v1/me`) lives in
 * `PortalScreen` itself, not here, so it stays exercised by the same tests
 * whether this screen is reached via this route or rendered directly.
 */
export default function Portal() {
  const scheme = useColorScheme();

  return (
    <PaperProvider theme={pickTheme(scheme)}>
      <Head>
        <title>Elegir dónde entrar · Vecingest</title>
      </Head>
      <PortalScreen />
    </PaperProvider>
  );
}
