// Package apperr defines the stable, cross-cutting error-code envelope
// every M0 handler returns (design D-F/D-D/D-E). huma writes a returned
// huma.StatusError's own value as the response body via content
// negotiation, so this type controls the exact JSON shape
// (`{"code": "...", "message": "...", "details": {...}}`) without going
// through huma's own generic ErrorModel, which has no room for a stable
// machine-readable code. packages/shared re-exports these codes once
// Phase 7 generates the client.
package apperr

// Stable error codes. Every one of these MUST be stable: once shipped,
// renaming a code is a breaking API change for every generated client.
const (
	CodeInvalidCredentials      = "AUTH_INVALID_CREDENTIALS" //nolint:gosec // G101: a stable machine-readable error CODE, not a credential value -- gosec pattern-matches the substring "Credentials" in the identifier
	CodeCSRFInvalid             = "AUTH_CSRF_INVALID"
	CodeAmbiguousTokenTransport = "AUTH_AMBIGUOUS_TOKEN_TRANSPORT" //nolint:gosec // G101: false positive -- "Token" substring match, this is an error code string, not a secret
	CodeMissingTokenTransport   = "AUTH_MISSING_TOKEN_TRANSPORT"   //nolint:gosec // G101: false positive -- "Token" substring match, this is an error code string, not a secret
	CodeRefreshReused           = "AUTH_REFRESH_REUSED"
	CodeRefreshInvalid          = "AUTH_REFRESH_INVALID"
	CodeOriginInvalid           = "AUTH_ORIGIN_INVALID"
	CodeTOTPReplayed            = "AUTH_TOTP_REPLAYED"
	CodeTOTPInvalid             = "AUTH_TOTP_INVALID"
	CodeTOTPThrottled           = "AUTH_TOO_MANY_ATTEMPTS"
	CodeTooManyAttempts         = "AUTH_TOO_MANY_ATTEMPTS"
	CodeMFAEnrollmentRequired   = "AUTH_MFA_ENROLLMENT_REQUIRED"
	CodeResetTokenInvalid       = "AUTH_RESET_TOKEN_INVALID" //nolint:gosec // G101: false positive -- "Token" substring match, this is an error code string, not a secret
	CodeUnauthorized            = "AUTH_UNAUTHORIZED"
	CodeNotFound                = "NOT_FOUND"
	CodeValidation              = "VALIDATION_ERROR"
	CodeInternal                = "INTERNAL_ERROR"
)

// Error is the stable envelope. It implements huma.StatusError
// structurally (GetStatus() int, Error() string) without importing huma,
// keeping this package free of an HTTP-framework dependency.
type Error struct {
	StatusCode int            `json:"-"`
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	Details    map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string  { return e.Message }
func (e *Error) GetStatus() int { return e.StatusCode }

// New builds an *Error. details may be nil.
func New(status int, code, message string, details map[string]any) *Error {
	return &Error{StatusCode: status, Code: code, Message: message, Details: details}
}
