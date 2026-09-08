package password

import (
	"context"
	"fmt"
	"unicode/utf8"
)

// Stable error codes surfaced to the API layer (D-F). packages/shared
// re-exports these once Phase 7 generates the client; they are defined
// here because internal/domain/auth is their single authority.
const (
	ErrCodeTooShortNoMFA   = "AUTH_PASSWORD_TOO_SHORT_NO_MFA"
	ErrCodeTooShortWithMFA = "AUTH_PASSWORD_TOO_SHORT_WITH_MFA"
	ErrCodeBreached        = "AUTH_PASSWORD_BREACHED"
)

// The conditional length floor (NIST SP 800-63B rev.4; auth-credentials:
// Conditional Password Length Floor).
const (
	MinLengthNoMFA   = 15
	MinLengthWithMFA = 12
)

// PolicyError is what PasswordPolicy returns for a rejected password. It
// carries a stable Code and Details (min_length, rule) so the API layer
// can return `details: { min_length, rule }` without a second
// implementation of the rule (D-F).
type PolicyError struct {
	Code    string
	Details map[string]any
}

func (e *PolicyError) Error() string {
	return fmt.Sprintf("%s: %v", e.Code, e.Details)
}

// HIBPChecker checks whether a password appears in the Have I Been
// Pwned breach corpus. err != nil signals a transport-level failure
// (network error, unexpected status) as distinct from a definitive
// breached/not-breached answer -- PasswordPolicy.Validate fails open
// exactly on that distinction (auth-credentials: HIBP k-Anonymity Breach
// Check). The concrete implementation lives in internal/platform/hibp,
// which is a platform adapter for this domain-owned port.
type HIBPChecker interface {
	IsBreached(ctx context.Context, password string) (breached bool, err error)
}

// PasswordPolicy is the single authoritative definition of what makes a
// password acceptable (D-F): the conditional length floor, plus an
// optional HIBP breach check. The huma request struct's static
// `minLength:12` tag (and the Zod schema generated from it) is only the
// absolute floor for offline validation; this type is the dynamic,
// authoritative check that actually decides "15 or 12".
type PasswordPolicy struct {
	// HIBP is optional: nil skips the breach check entirely (e.g. the
	// seed command's fixture users, which deliberately bypass HIBP and
	// the length floor via their own package -- D-S).
	HIBP HIBPChecker

	// WarnLog receives a WARN-level message when the HIBP check fails
	// open. It is a plain function (not *slog.Logger) so this package
	// does not need to depend on how the caller structures logging; a
	// nil WarnLog simply skips logging.
	WarnLog func(ctx context.Context, msg string, args ...any)

	// OnHIBPUnavailable is invoked when the HIBP check fails open. It is
	// the seam this package exposes for writing the required audit_log
	// entry: internal/domain/audit (Phase 4) is what a caller wires in
	// here once it exists. A nil hook simply skips that side effect.
	OnHIBPUnavailable func(ctx context.Context)
}

// CheckLength enforces the conditional length floor: 15 characters
// without an active second factor, 12 with TOTP active. Length is
// counted in runes, not bytes, so multi-byte characters are not
// penalized.
func (p PasswordPolicy) CheckLength(candidate string, totpActive bool) error {
	minLength := MinLengthNoMFA
	code := ErrCodeTooShortNoMFA
	rule := "minimum 15 characters without an active second factor"
	if totpActive {
		minLength = MinLengthWithMFA
		code = ErrCodeTooShortWithMFA
		rule = "minimum 12 characters with TOTP active"
	}

	if utf8.RuneCountInString(candidate) < minLength {
		return &PolicyError{
			Code: code,
			Details: map[string]any{
				"min_length": minLength,
				"rule":       rule,
			},
		}
	}
	return nil
}

// Validate runs the full policy: the length floor, then (when HIBP is
// configured) the breach check. On an HIBP transport error, Validate
// fails open: the password is accepted on the length floor alone, a
// WARN is logged, and the audit hook fires -- the length floor and
// Argon2id hashing still hold, so a third-party outage cannot block
// registration or password recovery (auth-credentials: HIBP
// k-Anonymity Breach Check).
func (p PasswordPolicy) Validate(ctx context.Context, candidate string, totpActive bool) error {
	if err := p.CheckLength(candidate, totpActive); err != nil {
		return err
	}

	if p.HIBP == nil {
		return nil
	}

	breached, err := p.HIBP.IsBreached(ctx, candidate)
	if err != nil {
		if p.WarnLog != nil {
			p.WarnLog(ctx, "HIBP check unavailable, failing open", "error", err)
		}
		if p.OnHIBPUnavailable != nil {
			p.OnHIBPUnavailable(ctx)
		}
		return nil
	}

	if breached {
		return &PolicyError{
			Code: ErrCodeBreached,
			Details: map[string]any{
				"rule": "password found in a known breach corpus",
			},
		}
	}
	return nil
}
