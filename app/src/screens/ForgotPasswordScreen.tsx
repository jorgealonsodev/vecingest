import { zodResolver } from "@hookform/resolvers/zod";
import type { ApiErrorBody } from "@vecingest/shared/errors";
import { schemas } from "@vecingest/shared/schemas";
import { Link } from "expo-router";
import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { ScrollView, StyleSheet, View } from "react-native";
import {
  Button,
  HelperText,
  Text,
  TextInput,
  useTheme,
} from "react-native-paper";
import type { z } from "zod";
import { apiClient } from "../auth/api";
import { MIN_TOUCH_TARGET, spacing } from "../theme";
import { forgotPasswordErrorMessage } from "./errorMessages";

type ForgotPasswordFormValues = z.infer<typeof schemas.ForgotPasswordRequest>;

/**
 * Password-recovery request screen (Stitch mobile screen "Recuperar
 * contraseña", project `11075381530582947267`, screen
 * `ba621c2b26b7433f812fd5ea34d0a929`). No desktop-specific design exists for
 * this screen (`docs/design/stitch-screens-web.md` has no "Recuperar"/"Nueva
 * contraseña" entries), so one responsive layout serves both: full width on
 * mobile, capped at 420px and centered on desktop — the same treatment as
 * the login screen's right panel.
 *
 * `POST /v1/auth/forgot-password` intentionally never reveals whether the
 * email exists (`ForgotPasswordResponse` is just `{accepted: boolean}`), so
 * a successful call always shows the same non-committal confirmation.
 */
export function ForgotPasswordScreen() {
  const theme = useTheme();
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

  return (
    <View
      testID="forgot-password-screen"
      style={[styles.container, { backgroundColor: theme.colors.background }]}
    >
      <ScrollView contentContainerStyle={styles.scrollContent}>
        <View style={styles.formWrap}>
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
              Recuperar contraseña
            </Text>
            <Text
              variant="bodyMedium"
              style={[
                styles.subtitle,
                { color: theme.colors.onSurfaceVariant },
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
                    color: theme.colors.onSurfaceVariant,
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
                      <TextInput
                        testID="forgot-password-email"
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

                <Button
                  testID="forgot-password-submit"
                  mode="contained"
                  accessibilityLabel="Enviar enlace"
                  onPress={submit}
                  loading={submitting}
                  disabled={submitting}
                  style={styles.submit}
                >
                  Enviar enlace
                </Button>
              </>
            )}
          </View>

          <Link
            href="/(auth)/login"
            testID="forgot-password-back-link"
            style={[styles.backLink, { color: theme.colors.primary }]}
          >
            Volver a iniciar sesión
          </Link>
        </View>
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
