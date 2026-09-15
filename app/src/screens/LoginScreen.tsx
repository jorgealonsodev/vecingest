import { MaterialCommunityIcons } from "@expo/vector-icons";
import { zodResolver } from "@hookform/resolvers/zod";
import type { ApiErrorBody } from "@vecingest/shared/errors";
import { schemas } from "@vecingest/shared/schemas";
import { Link, useRouter } from "expo-router";
import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import {
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  useWindowDimensions,
  View,
} from "react-native";
import {
  HelperText,
  type MD3Theme,
  PaperProvider,
  Text,
  TextInput,
  useTheme,
} from "react-native-paper";
import type { z } from "zod";
import { apiClient } from "../auth/api";
import { persistRefreshToken } from "../auth/secureTokens";
import { setSession } from "../auth/session";
import { AuthHeader } from "../components/AuthHeader";
import { FormTextInput } from "../components/FormTextInput";
import { PrimaryButton } from "../components/PrimaryButton";
import {
  BREAKPOINT_DESKTOP,
  INSTITUTIONAL_PANEL_BACKGROUND,
  INSTITUTIONAL_PANEL_FOREGROUND,
  lightTheme,
  MIN_TOUCH_TARGET,
  spacing,
  WEB_BORDER,
  WEB_CONTENT_MAX,
  WEB_PAGE_BACKGROUND,
  WEB_SURFACE,
} from "../theme";
import { loginErrorMessage } from "./errorMessages";

type LoginFormValues = z.infer<typeof schemas.LoginRequest>;

function nativePlatform(): LoginFormValues["platform"] {
  if (Platform.OS === "ios" || Platform.OS === "android") {
    return Platform.OS;
  }
  return "web";
}

/**
 * The login form. No hand-written validation schema — `resolver` is bound
 * directly to the generated `schemas.LoginRequest` (app-login-ui:
 * Generated-Schema Form Validation). `platform` is a hidden default value,
 * never a field the user fills in.
 *
 * Layout follows the two Stitch "Iniciar sesión" / "Login web" designs: a
 * single column below `BREAKPOINT_DESKTOP`, and — at and above it — the WEB
 * design system's actual structure (`docs/design/vecingest-web.md`, Stitch
 * project `projects/14138416730329310203`, screen "Login web"
 * `5f83c21f55d9493da67cc330fa8528d4`): a 64px header, then a `content-max`
 * (1120px) rounded card containing the institutional left panel and the
 * capped-width form column, vertically centered on the page — not two
 * full-bleed, top-aligned halves. The invitation-code entry point is
 * visually faithful to the desktop design but has no backend at M0 — see
 * `docs/pendientes-funcionalidad.md`. The design's three-segment profile
 * selector ("Vecino / Administrador / Empresa") was removed entirely
 * (2026-09-15, same doc): `POST /v1/auth/login` takes no role parameter, so
 * a disabled control that visually implied one read as broken rather than
 * as an honest "not yet" state. Choosing a real context happens on
 * `PortalScreen`, the screen a successful login now navigates to.
 *
 * The desktop split is forced into the WEB system's light palette via
 * `activeTheme` (see below) regardless of device color scheme, because that
 * design system has no dark mode — see the token comment in `../theme`.
 */
