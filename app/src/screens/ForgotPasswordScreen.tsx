import { zodResolver } from "@hookform/resolvers/zod";
import type { ApiErrorBody } from "@vecingest/shared/errors";
import { schemas } from "@vecingest/shared/schemas";
import { Link } from "expo-router";
import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import {
  ScrollView,
  StyleSheet,
  useWindowDimensions,
  View,
} from "react-native";
import {
  Button,
  HelperText,
  type MD3Theme,
  PaperProvider,
  Text,
  useTheme,
} from "react-native-paper";
import type { z } from "zod";
import { apiClient } from "../auth/api";
import { AuthHeader } from "../components/AuthHeader";
import { FormTextInput } from "../components/FormTextInput";
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
import { forgotPasswordErrorMessage } from "./errorMessages";

type ForgotPasswordFormValues = z.infer<typeof schemas.ForgotPasswordRequest>;

/**
 * Password-recovery request screen (Stitch mobile screen "Recuperar
 * contraseña", project `11075381530582947267`, screen
 * `ba621c2b26b7433f812fd5ea34d0a929`). No desktop-specific design exists for
 * this screen (`docs/design/stitch-screens-web.md` has no "Recuperar"/"Nueva
 * contraseña" entries), so one responsive layout serves both: full width on
 * mobile, capped at 420px and centered on desktop.
 *
 * At and above `BREAKPOINT_DESKTOP` it borrows the WEB design system's
 * chrome from `LoginScreen` — the 64px `AuthHeader` and the fixed light card
 * surface — so the three auth screens read as one system, even though this
 * one has no two-panel Stitch design of its own. Like `LoginScreen`'s
 * desktop split, that chrome always renders in the light WEB palette via
 * `activeTheme`, never the device color scheme — see the token comment in
 * `../theme`.
 *
 * `POST /v1/auth/forgot-password` intentionally never reveals whether the
 * email exists (`ForgotPasswordResponse` is just `{accepted: boolean}`), so
 * a successful call always shows the same non-committal confirmation.
 */
export function ForgotPasswordScreen() {
  const theme = useTheme();
  const { width } = useWindowDimensions();
  const isDesktop = width >= BREAKPOINT_DESKTOP;
  const activeTheme: MD3Theme = isDesktop ? lightTheme : theme;
  const [serverError, setServerError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [sent, setSent] = useState(false);

  const {
    control,
    handleSubmit,
    formState: { errors },
  } = useForm<ForgotPasswordFormValues>({
    resolver: zodResolver(schemas.ForgotPasswordRequest),
    defaultValues: { email: "" },
  });

  const submit = handleSubmit(async (values) => {
    setServerError(null);
    setSubmitting(true);
    try {
      const { error } = await apiClient.POST("/v1/auth/forgot-password", {
        body: values,
      });

      if (error) {
        setServerError(
          forgotPasswordErrorMessage(error as unknown as ApiErrorBody),
        );
        return;
      }

      setSent(true);
    } finally {
      setSubmitting(false);
    }
  });

  const content = (
    <View style={styles.formWrap}>
      <View
        style={[
          styles.card,
          isDesktop
            ? {
                backgroundColor: WEB_SURFACE,
                borderColor: WEB_BORDER,
              }
            : {
                backgroundColor: activeTheme.colors.surface,
                borderColor: activeTheme.colors.outline,
              },
        ]}
      >
        <Text variant="headlineSmall" style={styles.title}>
          Recuperar contraseña
        </Text>
        <Text
          variant="bodyMedium"
          style={[
            styles.subtitle,
            { color: activeTheme.colors.onSurfaceVariant },
          ]}
        >
          Indica tu dirección de correo asociada a la comunidad.
        </Text>

        {sent ? (
          <View testID="forgot-password-confirmation" style={styles.field}>
            <Text variant="titleMedium">Instrucciones enviadas</Text>
            <Text
              variant="bodyMedium"
              style={{
                color: activeTheme.colors.onSurfaceVariant,
                marginTop: spacing.space8,
              }}
            >
              Si el email existe, te hemos enviado un enlace.
            </Text>
            <Button
              testID="forgot-password-resend"
              mode="text"
              onPress={() => submit()}
              style={styles.resend}
            >
              Reenviar correo
            </Button>
          </View>
        ) : (
          <>
            <Controller
              control={control}
              name="email"
              render={({ field: { onChange, onBlur, value } }) => (
                <View style={styles.field}>
                  <FormTextInput
                    testID="forgot-password-email"
                    label="Correo electrónico"
                    autoCapitalize="none"
                    keyboardType="email-address"
                    value={value}
                    onBlur={onBlur}
                    onChangeText={onChange}
                    style={styles.input}
                  />
                  {errors.email ? (
                    <HelperText
                      type="error"
                      visible
                      testID="forgot-password-email-error"
                    >
                      Escribe un correo electrónico válido.
                    </HelperText>
                  ) : null}
                </View>
              )}
            />

            {serverError ? (
              <HelperText
                type="error"
                visible
                testID="forgot-password-server-error"
                style={styles.serverError}
              >
                {serverError}
              </HelperText>
            ) : null}

            <PrimaryButton
              testID="forgot-password-submit"
              accessibilityLabel="Enviar enlace"
              onPress={submit}
              loading={submitting}
              disabled={submitting}
              style={styles.submit}
            >
              Enviar enlace
            </PrimaryButton>
          </>
        )}
      </View>

      <Link
        href="/(auth)/login"
        testID="forgot-password-back-link"
        style={[styles.backLink, { color: activeTheme.colors.primary }]}
      >
        Volver a iniciar sesión
      </Link>
    </View>
  );

  if (isDesktop) {
    return (
      <View
        testID="forgot-password-screen"
        style={[styles.container, { backgroundColor: WEB_PAGE_BACKGROUND }]}
      >
        <PaperProvider theme={lightTheme}>
          <AuthHeader />
          <ScrollView contentContainerStyle={styles.scrollContent}>
            {content}
          </ScrollView>
        </PaperProvider>
      </View>
    );
  }

  return (
    <View
      testID="forgot-password-screen"
      style={[styles.container, { backgroundColor: theme.colors.background }]}
    >
      <ScrollView contentContainerStyle={styles.scrollContent}>
        {content}
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
  },
  scrollContent: {
    flexGrow: 1,
    justifyContent: "center",
    padding: spacing.space16,
  },
  formWrap: {
    alignSelf: "center",
    gap: spacing.space16,
    width: "100%",
    maxWidth: 420,
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
  serverError: {
    marginBottom: spacing.space8,
  },
  submit: {
    minHeight: MIN_TOUCH_TARGET,
    justifyContent: "center",
  },
  resend: {
    marginTop: spacing.space8,
  },
  backLink: {
    alignSelf: "center",
    fontSize: 13,
    minHeight: MIN_TOUCH_TARGET,
    textAlignVertical: "center",
  },
});
