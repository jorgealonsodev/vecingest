import { MaterialCommunityIcons } from "@expo/vector-icons";
import type { ApiErrorBody } from "@vecingest/shared/errors";
import { ErrorCode } from "@vecingest/shared/errors";
import { schemas } from "@vecingest/shared/schemas";
import { useRouter } from "expo-router";
import { useState } from "react";
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
  lightTheme,
  MIN_TOUCH_TARGET,
  spacing,
  WEB_BORDER,
  WEB_PAGE_BACKGROUND,
  WEB_SURFACE,
} from "../theme";
import {
  invitationAcceptErrorMessage,
  invitationPreviewErrorMessage,
} from "./errorMessages";
import { ROLE_LABELS } from "./portalContext";

type Preview = z.infer<typeof schemas.PreviewInvitationResponse>;
type AcceptBody = z.infer<typeof schemas.AcceptInvitationRequest>;

/** The short code format the Stitch design states: 8 alphanumeric chars. */
const CODE_LENGTH = 8;

function normalizeCode(raw: string): string {
  return raw
    .replace(/[^a-zA-Z0-9]/g, "")
    .toUpperCase()
    .slice(0, CODE_LENGTH);
}

function nativePlatform(): AcceptBody["platform"] {
  if (Platform.OS === "ios" || Platform.OS === "android") {
    return Platform.OS;
  }
  return "web";
}

/** `PreviewInvitationResponse.role` is a plain string on the wire. */
function roleLabel(role: string): string {
  return role in ROLE_LABELS
    ? ROLE_LABELS[role as keyof typeof ROLE_LABELS]
    : role;
}

function formatDate(iso: string): string {
  const date = new Date(iso);
  const day = String(date.getDate()).padStart(2, "0");
  const month = String(date.getMonth() + 1).padStart(2, "0");
  return `${day}/${month}/${date.getFullYear()}`;
}

type Step =
  | { kind: "code" }
  | { kind: "preview"; code: string; preview: Preview }
  | { kind: "accept"; code: string; preview: Preview };

/**
 * Invitation-code entry (Stitch mobile "Código de invitación", screen
 * `4b98138c2d4747a7bd0567e5ac174f2a`, then "Invitación reconocida", screen
 * `607f8aeb9682403987dc2b297615be76`, project `11075381530582947267`).
 * Reached from both `login-invitation-link` and `portal-invitation-link`.
 *
 * The whole flow is the existing sessionless backend one, three steps held
 * in local state on ONE route — never as route params, because the short
 * code is a credential and a URL would write it into history and logs (the
 * same reason the preview endpoint is POST, design D-6):
 *
 * 1. Code: 8 alphanumeric characters, uppercased as typed; "Continuar"
 *    enables at 8. `POST /v1/invitations/preview`; any failure shows one
 *    generic message (the endpoint is deliberately not an existence oracle).
 * 2. Preview: community, unit (only that one is assigned — the response
 *    carries its id, not a label), role and expiry, BEFORE any account
 *    action. It never shows the invited email: preview does not return it
 *    (design D-6). "No soy yo" leaves without calling anything.
 * 3. Accept: `POST /v1/auth/accept-invitation` with name, password,
 *    consent and platform. The backend creates an account for the invited
 *    address or, when one exists, links it only with that account's own
 *    password — and its TOTP code when it answers `AUTH_MFA_REQUIRED`, in
 *    which case the code field appears and the user retries. `name` is
 *    required by the contract on both branches (ignored when linking);
 *    `GET /v1/me` carries no name to prefill it from. The returned session
 *    replaces any current one exactly as `LoginScreen` stores a login, and
 *    the user lands on `/portal`. Any other failure: one generic message.
 */
