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
