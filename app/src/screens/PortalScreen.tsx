import { MaterialCommunityIcons } from "@expo/vector-icons";
import { type ApiErrorBody, ErrorCode } from "@vecingest/shared/errors";
import type { schemas } from "@vecingest/shared/schemas";
import { useRouter } from "expo-router";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  Pressable,
  ScrollView,
  StyleSheet,
  useWindowDimensions,
  View,
} from "react-native";
import {
  ActivityIndicator,
  type MD3Theme,
  PaperProvider,
  Text,
  useTheme,
} from "react-native-paper";
import type { z } from "zod";
import { apiClient } from "../auth/api";
import { clearRefreshToken } from "../auth/secureTokens";
import { clearSession, getSession } from "../auth/session";
import { AuthHeader } from "../components/AuthHeader";
import { PrimaryButton } from "../components/PrimaryButton";
import {
  BREAKPOINT_DESKTOP,
  lightTheme,
  MIN_TOUCH_TARGET,
  spacing,
  WEB_BORDER,
  WEB_PAGE_BACKGROUND,
  WEB_SURFACE,
} from "../theme";

type Me = z.infer<typeof schemas.MeResponse>;
type Membership = Me["memberships"][number];

/**
 * User-facing Spanish label for each generated `MembershipEntry.role`. A
 * `Record` over the generated union, so a role added to `openapi.yaml`
 * fails the typecheck here instead of rendering a raw enum value.
 */
const ROLE_LABELS: Record<Membership["role"], string> = {
  owner: "Propietario",
  tenant: "Inquilino",
  admin: "Administrador",
  admin_staff: "Personal del despacho",
};

const SCOPE_LABELS: Record<Membership["scope"], string> = {
  community: "Comunidad",
  office: "Despacho",
};

const SCOPE_ICONS: Record<
  Membership["scope"],
  "home-city-outline" | "briefcase-outline"
> = {
  community: "home-city-outline",
  office: "briefcase-outline",
};

type LoadState =
  | { status: "loading" }
  | { status: "redirect" }
  | { status: "error" }
  | { status: "ready"; me: Me };

/**
 * First post-login destination (Stitch mobile "Elegir dónde entrar",
 * project `11075381530582947267`, screen
 * `95e97f53757e419bb498acc29cf049e9`; desktop counterpart "Selector de
 * contexto", WEB project `14138416730329310203`, screen
 * `537dd208d9d649b6a56afa3f29f51de1`).
 *
 * Both Stitch screens are a portal/context picker fed by the caller's
 * memberships. Since M1, `GET /v1/me` returns them (`memberships`, never
 * null, typed from the generated `schemas.MeResponse`), and this screen
 * renders one selectable row per membership (app-portal-memberships: Portal
 * Renders Real Membership Rows). An empty array keeps the original honest
 * empty state, unchanged.
 *
 * Still not built: the design's "remember my choice" toggle (no column or
 * endpoint persists that preference, `docs/funcionalidad/app-movil-2.md`)
 * and the fake "Colegiación Oficial nº 4.192" legend (demo-persona copy,
 * not real per-user data). The invitation-code entry point stays disabled
 * (mirroring `LoginScreen`'s `login-invitation-link`) — see
 * `docs/pendientes-funcionalidad.md`.
 *
 * Sign-out (`POST /v1/auth/logout`) is real and is the only enabled action
 * besides the redirect guard: reached with no in-memory access token (a
 * fresh page load, a deep link, a cold app start — the access token is
 * memory-only by design, see `../auth/session.ts`) or once `GET /v1/me`
 * reports the session has expired (`AUTH_UNAUTHORIZED`), this screen
 * redirects to `/(auth)/login` instead of rendering blank or a broken
 * authenticated shell.
 */