export function LoginScreen() {
  const theme = useTheme();
  const router = useRouter();
  const { width } = useWindowDimensions();
  const isDesktop = width >= BREAKPOINT_DESKTOP;
  const [serverError, setServerError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [passwordVisible, setPasswordVisible] = useState(false);

  // The WEB design system (desktop split) has no dark mode: it is always
  // rendered with `lightTheme`, never the ambient device-scheme theme. Below
  // the desktop breakpoint the screen keeps following the device scheme, as
  // the APP design system (`docs/design/vecingest-dark.md`) intends.
  const activeTheme: MD3Theme = isDesktop ? lightTheme : theme;

  const {
    control,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginFormValues>({
    resolver: zodResolver(schemas.LoginRequest),
    defaultValues: {
      email: "",
      password: "",
      platform: nativePlatform(),
    },
  });

  const onSubmit = handleSubmit(async (values) => {
    setServerError(null);
    setSubmitting(true);
    try {
      const { data, error } = await apiClient.POST("/v1/auth/login", {
        body: values,
      });

      if (error) {
        // huma documents this response with its generic RFC 7807
        // `ErrorModel`; the real runtime body is `{code, message, details}`
        // (see packages/shared/src/errors.ts's header comment for why).
        setServerError(loginErrorMessage(error as unknown as ApiErrorBody));
        return;
      }

      if (data) {
        setSession({
          accessToken: data.access_token,
          csrfToken: data.csrf_token ?? null,
        });
        if (data.refresh_token) {
          await persistRefreshToken(data.refresh_token);
        }
        router.replace("/portal");
      }
    } finally {
      setSubmitting(false);
    }
  });

  const invitationLabel = (
    <>
      <Text
        variant="labelMedium"
        style={{ color: activeTheme.colors.onSurfaceVariant }}
      >
        Tengo un código de invitación
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
    </>
  );

  const invitationLink = isDesktop ? (
    <View style={styles.invitationDesktopWrap}>
      <Text
        variant="labelSmall"
        style={[
          styles.mutedCaption,
          { color: activeTheme.colors.onSurfaceVariant },
        ]}
      >
        ¿Primera convocatoria o registro?
      </Text>
      <Pressable
        disabled
        accessibilityRole="link"
        accessibilityState={{ disabled: true }}
        accessibilityLabel="Código de invitación, no disponible todavía"
        testID="login-invitation-link"
        style={[
          styles.invitationPill,
          { borderColor: activeTheme.colors.outline },
        ]}
      >
        {invitationLabel}
      </Pressable>
    </View>
  ) : (
    <Pressable
      disabled
      accessibilityRole="link"
      accessibilityState={{ disabled: true }}
      accessibilityLabel="Código de invitación, no disponible todavía"
      testID="login-invitation-link"
      style={styles.invitationRow}
    >
      {invitationLabel}
    </Pressable>
  );

  const formContent = (
    <View style={styles.formColumn}>
      {isDesktop ? (
        <View style={styles.formHeader}>
          <View
            testID="login-form-mark"
            style={[styles.formMark, { backgroundColor: WEB_PAGE_BACKGROUND }]}
          />
          <Text variant="headlineSmall" style={styles.title}>
            Iniciar sesión
          </Text>
          <Text
            variant="bodySmall"
            style={{ color: activeTheme.colors.onSurfaceVariant }}
          >
            Introduzca sus credenciales autorizadas
          </Text>
        </View>
      ) : (
        <Text
          variant="labelSmall"
          style={[
            styles.kicker,
            { color: activeTheme.colors.onSurfaceVariant },
          ]}
        >
          Gestión integral de fincas
        </Text>
      )}

      <View
        style={[
          styles.card,
          isDesktop
            ? styles.cardBorderless
            : {
                backgroundColor: activeTheme.colors.surface,
                borderColor: activeTheme.colors.outline,
              },
        ]}
      >
        {isDesktop ? null : (
          <>
            <Text variant="headlineSmall" style={styles.title}>
              Acceso a la plataforma
            </Text>
            <Text
              variant="bodyMedium"
              style={[
                styles.subtitle,
                { color: activeTheme.colors.onSurfaceVariant },
              ]}
            >
              Introduzca sus credenciales autorizadas
            </Text>
          </>
        )}

        <Controller
          control={control}
          name="email"
          render={({ field: { onChange, onBlur, value } }) => (
            <View style={styles.field}>
              <FormTextInput
                testID="login-email"
                label="Correo electrónico"
                autoCapitalize="none"
                keyboardType="email-address"
                value={value}
                onBlur={onBlur}
                onChangeText={onChange}
                style={styles.input}
              />
              {errors.email ? (
                <HelperText type="error" visible testID="login-email-error">
                  Escribe un correo electrónico válido.
                </HelperText>
              ) : null}
            </View>
          )}
        />

        <Controller
          control={control}
          name="password"
          render={({ field: { onChange, onBlur, value } }) => (
            <View style={styles.field}>
              <FormTextInput
                testID="login-password"
                label="Contraseña"
                secureTextEntry={!passwordVisible}
                value={value}
                onBlur={onBlur}
                onChangeText={onChange}
                style={styles.input}
                right={
                  <TextInput.Icon
                    testID="login-password-toggle"
                    icon={passwordVisible ? "eye-off" : "eye"}
                    accessibilityLabel={
                      passwordVisible
                        ? "Ocultar contraseña"
                        : "Mostrar contraseña"
                    }
                    onPress={() => setPasswordVisible((visible) => !visible)}
                    forceTextInputFocus={false}
                  />
                }
              />
              {errors.password ? (
                <HelperText type="error" visible testID="login-password-error">
                  Escribe la contraseña.
                </HelperText>
              ) : null}
            </View>
          )}
        />

        <View style={styles.forgotRow}>
          <Link
            href="/(auth)/forgot-password"
            testID="login-forgot-password-link"
            style={[styles.forgotLink, { color: activeTheme.colors.primary }]}
          >
            ¿Has olvidado la contraseña?
          </Link>
        </View>

        {serverError ? (
          <HelperText
            type="error"
            visible
            testID="login-server-error"
            style={styles.serverError}
          >
            {serverError}
          </HelperText>
        ) : null}

        <PrimaryButton
          testID="login-submit"
          accessibilityLabel="Entrar"
          onPress={onSubmit}
          loading={submitting}
          disabled={submitting}
          style={styles.submit}
        >
          Entrar
        </PrimaryButton>
      </View>

      <View style={styles.footer}>
        {invitationLink}
        <View style={styles.encryptedRow}>
          <MaterialCommunityIcons
            name="lock"
            size={14}
            color={activeTheme.colors.onSurfaceVariant}
          />
          <Text
            variant="labelSmall"
            style={[
              styles.encryptedText,
              { color: activeTheme.colors.onSurfaceVariant },
            ]}
          >
            Conexión cifrada de alta seguridad
          </Text>
        </View>
      </View>
    </View>
  );

  if (isDesktop) {
    return (
      <View
        testID="login-screen"
        style={[styles.desktopPage, { backgroundColor: WEB_PAGE_BACKGROUND }]}
      >
        {/*
          The WEB design system's split renders in its own fixed light
          theme regardless of device color scheme (see `activeTheme`
          above). Nesting `PaperProvider` here forces every descendant
          Paper component (`TextInput`, `Button`, default `Text` variant
          colors) into that same light palette too, instead of only fixing
          the explicit inline colors this file sets directly.
        */}
        <PaperProvider theme={lightTheme}>
          <AuthHeader />
          <ScrollView contentContainerStyle={styles.desktopMain}>
            <View style={styles.desktopCardWrap}>
              <View
                testID="login-desktop-card"
                style={[
                  styles.desktopCard,
                  { backgroundColor: WEB_SURFACE, borderColor: WEB_BORDER },
                ]}
              >
                <View
                  testID="login-institutional-panel"
                  style={[
                    styles.leftPanel,
                    { backgroundColor: INSTITUTIONAL_PANEL_BACKGROUND },
                  ]}
                >
                  <View style={styles.leftTop}>
                    <View style={styles.leftBrandRow}>
                      <View
                        testID="login-panel-logo"
                        style={styles.leftLogoBox}
                      />
                      <View>
                        <Text
                          variant="titleMedium"
                          style={{ color: INSTITUTIONAL_PANEL_FOREGROUND }}
                        >
                          Vecingest
                        </Text>
                        <Text variant="labelSmall" style={styles.mutedOnDark}>
                          Entorno Operativo Colegiado
                        </Text>
                      </View>
                    </View>

                    <View style={styles.statusPill}>
                      <View style={styles.statusDot} />
                      <Text
                        variant="labelSmall"
                        style={styles.mutedOnDarkStrong}
                      >
                        Sistema Activo · Nodo Seguro Madrid ES-01
                      </Text>
                    </View>
                  </View>

                  <View style={styles.leftMiddle}>
                    <Text
                      variant="headlineMedium"
                      style={{ color: INSTITUTIONAL_PANEL_FOREGROUND }}
                    >
                      La comunidad de propietarios, en orden
                    </Text>
                    <Text variant="bodyMedium" style={styles.mutedOnDark}>
                      Plataforma de gestión integral para administradores de
                      fincas colegiados y propietarios. Trazabilidad jurídica
                      según Ley de Propiedad Horizontal.
                    </Text>

                    <View style={styles.statGrid}>
                      <View style={styles.statItem}>
                        <Text
                          variant="headlineSmall"
                          style={{ color: INSTITUTIONAL_PANEL_FOREGROUND }}
                        >
                          100%
                        </Text>
                        <Text variant="labelSmall" style={styles.mutedOnDark}>
                          Cumplimiento LPH Art. 17
                        </Text>
                      </View>
                      <View style={styles.statItem}>
                        <Text
                          variant="headlineSmall"
                          style={{ color: INSTITUTIONAL_PANEL_FOREGROUND }}
                        >
                          256-bit
                        </Text>
                        <Text variant="labelSmall" style={styles.mutedOnDark}>
                          Cifrado de Libro de Actas
                        </Text>
                      </View>
                    </View>
                  </View>

                  <View style={styles.certRow}>
                    <Text variant="labelSmall" style={styles.mutedOnDark}>
                      Certificación CAF y CGCAFE
                    </Text>
                    <MaterialCommunityIcons
                      name="shield-check-outline"
                      size={18}
                      color="rgba(255,255,255,0.7)"
                    />
                  </View>
                </View>

                <View
                  style={[styles.rightPanel, { backgroundColor: WEB_SURFACE }]}
                >
                  <View style={styles.desktopFormWrap}>{formContent}</View>
                </View>
              </View>
            </View>
          </ScrollView>
        </PaperProvider>
      </View>
    );
  }

  return (
    <View
      testID="login-screen"
      style={[
        styles.mobileContainer,
        { backgroundColor: theme.colors.background },
      ]}
    >
      <ScrollView contentContainerStyle={styles.mobileScrollContent}>
        {formContent}
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  mobileContainer: {
    flex: 1,
  },
  mobileScrollContent: {
    flexGrow: 1,
    justifyContent: "center",
    padding: spacing.space16,
  },

  // Desktop (WEB design system) — page shell: header + centered content-max
  // card, replacing the previous two full-bleed, top-aligned halves.
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
  desktopCardWrap: {
    width: "100%",
    maxWidth: WEB_CONTENT_MAX,
    alignSelf: "center",
    paddingVertical: spacing.space32,
  },
  desktopCard: {
    flexDirection: "row",
    borderRadius: 12,
    borderWidth: 1,
    overflow: "hidden",
  },
  leftPanel: {
    flex: 1,
    padding: spacing.space40,
    justifyContent: "space-between",
  },
  leftTop: {
    gap: spacing.space24,
  },
  leftBrandRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.space12,
  },
  leftLogoBox: {
    width: 40,
    height: 40,
    borderRadius: 8,
    backgroundColor: "#ffffff",
  },
  statusPill: {
    alignSelf: "flex-start",
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.space8,
    backgroundColor: "rgba(255,255,255,0.1)",
    borderRadius: 999,
    paddingHorizontal: spacing.space12,
    paddingVertical: spacing.space4,
  },
  statusDot: {
    width: 8,
    height: 8,
    borderRadius: 4,
    backgroundColor: "#1d9e75",
  },
  leftMiddle: {
    marginVertical: spacing.space48,
    gap: spacing.space16,
  },
  statGrid: {
    flexDirection: "row",
    gap: spacing.space16,
    paddingTop: spacing.space24,
    paddingRight: spacing.space16,
    paddingBottom: spacing.space16,
    paddingLeft: spacing.space16,
    marginTop: spacing.space16,
    backgroundColor: "rgba(255,255,255,0.05)",
    borderRadius: 8,
  },
  statItem: {
    flex: 1,
  },
  certRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
  mutedOnDark: {
    color: "rgba(255, 255, 255, 0.7)",
  },
  mutedOnDarkStrong: {
    color: "rgba(255, 255, 255, 0.85)",
  },
  rightPanel: {
    flex: 1,
    padding: spacing.space32,
    alignItems: "center",
    justifyContent: "center",
  },
  desktopFormWrap: {
    width: "100%",
    maxWidth: 420,
    alignSelf: "center",
  },
  formHeader: {
    alignItems: "center",
    gap: spacing.space4,
  },
  formMark: {
    width: 48,
    height: 48,
    borderRadius: 24,
    marginBottom: spacing.space12,
    borderWidth: 1,
    borderColor: WEB_BORDER,
  },

  formColumn: {
    gap: spacing.space24,
  },
  kicker: {
    textAlign: "center",
    textTransform: "uppercase",
    letterSpacing: 1.5,
  },
  card: {
    borderRadius: 12,
    padding: spacing.space16,
  },
  cardBorderless: {
    padding: 0,
  },
  title: {
    marginBottom: spacing.space4,
  },
  subtitle: {
    marginBottom: spacing.space16,
  },
  field: {
    marginBottom: spacing.space12,
  },
  input: {
    minHeight: MIN_TOUCH_TARGET,
  },
  mutedCaption: {
    marginBottom: spacing.space8,
  },
  forgotRow: {
    alignItems: "flex-end",
    justifyContent: "center",
    marginBottom: spacing.space8,
    minHeight: MIN_TOUCH_TARGET,
  },
  forgotLink: {
    fontSize: 13,
  },
  serverError: {
    marginBottom: spacing.space8,
  },
  submit: {
    minHeight: MIN_TOUCH_TARGET,
    justifyContent: "center",
  },
  footer: {
    alignItems: "center",
    gap: spacing.space16,
  },
  invitationRow: {
    alignItems: "center",
    gap: spacing.space4,
    minHeight: MIN_TOUCH_TARGET,
    justifyContent: "center",
  },
  invitationDesktopWrap: {
    alignItems: "center",
  },
  invitationPill: {
    alignItems: "center",
    borderRadius: 999,
    borderWidth: 1,
    flexDirection: "row",
    gap: spacing.space8,
    minHeight: MIN_TOUCH_TARGET,
    paddingHorizontal: spacing.space16,
    paddingVertical: spacing.space8,
  },
  invitationBadge: {
    fontStyle: "italic",
  },
  encryptedRow: {
    alignItems: "center",
    flexDirection: "row",
  },
  encryptedText: {
    marginLeft: spacing.space8,
  },
});
