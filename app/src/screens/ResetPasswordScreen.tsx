import { zodResolver } from "@hookform/resolvers/zod";
import type { ApiErrorBody } from "@vecingest/shared/errors";
import { schemas } from "@vecingest/shared/schemas";
import { Link } from "expo-router";
import { useState } from "react";
import { Controller, type Resolver, useForm } from "react-hook-form";
import {
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
import { resetPasswordErrorMessage } from "./errorMessages";

type ResetPasswordFormValues = z.infer<typeof schemas.ResetPasswordRequest> & {
  confirm_password: string;
};

interface ResetPasswordScreenProps {
  /** The `token` route param from the deep link, e.g. `?token=...`. */
  token?: string;
}

/**
 * New-password screen (Stitch mobile screen "Nueva contraseña", project
 * `11075381530582947267`, screen `502fdbc3adb84b7da3ae388ece6da9dd`). Same
 * "no desktop-specific design, cap at 420px and center" treatment as
 * `ForgotPasswordScreen` — see that file's header comment.
 *
 * `confirm_password` is a client-side-only field: `ResetPasswordRequest`
 * only takes `token` + `new_password`, so the match check happens in
 * `onSubmit`, before the API is ever called, and the field is never sent.
 *
 * The design's animated strength bar and hardcoded leak-warning box are not
 * built — there is no endpoint that returns a pre-submit strength signal.
 * The leak warning is instead a real, server-driven state:
 * `ErrorCode.AuthPasswordBreached` mapped by `resetPasswordErrorMessage`.
 *
 * At and above `BREAKPOINT_DESKTOP` this screen borrows the same WEB-system
 * chrome as `LoginScreen`/`ForgotPasswordScreen` (the 64px `AuthHeader` and
 * the fixed light card surface via `activeTheme`), so all three auth screens
 * read as one system — see `ForgotPasswordScreen`'s header comment.
 */
export function ResetPasswordScreen({ token }: ResetPasswordScreenProps) {
  const theme = useTheme();
  const { width } = useWindowDimensions();
  const isDesktop = width >= BREAKPOINT_DESKTOP;
  const activeTheme: MD3Theme = isDesktop ? lightTheme : theme;
  const [serverError, setServerError] = useState<string | null>(null);
  const [confirmError, setConfirmError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [succeeded, setSucceeded] = useState(false);
  const [newPasswordVisible, setNewPasswordVisible] = useState(false);
  const [confirmPasswordVisible, setConfirmPasswordVisible] = useState(false);

  const {
    control,
    handleSubmit,
    getValues,
    formState: { errors },
  } = useForm<ResetPasswordFormValues>({
    // `schemas.ResetPasswordRequest` has no `confirm_password` field (it is
    // client-side only, see the header comment), so the resolver's inferred
    // value type is narrower than `ResetPasswordFormValues` — safe to widen,
    // since `confirm_password` is never validated by this schema anyway.
    resolver: zodResolver(
      schemas.ResetPasswordRequest,
    ) as unknown as Resolver<ResetPasswordFormValues>,
    defaultValues: {
      token: token ?? "",
      new_password: "",
      confirm_password: "",
    },
  });

  const submit = handleSubmit(async (values) => {
    setServerError(null);

    // `zodResolver` parses against `schemas.ResetPasswordRequest`, which has
    // no `confirm_password` field, so the resolved `values` never carry it —
    // read it straight from form state instead.
    const confirmPassword = getValues("confirm_password");
    if (values.new_password !== confirmPassword) {
      setConfirmError("Las contraseñas no coinciden.");
      return;
    }
    setConfirmError(null);

    setSubmitting(true);
    try {
      const { data, error } = await apiClient.POST("/v1/auth/reset-password", {
        body: { token: token ?? "", new_password: values.new_password },
      });

      if (error) {
        setServerError(
          resetPasswordErrorMessage(error as unknown as ApiErrorBody),
        );
        return;
      }

      if (data?.accepted) {
        setSucceeded(true);
      }
    } finally {
      setSubmitting(false);
    }
  });

  /**
   * Wraps a screen's content in the shared shell: below the desktop
   * breakpoint, a plain theme-following container (the APP design system,
   * dark-mode aware); at and above it, the WEB design system's fixed-light
   * `AuthHeader` + `PaperProvider(lightTheme)` chrome — the same pattern
   * `LoginScreen`/`ForgotPasswordScreen` use.
   */
  function renderShell(children: React.ReactNode) {
    if (isDesktop) {
      return (
        <View
          testID="reset-password-screen"
          style={[styles.container, { backgroundColor: WEB_PAGE_BACKGROUND }]}
        >
          <PaperProvider theme={lightTheme}>
            <AuthHeader />
            <ScrollView contentContainerStyle={styles.scrollContent}>
              {children}
            </ScrollView>
          </PaperProvider>
        </View>
      );
    }

    return (
      <View
        testID="reset-password-screen"
        style={[styles.container, { backgroundColor: theme.colors.background }]}
      >
        <ScrollView contentContainerStyle={styles.scrollContent}>
          {children}
        </ScrollView>
      </View>
    );
  }

  if (!token) {
    return renderShell(
      <View style={styles.formWrap}>
        <View
          testID="reset-password-invalid-link"
          style={[
            styles.card,
            isDesktop
              ? { backgroundColor: WEB_SURFACE, borderColor: WEB_BORDER }
              : {
                  backgroundColor: activeTheme.colors.surface,
                  borderColor: activeTheme.colors.outline,
                },
          ]}
        >
          <Text variant="headlineSmall" style={styles.title}>
            Enlace inválido
          </Text>
          <Text
            variant="bodyMedium"
            style={{ color: activeTheme.colors.onSurfaceVariant }}
          >
            El enlace no es válido o ha caducado. Solicita uno nuevo.
          </Text>
          <Link
            href="/(auth)/forgot-password"
            testID="reset-password-invalid-link-cta"
            style={[styles.backLink, { color: activeTheme.colors.primary }]}
          >
            Solicitar un enlace nuevo
          </Link>
        </View>
      </View>,
    );
  }

  return renderShell(
    <View style={styles.formWrap}>
      <View
        style={[
          styles.card,
          isDesktop
            ? { backgroundColor: WEB_SURFACE, borderColor: WEB_BORDER }
            : {
                backgroundColor: activeTheme.colors.surface,
                borderColor: activeTheme.colors.outline,
              },
        ]}
      >
        <Text variant="headlineSmall" style={styles.title}>
          Nueva contraseña
        </Text>
        <Text
          variant="bodyMedium"
          style={[
            styles.subtitle,
            { color: activeTheme.colors.onSurfaceVariant },
          ]}
        >
          Establece una clave de acceso segura.
        </Text>

        {succeeded ? (
          <View testID="reset-password-success" style={styles.field}>
            <Text variant="titleMedium">Contraseña actualizada</Text>
            <Text
              variant="bodyMedium"
              style={{
                color: activeTheme.colors.onSurfaceVariant,
                marginTop: spacing.space8,
              }}
            >
              Ya puedes iniciar sesión.
            </Text>
            <Link
              href="/(auth)/login"
              testID="reset-password-success-link"
              style={[styles.backLink, { color: activeTheme.colors.primary }]}
            >
              Ir a iniciar sesión
            </Link>
          </View>
        ) : (
          <>
            <Controller
              control={control}
              name="new_password"
              render={({ field: { onChange, onBlur, value } }) => (
                <View style={styles.field}>
                  <FormTextInput
                    testID="reset-password-new-password"
                    label="Nueva contraseña"
                    secureTextEntry={!newPasswordVisible}
                    value={value}
                    onBlur={onBlur}
                    onChangeText={onChange}
                    style={styles.input}
                    right={
                      <TextInput.Icon
                        testID="reset-password-new-password-toggle"
                        icon={newPasswordVisible ? "eye-off" : "eye"}
                        accessibilityLabel={
                          newPasswordVisible
                            ? "Ocultar contraseña"
                            : "Mostrar contraseña"
                        }
                        onPress={() =>
                          setNewPasswordVisible((visible) => !visible)
                        }
                        forceTextInputFocus={false}
                      />
                    }
                  />
                  {errors.new_password ? (
                    <HelperText
                      type="error"
                      visible
                      testID="reset-password-new-password-error"
                    >
                      La contraseña debe tener al menos 12 caracteres.
                    </HelperText>
                  ) : (
                    <HelperText type="info" visible>
                      Mínimo 12 caracteres.
                    </HelperText>
                  )}
                </View>
              )}
            />

            <Controller
              control={control}
              name="confirm_password"
              render={({ field: { onChange, onBlur, value } }) => (
                <View style={styles.field}>
                  <FormTextInput
                    testID="reset-password-confirm-password"
                    label="Confirmar nueva contraseña"
                    secureTextEntry={!confirmPasswordVisible}
                    value={value}
                    onBlur={onBlur}
                    onChangeText={onChange}
                    style={styles.input}
                    right={
                      <TextInput.Icon
                        testID="reset-password-confirm-password-toggle"
                        icon={confirmPasswordVisible ? "eye-off" : "eye"}
                        accessibilityLabel={
                          confirmPasswordVisible
                            ? "Ocultar contraseña"
                            : "Mostrar contraseña"
                        }
                        onPress={() =>
                          setConfirmPasswordVisible((visible) => !visible)
                        }
                        forceTextInputFocus={false}
                      />
                    }
                  />
                  {confirmError ? (
                    <HelperText
                      type="error"
                      visible
                      testID="reset-password-confirm-password-error"
                    >
                      {confirmError}
                    </HelperText>
                  ) : null}
                </View>
              )}
            />

            {serverError ? (
              <HelperText
                type="error"
                visible
                testID="reset-password-server-error"
                style={styles.serverError}
              >
                {serverError}
              </HelperText>
            ) : null}

            <PrimaryButton
              testID="reset-password-submit"
              accessibilityLabel="Guardar contraseña"
              onPress={submit}
              loading={submitting}
              disabled={submitting}
              style={styles.submit}
            >
              Guardar contraseña
            </PrimaryButton>
          </>
        )}
      </View>
    </View>,
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
  backLink: {
    fontSize: 13,
    marginTop: spacing.space12,
    minHeight: MIN_TOUCH_TARGET,
    textAlignVertical: "center",
  },
});
