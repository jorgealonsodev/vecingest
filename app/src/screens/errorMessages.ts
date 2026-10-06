import { type ApiErrorBody, ErrorCode } from "@vecingest/shared/errors";

/**
 * Maps the server's stable error codes to Spanish, sentence-case UI copy.
 *
 * The password-length rule is server-authoritative (design.md D-F): whether
 * an account has TOTP active is exactly the fact a client-side conditional
 * would leak, so the two password-too-short codes travel the applicable
 * rule as `details.min_length` data, interpolated here, never as a second
 * client-side length check.
 */
export function loginErrorMessage(error: ApiErrorBody): string {
  switch (error.code) {
    case ErrorCode.AuthInvalidCredentials:
      return "El correo o la contraseña no son correctos.";
    case ErrorCode.AuthPasswordTooShortNoMFA:
      return `La contraseña debe tener al menos ${minLength(error)} caracteres.`;
    case ErrorCode.AuthPasswordTooShortWithMFA:
      return `La contraseña debe tener al menos ${minLength(error)} caracteres al tener la verificación en dos pasos activada.`;
    case ErrorCode.AuthTooManyAttempts:
      return "Se ha bloqueado el acceso temporalmente por demasiados intentos.";
    case ErrorCode.AuthMFAEnrollmentRequired:
      return "Esta cuenta requiere activar la verificación en dos pasos.";
    case ErrorCode.AuthMFARequired:
      return "Esta cuenta tiene la verificación en dos pasos activada: introduce el código de tu aplicación de autenticación.";
    default:
      return "No se ha podido iniciar sesión. Inténtalo de nuevo.";
  }
}

function minLength(error: ApiErrorBody): string {
  const value = error.details?.min_length;
  return typeof value === "number" ? String(value) : "";
}

/**
 * `POST /v1/auth/forgot-password` never reveals whether the email exists
 * (`ForgotPasswordResponse` is just `{accepted: boolean}`), so the only
 * errors this ever maps are transport/abuse failures, never "not found".
 */
export function forgotPasswordErrorMessage(error: ApiErrorBody): string {
  switch (error.code) {
    case ErrorCode.AuthTooManyAttempts:
      return "Se ha bloqueado el acceso temporalmente por demasiados intentos.";
    default:
      return "No se ha podido procesar la solicitud. Inténtalo de nuevo.";
  }
}

export function resetPasswordErrorMessage(error: ApiErrorBody): string {
  switch (error.code) {
    case ErrorCode.AuthResetTokenInvalid:
      return "El enlace no es válido o ha caducado. Solicita uno nuevo.";
    case ErrorCode.AuthPasswordBreached:
      return "Esta contraseña aparece en filtraciones conocidas. Elige otra.";
    case ErrorCode.AuthPasswordTooShortNoMFA:
      return `La contraseña debe tener al menos ${minLength(error)} caracteres.`;
    case ErrorCode.AuthPasswordTooShortWithMFA:
      return `La contraseña debe tener al menos ${minLength(error)} caracteres al tener la verificación en dos pasos activada.`;
    case ErrorCode.AuthTooManyAttempts:
      return "Se ha bloqueado el acceso temporalmente por demasiados intentos.";
    default:
      return "No se ha podido restablecer la contraseña. Inténtalo de nuevo.";
  }
}

/**
 * `POST /v1/invitations/preview` answers every unusable code (unknown,
 * expired, revoked, locked out) with the same generic body so it is not an
 * existence oracle (design D-6); the UI keeps that property by showing one
 * message for every failure, whatever the code.
 */
export function invitationPreviewErrorMessage(): string {
  return "El código no es válido o ha caducado.";
}

/**
 * `POST /v1/auth/accept-invitation`: `AUTH_MFA_REQUIRED` is the one actionable
 * answer (the invited address has an account with an active second factor),
 * so the screen asks for the code and retries. Everything else — wrong
 * password for an existing account, password policy, used or expired
 * invitation, lockout — collapses into one generic message, so the screen
 * never discloses whether the invited address already has an account.
 */
export function invitationAcceptErrorMessage(error: ApiErrorBody): string {
  if (error.code === ErrorCode.AuthMFARequired) {
    return "Esta cuenta tiene la verificación en dos pasos activada: introduce el código de tu aplicación de autenticación.";
  }
  return "No se ha podido aceptar la invitación. Revisa los datos e inténtalo de nuevo.";
}
