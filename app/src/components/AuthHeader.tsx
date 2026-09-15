import { StyleSheet, View } from "react-native";
import { Text } from "react-native-paper";
import {
  ACCENT_LIGHT,
  WEB_BORDER,
  WEB_CONTENT_MAX,
  WEB_HEADER_HEIGHT,
  WEB_SURFACE,
  WEB_TEXT_PRIMARY,
} from "../theme";

/**
 * Fixed-light 64px header shared by the desktop split of the three web auth
 * screens (login, forgot password, reset password) — the "VecinGest WEB"
 * design system's header band (`docs/design/vecingest-web.md`, Stitch
 * project `projects/14138416730329310203`, screen "Login web"
 * `5f83c21f55d9493da67cc330fa8528d4`: `h-header-height max-w-content-max
 * mx-auto px-space-24 flex items-center justify-between`).
 *
 * No logo asset exists in the repo. The Stitch header carries a full nav bar
 * and a user avatar that don't apply to an unauthenticated auth screen; this
 * component renders only the one element the design specifies with an exact
 * shape — the brand mark (`w-8 h-8 rounded-full bg-primary`) — as an empty
 * rounded-full container in the single approved accent color
 * (`docs/design/README.md`: "One single interactive accent"), with no
 * artwork inside. Report/replace with the real mark image when one exists.
 *
 * Always renders the light WEB palette regardless of device color scheme —
 * see the token comment in `../theme` — and is desktop-only: the Stitch
 * mobile exports have no equivalent top bar.
 */
export function AuthHeader() {
  return (
    <View
      testID="auth-header"
      style={[
        styles.header,
        { backgroundColor: WEB_SURFACE, borderBottomColor: WEB_BORDER },
      ]}
    >
      <View style={styles.inner}>
        <View style={styles.brand}>
          <View
            testID="auth-header-brand-mark"
            style={[styles.mark, { backgroundColor: ACCENT_LIGHT }]}
          />
          <Text
            variant="titleMedium"
            style={[styles.wordmark, { color: WEB_TEXT_PRIMARY }]}
          >
            Vecingest
          </Text>
        </View>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  header: {
    width: "100%",
    height: WEB_HEADER_HEIGHT,
    borderBottomWidth: 1,
  },
  inner: {
    flex: 1,
    width: "100%",
    maxWidth: WEB_CONTENT_MAX,
    alignSelf: "center",
    paddingHorizontal: 24,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
  brand: {
    flexDirection: "row",
    alignItems: "center",
    gap: 12,
  },
  mark: {
    width: 32,
    height: 32,
    borderRadius: 16,
  },
  wordmark: {
    fontWeight: "600",
  },
});
