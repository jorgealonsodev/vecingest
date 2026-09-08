// Stable API error codes (task 7.3).
//
// This file is the ONE deliberate exception to "no type in packages/shared
// MAY be hand-written" (api-contract-generation: Generation Chain). It is
// hand-authored, not generated, because these codes cannot be derived from
// `openapi.yaml`: huma documents error responses with its generic RFC 7807
// `ErrorModel`, which has no field for a stable machine-readable code. The
// real runtime error body is `{"code": "...", "message": "...", "details":
// {...}}`, written by `internal/http/apperr.Error` (see that package's own
// header comment: "packages/shared re-exports these codes once Phase 7
// generates the client").
//
// Source of truth: `api/internal/http/apperr/apperr.go` (most codes) and
// `api/internal/domain/auth/password/policy.go` (the two password-length
// codes and the HIBP-breach code, D-F). Keep this list in sync by hand
// whenever a Go constant is added, renamed, or removed — renaming a
// shipped code is a breaking change for every generated client.
export const ErrorCode = {
  AuthInvalidCredentials: "AUTH_INVALID_CREDENTIALS",
  AuthPasswordTooShortNoMFA: "AUTH_PASSWORD_TOO_SHORT_NO_MFA",
  AuthPasswordTooShortWithMFA: "AUTH_PASSWORD_TOO_SHORT_WITH_MFA",
  AuthPasswordBreached: "AUTH_PASSWORD_BREACHED",
  AuthCSRFInvalid: "AUTH_CSRF_INVALID",
  AuthAmbiguousTokenTransport: "AUTH_AMBIGUOUS_TOKEN_TRANSPORT",
  AuthMissingTokenTransport: "AUTH_MISSING_TOKEN_TRANSPORT",
  AuthRefreshReused: "AUTH_REFRESH_REUSED",
  AuthRefreshInvalid: "AUTH_REFRESH_INVALID",
  AuthOriginInvalid: "AUTH_ORIGIN_INVALID",
  AuthTOTPReplayed: "AUTH_TOTP_REPLAYED",
  AuthTOTPInvalid: "AUTH_TOTP_INVALID",
  AuthTooManyAttempts: "AUTH_TOO_MANY_ATTEMPTS",
  AuthMFAEnrollmentRequired: "AUTH_MFA_ENROLLMENT_REQUIRED",
  AuthResetTokenInvalid: "AUTH_RESET_TOKEN_INVALID",
  AuthUnauthorized: "AUTH_UNAUTHORIZED",
  NotFound: "NOT_FOUND",
  ValidationError: "VALIDATION_ERROR",
  InternalError: "INTERNAL_ERROR",
} as const;

export type ErrorCode = (typeof ErrorCode)[keyof typeof ErrorCode];

/** The API's stable error envelope shape (`internal/http/apperr.Error`). */
export interface ApiErrorBody {
  code: ErrorCode;
  message: string;
  details?: Record<string, unknown>;
}