export function InvitationScreen() {
  const theme = useTheme();
  const router = useRouter();
  const { width } = useWindowDimensions();
  const isDesktop = width >= BREAKPOINT_DESKTOP;
  const activeTheme: MD3Theme = isDesktop ? lightTheme : theme;

  const [step, setStep] = useState<Step>({ kind: "code" });
  const [code, setCode] = useState("");
  const [codeError, setCodeError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [passwordVisible, setPasswordVisible] = useState(false);
  const [consent, setConsent] = useState(false);
  const [totpCode, setTotpCode] = useState("");
  const [mfaRequired, setMfaRequired] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<{
    name?: boolean;
    password?: boolean;
    consent?: boolean;
  }>({});
  const [acceptError, setAcceptError] = useState<string | null>(null);

  function leave() {
    if (router.canGoBack()) {
      router.back();
    } else {
      router.replace("/(auth)/login");
    }
  }

  async function submitCode() {
    if (code.length !== CODE_LENGTH) {
      return;
    }
    setCodeError(null);
    setSubmitting(true);
    try {
      const { data, error } = await apiClient.POST("/v1/invitations/preview", {
        body: { short_code: code },
      });
      if (error || !data) {
        setCodeError(invitationPreviewErrorMessage());
        return;
      }
      setStep({ kind: "preview", code, preview: data });
    } catch {
      setCodeError(invitationPreviewErrorMessage());
    } finally {
      setSubmitting(false);
    }
  }

  async function submitAccept(shortCode: string) {
    setAcceptError(null);
    const body: AcceptBody = {
      short_code: shortCode,
      name,
      password,
      consent: true,
      platform: nativePlatform(),
      ...(mfaRequired && totpCode ? { totp_code: totpCode } : {}),
    };
    const parsed = schemas.AcceptInvitationRequest.safeParse(body);
    const invalid = new Set(
      parsed.success ? [] : parsed.error.issues.map((issue) => issue.path[0]),
    );
    const errors = {
      name: invalid.has("name"),
      password: invalid.has("password"),
      consent: !consent,
    };
    setFieldErrors(errors);
    if (errors.name || errors.password || errors.consent) {
      return;
    }

    setSubmitting(true);
    try {
      const { data, error } = await apiClient.POST(
        "/v1/auth/accept-invitation",
        { body },
      );
      if (error || !data) {
        // huma's generic `ErrorModel` vs. the real `{code, message,
        // details}` body — see `LoginScreen.tsx`'s identical cast.
        const apiError = (error ?? {}) as unknown as ApiErrorBody;
        if (apiError.code === ErrorCode.AuthMFARequired) {
          setMfaRequired(true);
        }
        setAcceptError(invitationAcceptErrorMessage(apiError));
        return;
      }
      setSession({
        accessToken: data.access_token,
        csrfToken: data.csrf_token ?? null,
      });
      if (data.refresh_token) {
        await persistRefreshToken(data.refresh_token);
      }
      router.replace("/portal");
    } catch {
      setAcceptError(
        invitationAcceptErrorMessage({
          code: ErrorCode.InternalError,
          message: "",
        }),
      );
    } finally {
      setSubmitting(false);
    }
  }

  const cardStyle = [
    styles.card,
    isDesktop
      ? { backgroundColor: WEB_SURFACE, borderColor: WEB_BORDER }
      : {
          backgroundColor: activeTheme.colors.surface,
          borderColor: activeTheme.colors.outline,
        },
  ];
  const muted = { color: activeTheme.colors.onSurfaceVariant };

  function backButton(onPress: () => void) {
    return (
      <Pressable
        accessibilityRole="button"
        accessibilityLabel="Volver"
        testID="invitation-back"
        onPress={onPress}
        style={styles.backButton}
      >
        <MaterialCommunityIcons
          name="arrow-left"
          size={24}
          color={activeTheme.colors.onSurface}
        />
      </Pressable>
    );
  }

  let content: React.ReactNode;

  if (step.kind === "code") {
    content = (
      <>
        {backButton(leave)}
        <View style={cardStyle}>
          <MaterialCommunityIcons
            name="email-check-outline"
            size={32}
            color={activeTheme.colors.primary}
          />
          <Text variant="headlineSmall" style={styles.title}>
            Tu administrador te ha invitado
          </Text>
          <Text variant="bodyMedium" style={[styles.subtitle, muted]}>
            Introduce el código alfanumérico facilitado por la administración
            para vincular tu inmueble.
          </Text>
          <FormTextInput
            testID="invitation-code-input"
            label="Código de acceso alfanumérico"
            value={code}
            onChangeText={(text) => {
              setCode(normalizeCode(text));
              setCodeError(null);
            }}
            autoCapitalize="characters"
            autoCorrect={false}
            maxLength={CODE_LENGTH}
            style={styles.input}
          />
          {codeError ? (
            <HelperText type="error" visible testID="invitation-code-error">
              {codeError}
            </HelperText>
          ) : (
            <HelperText type="info" visible>
              Lo encontrarás en la convocatoria impresa o en el correo de
              bienvenida remitido por tu administrador. El código tiene 8
              caracteres.
            </HelperText>
          )}
          <PrimaryButton
            testID="invitation-code-submit"
            accessibilityLabel="Continuar"
            onPress={submitCode}
            loading={submitting}
            disabled={submitting || code.length !== CODE_LENGTH}
            style={styles.submit}
          >
            Continuar
          </PrimaryButton>
          <View style={styles.noInvitation}>
            <Text variant="labelMedium" style={muted}>
              ¿No has recibido la invitación?
            </Text>
            <Text variant="labelSmall" style={muted}>
              Contacta con tu administración de fincas.
            </Text>
          </View>
        </View>
      </>
    );
  } else if (step.kind === "preview") {
    const { preview } = step;
    content = (
      <>
        {backButton(() => setStep({ kind: "code" }))}
        <View style={cardStyle} testID="invitation-preview">
          <View style={styles.validBadge}>
            <MaterialCommunityIcons
              name="check-decagram"
              size={16}
              color={activeTheme.colors.primary}
            />
            <Text
              variant="labelMedium"
              style={{ color: activeTheme.colors.primary }}
            >
              Código válido
            </Text>
          </View>
          <Text variant="headlineSmall" style={styles.title}>
            Invitación verificada
          </Text>
          <Text variant="bodyMedium" style={[styles.subtitle, muted]}>
            Comprueba la información del inmueble asignado antes de formalizar
            el alta.
          </Text>
          <View style={styles.detailRow}>
            <Text variant="labelSmall" style={muted}>
              Comunidad asignada
            </Text>
            <Text variant="titleMedium">{preview.community_name}</Text>
          </View>
          {preview.unit_id ? (
            <View style={styles.detailRow}>
              <Text variant="labelSmall" style={muted}>
                Inmueble
              </Text>
              <Text variant="bodyLarge">Asignado por tu administración</Text>
            </View>
          ) : null}
          <View style={styles.detailRow}>
            <Text variant="labelSmall" style={muted}>
              Titularidad
            </Text>
            <Text variant="bodyLarge">{roleLabel(preview.role)}</Text>
          </View>
          <Text variant="labelSmall" style={[styles.detailRow, muted]}>
            {`Válida hasta el ${formatDate(preview.expires_at)}`}
          </Text>
          <PrimaryButton
            testID="invitation-preview-continue"
            accessibilityLabel="Crear mi cuenta"
            onPress={() =>
              setStep({ kind: "accept", code: step.code, preview })
            }
            style={styles.submit}
          >
            Crear mi cuenta
          </PrimaryButton>
          <Pressable
            accessibilityRole="button"
            testID="invitation-preview-not-me"
            onPress={leave}
            style={styles.textAction}
          >
            <Text
              variant="labelLarge"
              style={{ color: activeTheme.colors.primary }}
            >
              No soy yo
            </Text>
          </Pressable>
          <Text variant="labelSmall" style={muted}>
            Al continuar vincularás tu identidad a las actas, recibos y
            convocatorias oficiales de esta comunidad conforme a la Ley de
            Propiedad Horizontal.
          </Text>
        </View>
      </>
    );
  } else {
    const shortCode = step.code;
    content = (
      <>
        {backButton(() =>
          setStep({ kind: "preview", code: step.code, preview: step.preview }),
        )}
        <View style={cardStyle} testID="invitation-accept-form">
          <Text variant="headlineSmall" style={styles.title}>
            Crear mi cuenta
          </Text>
          <Text variant="bodyMedium" style={[styles.subtitle, muted]}>
            Si ya tienes una cuenta con el correo al que se envió la invitación,
            introduce su contraseña para vincularla.
          </Text>
          <View style={styles.field}>
            <FormTextInput
              testID="invitation-accept-name"
              label="Nombre y apellidos"
              value={name}
              onChangeText={setName}
              autoComplete="name"
              style={styles.input}
            />
            {fieldErrors.name ? (
              <HelperText type="error" visible>
                Introduce tu nombre.
              </HelperText>
            ) : null}
          </View>
          <View style={styles.field}>
            <FormTextInput
              testID="invitation-accept-password"
              label="Contraseña"
              value={password}
              onChangeText={setPassword}
              secureTextEntry={!passwordVisible}
              style={styles.input}
              right={
                <TextInput.Icon
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
            {fieldErrors.password ? (
              <HelperText type="error" visible>
                La contraseña debe tener al menos 12 caracteres.
              </HelperText>
            ) : (
              <HelperText type="info" visible>
                Mínimo 12 caracteres.
              </HelperText>
            )}
          </View>
          {mfaRequired ? (
            <View style={styles.field}>
              <FormTextInput
                testID="invitation-accept-totp"
                label="Código de verificación"
                value={totpCode}
                onChangeText={setTotpCode}
                keyboardType="number-pad"
                autoComplete="one-time-code"
                style={styles.input}
              />
            </View>
          ) : null}
          <Pressable
            accessibilityRole="checkbox"
            accessibilityState={{ checked: consent }}
            testID="invitation-accept-consent"
            onPress={() => setConsent((value) => !value)}
            style={styles.consentRow}
          >
            <MaterialCommunityIcons
              name={consent ? "checkbox-marked" : "checkbox-blank-outline"}
              size={24}
              color={activeTheme.colors.primary}
            />
            <Text variant="bodyMedium" style={styles.consentText}>
              Acepto el tratamiento de mis datos para gestionar mi relación con
              esta comunidad.
            </Text>
          </Pressable>
          {fieldErrors.consent ? (
            <HelperText
              type="error"
              visible
              testID="invitation-accept-consent-error"
            >
              Debes aceptar para continuar.
            </HelperText>
          ) : null}
          {acceptError ? (
            <HelperText type="error" visible testID="invitation-accept-error">
              {acceptError}
            </HelperText>
          ) : null}
          <PrimaryButton
            testID="invitation-accept-submit"
            accessibilityLabel="Aceptar invitación"
            onPress={() => submitAccept(shortCode)}
            loading={submitting}
            disabled={submitting}
            style={styles.submit}
          >
            Aceptar invitación
          </PrimaryButton>
        </View>
      </>
    );
  }

  const body = (
    <ScrollView contentContainerStyle={styles.scrollContent}>
      <View style={styles.formWrap}>{content}</View>
    </ScrollView>
  );

  if (isDesktop) {
    return (
      <View
        testID="invitation-screen"
        style={[styles.container, { backgroundColor: WEB_PAGE_BACKGROUND }]}
      >
        <PaperProvider theme={lightTheme}>
          <AuthHeader />
          {body}
        </PaperProvider>
      </View>
    );
  }

  return (
    <View
      testID="invitation-screen"
      style={[styles.container, { backgroundColor: theme.colors.background }]}
    >
      {body}
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
    gap: spacing.space8,
    width: "100%",
    maxWidth: 420,
  },
  backButton: {
    alignItems: "center",
    justifyContent: "center",
    minHeight: MIN_TOUCH_TARGET,
    minWidth: MIN_TOUCH_TARGET,
  },
  card: {
    borderRadius: 12,
    borderWidth: 1,
    gap: spacing.space8,
    padding: spacing.space16,
  },
  title: {
    marginTop: spacing.space4,
  },
  subtitle: {
    marginBottom: spacing.space8,
  },
  field: {
    marginBottom: spacing.space4,
  },
  input: {
    minHeight: MIN_TOUCH_TARGET,
  },
  submit: {
    marginTop: spacing.space8,
    minHeight: MIN_TOUCH_TARGET,
    justifyContent: "center",
  },
  noInvitation: {
    alignItems: "center",
    gap: spacing.space4,
    marginTop: spacing.space16,
  },
  validBadge: {
    alignItems: "center",
    flexDirection: "row",
    gap: spacing.space4,
  },
  detailRow: {
    gap: spacing.space4,
    marginBottom: spacing.space8,
  },
  textAction: {
    alignItems: "center",
    justifyContent: "center",
    minHeight: MIN_TOUCH_TARGET,
  },
  consentRow: {
    alignItems: "center",
    flexDirection: "row",
    gap: spacing.space8,
    minHeight: MIN_TOUCH_TARGET,
  },
  consentText: {
    flex: 1,
  },
});