export function PortalScreen() {
  const theme = useTheme();
  const router = useRouter();
  const { width } = useWindowDimensions();
  const isDesktop = width >= BREAKPOINT_DESKTOP;
  const activeTheme: MD3Theme = isDesktop ? lightTheme : theme;

  const [state, setState] = useState<LoadState>({ status: "loading" });
  const [signingOut, setSigningOut] = useState(false);

  // Guards against a stale response (unmount, or a retry started before an
  // earlier request settled) writing state that no longer applies — an
  // incrementing counter instead of a plain boolean so the retry button can
  // re-run `loadMe` directly without needing it back in a `useEffect`
  // dependency array (this callback is deliberately created once).
  const requestIdRef = useRef(0);

  const loadMe = useCallback(async () => {
    const requestId = ++requestIdRef.current;
    setState({ status: "loading" });

    const { accessToken } = getSession();
    if (!accessToken) {
      if (requestIdRef.current === requestId) {
        setState({ status: "redirect" });
      }
      return;
    }

    const { data, error } = await apiClient.GET("/v1/me", {
      headers: { Authorization: `Bearer ${accessToken}` },
    });

    if (requestIdRef.current !== requestId) {
      return;
    }

    if (error || !data) {
      // huma's generic RFC 7807 `ErrorModel` vs. the real runtime body —
      // see `LoginScreen.tsx`'s identical comment on this cast.
      const code = (error as unknown as ApiErrorBody | undefined)?.code;
      if (code === ErrorCode.AuthUnauthorized) {
        clearSession();
        setState({ status: "redirect" });
        return;
      }
      setState({ status: "error" });
      return;
    }

    setState({ status: "ready", me: data });
  }, []);

  useEffect(() => {
    loadMe();
  }, [loadMe]);

  // Declarative `<Redirect>` needs a real navigation container (it calls
  // `useFocusEffect`/`useNavigation` under the hood), which this screen
  // cannot assume in isolation from tests — an imperative `router.replace`
  // in an effect is the same guard without that dependency, and is what
  // `handleSignOut` below already uses for the same navigation.
  useEffect(() => {
    if (state.status === "redirect") {
      router.replace("/(auth)/login");
    }
  }, [state.status, router]);

  const handleSignOut = useCallback(async () => {
    setSigningOut(true);
    try {
      const { accessToken } = getSession();
      if (accessToken) {
        await apiClient.POST("/v1/auth/logout", {
          headers: { Authorization: `Bearer ${accessToken}` },
        });
      }
    } finally {
      clearSession();
      try {
        await clearRefreshToken();
      } catch {
        // `expo-secure-store` has no web implementation
        // (`ExpoSecureStore.web.ts` exports `{}`) and throws when called —
        // the refresh token is native-only by design (`LoginRequest`'s web
        // transport never populates one for `persistRefreshToken` to have
        // stored in the first place, see `LoginScreen.tsx`). Sign-out must
        // still clear the in-memory session and navigate away on web even
        // though this native-only cleanup step cannot run there.
      }
      router.replace("/(auth)/login");
    }
  }, [router]);

  if (state.status === "redirect") {
    return (
      <View testID="portal-redirecting" style={styles.centeredBlock}>
        <ActivityIndicator animating color={theme.colors.primary} />
      </View>
    );
  }

  const signOutRow = (
    <Pressable
      testID="portal-sign-out"
      accessibilityRole="button"
      accessibilityLabel="Cerrar sesión"
      disabled={signingOut}
      onPress={handleSignOut}
      style={styles.signOutRow}
    >
      <MaterialCommunityIcons
        name="logout"
        size={16}
        color={activeTheme.colors.onSurfaceVariant}
      />
      <Text
        variant="labelMedium"
        style={{ color: activeTheme.colors.onSurfaceVariant }}
      >
        {signingOut ? "Cerrando sesión…" : "Cerrar sesión"}
      </Text>
    </Pressable>
  );

  const invitationRow = (
    <Pressable
      disabled
      accessibilityRole="link"
      accessibilityState={{ disabled: true }}
      accessibilityLabel="Añadir otra comunidad o empresa con código, no disponible todavía"
      testID="portal-invitation-link"
      style={styles.invitationRow}
    >
      <MaterialCommunityIcons
        name="plus-circle-outline"
        size={16}
        color={activeTheme.colors.onSurfaceVariant}
      />
      <Text
        variant="labelMedium"
        style={{ color: activeTheme.colors.onSurfaceVariant }}
      >
        Añadir otra comunidad o empresa con código
      </Text>
      <Text
        variant="labelSmall"
        style={[
          styles.invitationBadge,
          { color: activeTheme.colors.onSurfaceVariant },
        ]}
      >
        Próximamente
      </Text>
    </Pressable>
  );

  let body: React.ReactNode;

  if (state.status === "loading") {
    body = (
      <View testID="portal-loading" style={styles.centeredBlock}>
        <ActivityIndicator animating color={activeTheme.colors.primary} />
        <Text
          variant="bodyMedium"
          style={{ color: activeTheme.colors.onSurfaceVariant }}
        >
          Cargando tu perfil…
        </Text>
      </View>
    );
  } else if (state.status === "error") {
    body = (
      <View testID="portal-error" style={styles.centeredBlock}>
        <Text
          variant="bodyMedium"
          style={[styles.errorText, { color: activeTheme.colors.error }]}
        >
          No se ha podido cargar tu perfil. Comprueba tu conexión e inténtalo de
          nuevo.
        </Text>
        <PrimaryButton
          testID="portal-retry"
          accessibilityLabel="Reintentar"
          onPress={loadMe}
          style={styles.submit}
        >
          Reintentar
        </PrimaryButton>
      </View>
    );
  } else {
    const { me } = state;
    body = (
      <>
        <View style={styles.header}>
          <View
            testID="portal-greeting"
            style={[
              styles.greetingChip,
              { backgroundColor: activeTheme.colors.secondaryContainer },
            ]}
          >
            <MaterialCommunityIcons
              name="shield-check-outline"
              size={14}
              color={activeTheme.colors.onSecondaryContainer}
            />
            <Text
              variant="labelSmall"
              style={{ color: activeTheme.colors.onSecondaryContainer }}
            >
              Conectado como {me.email}
            </Text>
          </View>
          {me.is_superadmin ? (
            <View
              testID="portal-superadmin-badge"
              style={[
                styles.superadminBadge,
                { borderColor: activeTheme.colors.outline },
              ]}
            >
              <Text
                variant="labelSmall"
                style={{ color: activeTheme.colors.onSurfaceVariant }}
              >
                Superadministrador
              </Text>
            </View>
          ) : null}
          <Text variant="headlineSmall" style={styles.title}>
            ¿Con qué quieres trabajar?
          </Text>
          <Text
            variant="bodyMedium"
            style={{ color: activeTheme.colors.onSurfaceVariant }}
          >
            Selecciona la comunidad, vivienda o empresa con la que quieres
            trabajar.
          </Text>
        </View>

        {me.memberships.length > 0 ? (
          <View
            testID="portal-memberships"
            accessibilityRole="radiogroup"
            style={styles.membershipList}
          >
            {me.memberships.map((membership) => (
              <MembershipRow
                key={`${membership.scope}-${membership.id}`}
                membership={membership}
                theme={activeTheme}
              />
            ))}
          </View>
        ) : (
          <View
            testID="portal-empty-state"
            style={[
              styles.emptyState,
              {
                backgroundColor: activeTheme.colors.surfaceVariant,
                borderColor: activeTheme.colors.outline,
              },
            ]}
          >
            <MaterialCommunityIcons
              name="home-city-outline"
              size={28}
              color={activeTheme.colors.onSurfaceVariant}
            />
            <Text
              variant="bodyMedium"
              style={[
                styles.emptyStateTitle,
                { color: activeTheme.colors.onSurface },
              ]}
            >
              Todavía no perteneces a ninguna comunidad ni empresa.
            </Text>
            <Text
              variant="bodySmall"
              style={{ color: activeTheme.colors.onSurfaceVariant }}
            >
              Un administrador debe crear una comunidad e invitarte para que
              aparezca aquí.
            </Text>
          </View>
        )}

        <PrimaryButton
          testID="portal-primary-cta"
          accessibilityLabel="Acceder"
          accessibilityState={{ disabled: true }}
          accessibilityHint="No hay ninguna comunidad ni empresa disponible todavía"
          disabled
          style={styles.submit}
        >
          Acceder
        </PrimaryButton>

        <View style={styles.footer}>
          {invitationRow}
          {signOutRow}
        </View>
      </>
    );
  }

  if (isDesktop) {
    return (
      <View
        testID="portal-screen"
        style={[styles.desktopPage, { backgroundColor: WEB_PAGE_BACKGROUND }]}
      >
        <PaperProvider theme={lightTheme}>
          <AuthHeader />
          <ScrollView contentContainerStyle={styles.desktopMain}>
            <View
              testID="portal-desktop-card"
              style={[
                styles.desktopCard,
                { backgroundColor: WEB_SURFACE, borderColor: WEB_BORDER },
              ]}
            >
              {body}
            </View>
          </ScrollView>
        </PaperProvider>
      </View>
    );
  }

  return (
    <View
      testID="portal-screen"
      style={[
        styles.mobileContainer,
        { backgroundColor: theme.colors.background },
      ]}
    >
      <ScrollView contentContainerStyle={styles.mobileScrollContent}>
        {body}
      </ScrollView>
    </View>
  );
}

