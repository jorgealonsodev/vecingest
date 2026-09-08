package mail

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
)

// templateFS embeds the two html/template templates design.md's file
// table (line 473) mandates for internal/mail: password_reset.html and
// login_lockout.html (D-N). html/template, not text/template, so any
// future untrusted field is escaped automatically -- RawToken here is
// server-generated, but the escaping is free and the right default.
//
//go:embed templates/*.html
var templateFS embed.FS

var (
	passwordResetTemplate = template.Must(template.ParseFS(templateFS, "templates/password_reset.html"))
	loginLockoutTemplate  = template.Must(template.ParseFS(templateFS, "templates/login_lockout.html"))
)

// PasswordResetSubject is the password-reset email's fixed subject
// line (D-N). The login-lockout alert has no equivalent constant here:
// its subject is authored once, in lockout.Message.Subject, and
// RenderLoginLockoutAlert only renders the HTML body around it.
const PasswordResetSubject = "Password reset requested"

// PasswordResetData is what password_reset.html renders.
type PasswordResetData struct {
	// RawToken is the opaque, one-time password-reset token (never the
	// hash). auth-credentials's 1-hour reset-token expiry (task 5.21)
	// is stated in the template body as a fixed string, not templated
	// from a duration, since it is a fixed PRD value, not a runtime one.
	RawToken string
}

// RenderPasswordReset renders the password-reset alert email body.
func RenderPasswordReset(data PasswordResetData) (subject, body string, err error) {
	var buf bytes.Buffer
	if err := passwordResetTemplate.Execute(&buf, data); err != nil {
		return "", "", fmt.Errorf("mail: render password_reset.html: %w", err)
	}
	return PasswordResetSubject, buf.String(), nil
}

// LoginLockoutData is what login_lockout.html renders. Message is the
// domain-authored alert text (lockout.Message.Body) -- internal/mail
// wraps it in the branded HTML shell rather than re-authoring the copy,
// so the lockout domain package stays the single source of truth for
// what the alert says.
type LoginLockoutData struct {
	Message string
}

// RenderLoginLockoutAlert renders the progressive-lockout alert email
// body (auth-credentials: Progressive Lockout; D-N "the alert"). The
// subject is not this function's concern -- see LoginLockoutData.
func RenderLoginLockoutAlert(data LoginLockoutData) (body string, err error) {
	var buf bytes.Buffer
	if err := loginLockoutTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("mail: render login_lockout.html: %w", err)
	}
	return buf.String(), nil
}
