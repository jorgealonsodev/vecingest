import { MaterialCommunityIcons } from "@expo/vector-icons";
import { zodResolver } from "@hookform/resolvers/zod";
import type { ApiErrorBody } from "@vecingest/shared/errors";
import { schemas } from "@vecingest/shared/schemas";
import { Link } from "expo-router";
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
  Button,
  HelperText,
  Text,
  TextInput,
  useTheme,
} from "react-native-paper";
import type { z } from "zod";
import { apiClient } from "../auth/api";
import { persistRefreshToken } from "../auth/secureTokens";
import { setSession } from "../auth/session";
import { BREAKPOINT_DESKTOP, MIN_TOUCH_TARGET, spacing } from "../theme";
import { loginErrorMessage } from "./errorMessages";

type LoginFormValues = z.infer<typeof schemas.LoginRequest>;

const PROFILE_OPTIONS = ["Vecino", "Administrador", "Empresa"] as const;

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
 * single column below `BREAKPOINT_DESKTOP`, and an institutional left panel
 * + capped-width form column at and above it. The profile selector and the
 * invitation-code entry point are visually faithful to the desktop design
 * but have no backend at M0 — see `docs/pendientes-funcionalidad.md`.
 */
export function LoginScreen() {
  const theme = useTheme();
  const { width } = useWindowDimensions();
  const isDesktop = width >= BREAKPOINT_DESKTOP;
  const [serverError, setServerError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [passwordVisible, setPasswordVisible] = useState(false);

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
      }
    } finally {
      setSubmitting(false);
    }
  });

  const invitationLabel = (
    <>
      <Text
        variant="labelMedium"
        style={{ color: theme.colors.onSurfaceVariant }}
      >
        Tengo un código de invitación
      </Text>
      <Text
        variant="labelSmall"
        style={[
          styles.invitationBadge,
          { color: theme.colors.onSurfaceVariant },
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
        style={[styles.mutedCaption, { color: theme.colors.onSurfaceVariant }]}
      >
        ¿Primera convocatoria o registro?
      </Text>
      <Pressable
        disabled
        accessibilityRole="link"
        accessibilityState={{ disabled: true }}
        accessibilityLabel="Código de invitación, no disponible todavía"
        testID="login-invitation-link"
        style={[styles.invitationPill, { borderColor: theme.colors.outline }]}
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
      <Text
        variant="labelSmall"
        style={[styles.kicker, { color: theme.colors.onSurfaceVariant }]}
      >
        Gestión integral de fincas
      </Text>

      <View
        style={[
          styles.card,
          {
            backgroundColor: theme.colors.surface,
            borderColor: theme.colors.outline,
          },
        ]}
      >
        <Text variant="headlineSmall" style={styles.title}>
          Acceso a la plataforma
        </Text>
        <Text
          variant="bodyMedium"
          style={[styles.subtitle, { color: theme.colors.onSurfaceVariant }]}
        >
          Introduzca sus credenciales autorizadas
        </Text>

        {isDesktop ? (
          <View style={styles.field}>
            <Text
              variant="labelMedium"
              style={[
                styles.mutedCaption,
                { color: theme.colors.onSurfaceVariant },
              ]}
            >
              Perfil de usuario
            </Text>
            <View
              testID="login-profile-selector"
              accessibilityRole="radiogroup"
              accessibilityState={{ disabled: true }}
              accessibilityHint="Selector de perfil no disponible: el inicio de sesión no distingue todavía entre roles"
              style={[
                styles.segmentedControl,
                {
                  backgroundColor: theme.colors.surfaceVariant,
                  borderColor: theme.colors.outline,
                },
              ]}
            >
              {PROFILE_OPTIONS.map((label) => (
                <View key={label} style={styles.segment}>
                  <Text
                    variant="labelMedium"
                    style={{ color: theme.colors.onSurfaceVariant }}
                  >
                    {label}
                  </Text>
                </View>
              ))}
            </View>
            <Text
              variant="labelSmall"
              style={[
                styles.mutedCaption,
                { color: theme.colors.onSurfaceVariant },
              ]}
            >
              No disponible en esta versión.
            </Text>
          </View>
        ) : null}

        <Controller
          control={control}
          name="email"
          render={({ field: { onChange, onBlur, value } }) => (
            <View style={styles.field}>
              <TextInput
                testID="login-email"
                mode="outlined"
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
              <TextInput
                testID="login-password"
                mode="outlined"
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
            style={[styles.forgotLink, { color: theme.colors.primary }]}
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

        <Button
          testID="login-submit"
          mode="contained"
          accessibilityLabel="Entrar"
          onPress={onSubmit}
          loading={submitting}
          disabled={submitting}
          style={styles.submit}
        >
          Entrar
        </Button>
      </View>

      <View style={styles.footer}>
        {invitationLink}
        <View style={styles.encryptedRow}>
          <MaterialCommunityIcons
            name="lock"
            size={14}
            color={theme.colors.onSurfaceVariant}
          />
          <Text
            variant="labelSmall"
            style={[
              styles.encryptedText,
              { color: theme.colors.onSurfaceVariant },
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
        style={[
          styles.desktopContainer,
          { backgroundColor: theme.colors.background },
        ]}
      >
        <View
          style={[
            styles.leftPanel,
            { backgroundColor: theme.colors.onBackground },
          ]}
        >
          <View>
            <Text
              variant="titleMedium"
              style={{ color: theme.colors.onPrimary }}
            >
              Vecingest
            </Text>
            <Text variant="labelSmall" style={styles.mutedOnDark}>
              Entorno Operativo Colegiado
            </Text>
          </View>

          <View style={styles.statusPill}>
            <Text
              variant="labelSmall"
              style={{ color: theme.colors.onPrimary }}
            >
              Sistema Activo · Nodo Seguro Madrid ES-01
            </Text>
          </View>

          <Text
            variant="headlineSmall"
            style={[styles.leftHeadline, { color: theme.colors.onPrimary }]}
          >
            La comunidad de propietarios, en orden
          </Text>
          <Text
            variant="bodyMedium"
            style={[styles.leftParagraph, styles.mutedOnDark]}
          >
            Plataforma de gestión integral para administradores de fincas
            colegiados y propietarios. Trazabilidad jurídica según Ley de
            Propiedad Horizontal.
          </Text>

          <View style={styles.statGrid}>
            <View style={styles.statItem}>
              <Text
                variant="headlineSmall"
                style={{ color: theme.colors.onPrimary }}
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
                style={{ color: theme.colors.onPrimary }}
              >
                256-bit
              </Text>
              <Text variant="labelSmall" style={styles.mutedOnDark}>
                Cifrado de Libro de Actas
              </Text>
            </View>
          </View>

          <Text
            variant="labelSmall"
            style={[styles.mutedOnDark, styles.certRow]}
          >
            Certificación CAF y CGCAFE
          </Text>
        </View>

        <ScrollView
          style={styles.rightPanel}
          contentContainerStyle={[
            styles.rightPanelContent,
            { backgroundColor: theme.colors.surface },
          ]}
        >
          <View style={styles.desktopFormWrap}>{formContent}</View>
        </ScrollView>
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
  desktopContainer: {
    flex: 1,
    flexDirection: "row",
  },
  leftPanel: {
    flex: 1,
    padding: spacing.space40,
    justifyContent: "flex-start",
  },
  rightPanel: {
    flex: 1,
  },
  rightPanelContent: {
    flexGrow: 1,
    justifyContent: "center",
    padding: spacing.space32,
  },
  desktopFormWrap: {
    width: "100%",
    maxWidth: 420,
    alignSelf: "center",
  },
  mutedOnDark: {
    color: "rgba(255, 255, 255, 0.7)",
  },
  statusPill: {
    alignSelf: "flex-start",
    backgroundColor: "rgba(255, 255, 255, 0.12)",
    borderRadius: 999,
    marginTop: spacing.space24,
    paddingHorizontal: spacing.space12,
    paddingVertical: spacing.space4,
  },
  leftHeadline: {
    marginTop: spacing.space32,
  },
  leftParagraph: {
    marginTop: spacing.space16,
  },
  statGrid: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: spacing.space24,
    marginTop: spacing.space32,
  },
  statItem: {
    flex: 1,
    minWidth: 120,
  },
  certRow: {
    marginTop: spacing.space40,
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
    borderWidth: 1,
    padding: spacing.space16,
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
  segmentedControl: {
    borderRadius: 999,
    borderWidth: 1,
    flexDirection: "row",
    overflow: "hidden",
  },
  segment: {
    alignItems: "center",
    flex: 1,
    minHeight: MIN_TOUCH_TARGET,
    justifyContent: "center",
    paddingVertical: spacing.space8,
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
