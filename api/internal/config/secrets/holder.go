// Package secrets holds every credential-bearing configuration value
// behind unexported fields, reachable only through accessor methods, so
// logging or JSON-encoding a Config value cannot serialize a secret even
// by accident (design D-I; platform-bootstrap: ENCRYPTION_KEY Isolation).
package secrets

import (
	"encoding/json"
	"log/slog"
)

const redacted = "[redacted]"

// Values is the plain-value constructor input for a Holder. It exists so
// NewHolder can build a Holder in one call without exposing setters that
// could mutate a Holder after construction.
type Values struct {
	JWTSecret             string
	JWTSecretPrevious     string
	JWTRefreshSecret      string
	EncryptionKey         []byte
	DatabaseURL           string
	DatabaseURLRead       string
	DatabaseURLWorker     string
	BootstrapDatabaseURL  string
	MigrationsDatabaseURL string
	AppDBPassword         string
	SMTPURL               string
	SentryDSN             string
	TurnstileSecret       string
}

// Holder is opaque by design: its fields are unexported and every
// logging/serialization hook below is redacted, so a config value that
// embeds a *Holder cannot leak a secret through slog, fmt, or JSON.
type Holder struct {
	v Values
}

// NewHolder builds a Holder from plain values. Callers outside this
// package cannot construct or mutate a Holder any other way.
func NewHolder(v Values) *Holder {
	return &Holder{v: v}
}

func (h *Holder) JWTSecret() string             { return h.v.JWTSecret }
func (h *Holder) JWTSecretPrevious() string     { return h.v.JWTSecretPrevious }
func (h *Holder) JWTRefreshSecret() string      { return h.v.JWTRefreshSecret }
func (h *Holder) EncryptionKey() []byte         { return h.v.EncryptionKey }
func (h *Holder) DatabaseURL() string           { return h.v.DatabaseURL }
func (h *Holder) DatabaseURLRead() string       { return h.v.DatabaseURLRead }
func (h *Holder) DatabaseURLWorker() string     { return h.v.DatabaseURLWorker }
func (h *Holder) BootstrapDatabaseURL() string  { return h.v.BootstrapDatabaseURL }
func (h *Holder) MigrationsDatabaseURL() string { return h.v.MigrationsDatabaseURL }
func (h *Holder) AppDBPassword() string         { return h.v.AppDBPassword }
func (h *Holder) SMTPURL() string               { return h.v.SMTPURL }
func (h *Holder) SentryDSN() string             { return h.v.SentryDSN }
func (h *Holder) TurnstileSecret() string       { return h.v.TurnstileSecret }

// LogValue implements slog.LogValuer: the whole holder redacts to a
// single opaque string in any structured log record.
func (h *Holder) LogValue() slog.Value {
	return slog.StringValue(redacted)
}

// String implements fmt.Stringer: %v/%s formatting never reaches the
// underlying secret values.
func (h *Holder) String() string {
	return "secrets.Holder{redacted}"
}

// MarshalJSON redacts the whole holder, including when it is embedded as
// a field inside a larger struct that gets logged or reported as JSON.
func (h *Holder) MarshalJSON() ([]byte, error) {
	return json.Marshal(redacted)
}