function MembershipRow({
  membership,
  theme,
}: {
  membership: Membership;
  theme: MD3Theme;
}) {
  return (
    <Pressable
      testID={`portal-membership-${membership.scope}-${membership.id}`}
      accessibilityRole="radio"
      accessibilityLabel={`${SCOPE_LABELS[membership.scope]} ${membership.name}, ${ROLE_LABELS[membership.role]}`}
      style={[styles.membershipRow, { borderColor: theme.colors.outline }]}
    >
      <MaterialCommunityIcons
        name={SCOPE_ICONS[membership.scope]}
        size={24}
        color={theme.colors.onSurfaceVariant}
      />
      <View style={styles.membershipText}>
        <Text variant="bodyLarge" style={{ color: theme.colors.onSurface }}>
          {membership.name}
        </Text>
        <Text
          variant="bodySmall"
          style={{ color: theme.colors.onSurfaceVariant }}
        >
          {ROLE_LABELS[membership.role]}
        </Text>
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  mobileContainer: {
    flex: 1,
  },
  mobileScrollContent: {
    flexGrow: 1,
    padding: spacing.space16,
    gap: spacing.space16,
  },
  desktopPage: {
    flex: 1,
  },
  desktopMain: {
    flexGrow: 1,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: spacing.space24,
    paddingVertical: spacing.space40,
  },
  desktopCard: {
    width: "100%",
    maxWidth: 480,
    alignSelf: "center",
    borderRadius: 12,
    borderWidth: 1,
    padding: spacing.space32,
    gap: spacing.space16,
  },
  centeredBlock: {
    alignItems: "center",
    gap: spacing.space12,
    paddingVertical: spacing.space40,
  },
  errorText: {
    textAlign: "center",
  },
  header: {
    gap: spacing.space8,
  },
  greetingChip: {
    alignSelf: "flex-start",
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.space8,
    borderRadius: 999,
    paddingHorizontal: spacing.space12,
    paddingVertical: spacing.space4,
  },
  superadminBadge: {
    alignSelf: "flex-start",
    borderRadius: 999,
    borderWidth: 1,
    paddingHorizontal: spacing.space12,
    paddingVertical: spacing.space4,
  },
  title: {
    marginTop: spacing.space4,
  },
  emptyState: {
    alignItems: "center",
    borderRadius: 12,
    borderWidth: 1,
    gap: spacing.space8,
    padding: spacing.space24,
  },
  membershipList: {
    gap: spacing.space8,
  },
  membershipRow: {
    alignItems: "center",
    borderRadius: 12,
    borderWidth: 1,
    flexDirection: "row",
    gap: spacing.space12,
    minHeight: MIN_TOUCH_TARGET,
    padding: spacing.space16,
  },
  membershipText: {
    flex: 1,
    gap: spacing.space4,
  },
  emptyStateTitle: {
    textAlign: "center",
  },
  submit: {
    minHeight: MIN_TOUCH_TARGET,
    justifyContent: "center",
  },
  footer: {
    gap: spacing.space16,
    alignItems: "center",
  },
  invitationRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: spacing.space8,
    minHeight: MIN_TOUCH_TARGET,
    justifyContent: "center",
  },
  invitationBadge: {
    fontStyle: "italic",
  },
  signOutRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: spacing.space8,
    minHeight: MIN_TOUCH_TARGET,
    justifyContent: "center",
  },
});
