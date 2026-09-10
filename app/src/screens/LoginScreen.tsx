import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Platform, StyleSheet, View } from "react-native";
import { Button, HelperText, Text, TextInput, useTheme } from "react-native-paper";
import { schemas } from "@vecingest/shared/schemas";
import type { ApiErrorBody } from "@vecingest/shared/errors";
import type { z } from "zod";
import { apiClient } from "../auth/api";
import { setSession } from "../auth/session";
import { persistRefreshToken } from "../auth/secureTokens";
import { loginErrorMessage } from "./errorMessages";
import { MIN_TOUCH_TARGET } from "../theme";

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
 */
export function LoginScreen() {
  const theme = useTheme();
  const [serverError, setServerError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

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

  return (
    <View testID="login-screen" style={[styles.container, { backgroundColor: theme.colors.background }]}>
      <Text variant="headlineLarge" style={styles.title}>
        Iniciar sesión
      </Text>

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
              secureTextEntry
              value={value}
              onBlur={onBlur}
              onChangeText={onChange}
              style={styles.input}
            />
            {errors.password ? (
              <HelperText type="error" visible testID="login-password-error">
                Escribe la contraseña.
              </HelperText>
            ) : null}
          </View>
        )}
      />

      {serverError ? (
        <HelperText type="error" visible testID="login-server-error" style={styles.serverError}>
          {serverError}
        </HelperText>
      ) : null}

      <Button
        testID="login-submit"
        mode="contained"
        accessibilityLabel="Iniciar sesión"
        onPress={onSubmit}
        loading={submitting}
        disabled={submitting}
        style={styles.submit}
      >
        Iniciar sesión
      </Button>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    padding: 16,
    justifyContent: "center",
  },
  title: {
    marginBottom: 24,
  },
  field: {
    marginBottom: 12,
  },
  input: {
    minHeight: MIN_TOUCH_TARGET,
  },
  serverError: {
    marginBottom: 8,
  },
  submit: {
    minHeight: MIN_TOUCH_TARGET,
    justifyContent: "center",
  },
});
