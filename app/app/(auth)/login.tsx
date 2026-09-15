import Head from "expo-router/head";
import { useColorScheme } from "react-native";
import { PaperProvider } from "react-native-paper";
import { LoginScreen } from "../../src/screens/LoginScreen";
import { pickTheme } from "../../src/theme";

/**
 * Login route. PaperProvider theme wiring lives here, switched by
 * `useColorScheme()` — both `vecingest-light.md` and `vecingest-dark.md`
 * ship at M0 because both design systems already exist and the app follows
 * the device theme (design.md "Expo login screen"). `pickTheme` carries the
 * actual light/dark selection so it is unit-testable without mocking the
 * native `Appearance` module (see `src/theme/pickTheme.test.ts`).
 *
 * The web document `<title>` is set with `expo-router/head`'s `<Head>`
 * (react-helmet-async under the hood), NOT `Stack.Screen options.title`:
 * `ExpoRoot` hardcodes `documentTitle={{ enabled: false }}` on its
 * `NavigationContainer`, which disables React Navigation's own
 * options-to-title wiring on web, so that path silently no-ops. Spanish
 * sentence case, the screen's own heading, the product name so the tab is
 * identifiable among others (`docs/design/README.md` copy conventions).
 */
export default function Login() {
  const scheme = useColorScheme();

  return (
    <PaperProvider theme={pickTheme(scheme)}>
      <Head>
        <title>Acceso a la plataforma · Vecingest</title>
      </Head>
      <LoginScreen />
    </PaperProvider>
  );
}
