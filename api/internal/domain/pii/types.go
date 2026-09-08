// Package pii declares the distinct, never-bare-string types D-J's leg 3
// names (auth.Email, domain.Phone, domain.IBAN): each implements
// slog.LogValuer and fmt.Stringer so redaction survives a forgetful call
// site even when the attribute key is not one slogpii otherwise
// recognizes as sensitive.
package pii

import "log/slog"

const redacted = "[redacted]"

// Email is a user email address. Never log a bare string containing an
// email; wrap it in this type instead.
type Email string

func (e Email) LogValue() slog.Value { return slog.StringValue(redacted) }
func (e Email) String() string       { return redacted }

// Phone is a user phone number.
type Phone string

func (p Phone) LogValue() slog.Value { return slog.StringValue(redacted) }
func (p Phone) String() string       { return redacted }

// IBAN is a bank account IBAN.
type IBAN string

func (i IBAN) LogValue() slog.Value { return slog.StringValue(redacted) }
func (i IBAN) String() string       { return redacted }
