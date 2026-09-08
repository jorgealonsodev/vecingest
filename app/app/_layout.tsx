import { useEffect } from "react";
import {
  BricolageGrotesque_600SemiBold,
  useFonts as useBricolageFonts,
} from "@expo-google-fonts/bricolage-grotesque";
import {
  IBMPlexSans_400Regular,
  IBMPlexSans_500Medium,
  useFonts as useIBMPlexFonts,
} from "@expo-google-fonts/ibm-plex-sans";
import * as SplashScreen from "expo-splash-screen";
import { Stack } from "expo-router";

SplashScreen.preventAutoHideAsync();

/**
 * Root layout: loads the two design-system typefaces (Bricolage Grotesque
 * for headings, IBM Plex Sans for body/labels) before rendering any screen.
 */
export default function RootLayout() {
  const [bricolageLoaded] = useBricolageFonts({ BricolageGrotesque_600SemiBold });
  const [ibmPlexLoaded] = useIBMPlexFonts({
    IBMPlexSans_400Regular,
    IBMPlexSans_500Medium,
  });
  const fontsLoaded = bricolageLoaded && ibmPlexLoaded;

  useEffect(() => {
    if (fontsLoaded) {
      void SplashScreen.hideAsync();
    }
  }, [fontsLoaded]);

  if (!fontsLoaded) {
    return null;
  }

  return <Stack screenOptions={{ headerShown: false }} />;
}
