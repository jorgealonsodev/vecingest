// Package dto declares the huma-facing input/output structs. These
// structs ARE the OpenAPI generation source (design's technical
// approach: "Go structs are the single source"), so every field here
// eventually reaches api/openapi/openapi.yaml, the generated TS client
// and the generated Zod schema.
package dto

import "net/http"

// LoginRequest is POST /v1/auth/login's body.
type LoginRequest struct {
	Email      string `json:"email" format:"email" doc:"Account email address."`
	Password   string `json:"password" minLength:"12" doc:"Absolute floor is 12; the applicable 12-or-15 rule is enforced dynamically server-side (D-F)."`
	DeviceName string `json:"device_name,omitempty" maxLength:"255"`
	Platform   string `json:"platform" enum:"ios,android,web" doc:"Determines refresh-token transport: web gets an HttpOnly cookie, ios/android get it in the response body."`
}

// LoginInput wraps the login request body.
type LoginInput struct {
	Body LoginRequest
}

// LoginResponse is the shared success shape for login, superadmin login
// and refresh. RefreshToken is populated only for the body transport
// (native platforms); CSRFToken is populated only for the cookie
// transport (web) (design D-D/D-E).
type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int64  `json:"expires_in" doc:"Access-token lifetime in seconds."`
	RefreshToken string `json:"refresh_token,omitempty" doc:"Present only on the body (native) transport."`
	CSRFToken    string `json:"csrf_token,omitempty" doc:"Present only on the cookie (web) transport."`
}

// LoginOutput wraps LoginResponse with the conditional Set-Cookie
// header. SetCookie is the zero value (empty Name) when the login used
// the body transport, so huma's writeHeader skips an empty string
// value and emits no Set-Cookie header at all.
type LoginOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      LoginResponse
}

// RefreshRequest is POST /v1/auth/refresh's body: the native (body)
// transport's refresh token. The web transport instead presents the
// cookie, read via the input struct's RefreshCookie field.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token,omitempty"`
}

// RefreshInput carries every transport surface /v1/auth/refresh reads
// from (auth-session-tokens: Refresh Transport Mutual Exclusion;
// auth-csrf-origin: CSRF Token Delivered In-Body, Never By Cookie).
type RefreshInput struct {
	RefreshCookie http.Cookie `cookie:"vecingest_refresh"`
	XCSRFToken    string      `header:"X-CSRF-Token"`
	Origin        string      `header:"Origin"`
	Body          RefreshRequest
}

// RefreshOutput mirrors LoginOutput.
type RefreshOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      LoginResponse
}

// CSRFTokenResponse is GET /v1/auth/refresh/csrf's body.
type CSRFTokenResponse struct {
	CSRFToken string `json:"csrf_token"`
}

// CSRFTokenOutput carries the three mandatory response headers
// (auth-csrf-origin: CSRF Token Recovery Endpoint).
type CSRFTokenOutput struct {
	CacheControl string `header:"Cache-Control"`
	Vary         string `header:"Vary"`
	CORP         string `header:"Cross-Origin-Resource-Policy"`
	Body         CSRFTokenResponse
}

// LogoutInput is POST /v1/auth/logout's input: Bearer-authenticated,
// no body, no CSRF requirement (auth-session-tokens: Bearer-Authenticated
// Logout).
type LogoutInput struct {
	Authorization string `header:"Authorization"`
}

// LogoutOutput always clears the refresh cookie.
type LogoutOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
}

// ForgotPasswordRequest is POST /v1/auth/forgot-password's body.
type ForgotPasswordRequest struct {
	Email string `json:"email" format:"email"`
}

// ForgotPasswordInput wraps the body.
type ForgotPasswordInput struct {
	Body ForgotPasswordRequest
}

// ForgotPasswordResponse is identical regardless of whether the email
// is registered (auth-credentials: Enumeration-Safe Auth Responses).
type ForgotPasswordResponse struct {
	Accepted bool `json:"accepted"`
}

type ForgotPasswordOutput struct {
	Body ForgotPasswordResponse
}

// ResetPasswordRequest is POST /v1/auth/reset-password's body.
type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password" minLength:"12"`
}

type ResetPasswordInput struct {
	Body ResetPasswordRequest
}

type ResetPasswordResponse struct {
	Accepted bool `json:"accepted"`
}

type ResetPasswordOutput struct {
	Body ResetPasswordResponse
}

// SuperadminLoginRequest is POST /v1/auth/superadmin/login's body: the
// same credentials plus a mandatory TOTP code (auth-mfa-totp: Mandatory
// TOTP for Superadmin).
type SuperadminLoginRequest struct {
	Email      string `json:"email" format:"email"`
	Password   string `json:"password" minLength:"12"`
	TOTPCode   string `json:"totp_code,omitempty" doc:"Required unless the response is 403 MFA enrollment-required."`
	DeviceName string `json:"device_name,omitempty" maxLength:"255"`
	Platform   string `json:"platform" enum:"ios,android,web"`
}

type SuperadminLoginInput struct {
	Body SuperadminLoginRequest
}

type SuperadminLoginOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      LoginResponse
}
