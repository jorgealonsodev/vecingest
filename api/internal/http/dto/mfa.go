package dto

// MFAEnrollInput is POST /v1/me/mfa/enroll's input (auth-mfa-totp delta:
// Non-Superadmin TOTP HTTP Endpoints). Bearer-authenticated, no body:
// the caller is always the authenticated user's OWN account, never a
// path/body-supplied user id.
type MFAEnrollInput struct {
	Authorization string `header:"Authorization"`
}

// MFAEnrollResponse carries the freshly generated secret, shown to the
// caller exactly once. Neither value is ever persisted in this form:
// user_mfa stores only mfa.EncryptSecret's output (rules.apply.guidelines:
// never persist a TOTP secret in plaintext).
type MFAEnrollResponse struct {
	Secret          string `json:"secret" doc:"Base32-encoded TOTP secret, for manual entry into an authenticator app."`
	ProvisioningURI string `json:"provisioning_uri" doc:"otpauth:// URI an authenticator app can scan as a QR code."`
}

type MFAEnrollOutput struct {
	Body MFAEnrollResponse
}

// MFAVerifyRequest is POST /v1/me/mfa/verify's body. EmailCode is
// optional in the schema because the same endpoint serves two calls:
// the handler requires it on the call that confirms an enrollment and
// ignores it once the factor is active.
type MFAVerifyRequest struct {
	Code      string `json:"code" minLength:"6" maxLength:"6" doc:"6-digit TOTP code."`
	EmailCode string `json:"email_code,omitempty" minLength:"6" maxLength:"6" doc:"6-digit code emailed to the account's address by the enroll call. Required to confirm an enrollment; ignored once TOTP is active."`
}

type MFAVerifyInput struct {
	Authorization string `header:"Authorization"`
	Body          MFAVerifyRequest
}

// MFAVerifyResponse reports whether TOTP is now active. RecoveryCodes is
// populated EXACTLY ONCE: the call that transitions enabled_at from NULL
// to a real value (auth-mfa-totp: One-Time Recovery Codes) -- a
// subsequent verify against an already-active factor never repeats
// them.
type MFAVerifyResponse struct {
	Active        bool     `json:"active"`
	RecoveryCodes []string `json:"recovery_codes,omitempty" doc:"Present only on the call that activates TOTP for the first time. Shown exactly once; store them safely."`
}

type MFAVerifyOutput struct {
	Body MFAVerifyResponse
}
