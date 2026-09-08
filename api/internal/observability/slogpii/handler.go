// Package slogpii implements the three-leg PII redaction design D-J
// requires: ReplaceAttr never sees the message string, and a LogValuer
// does not fire reliably on nested struct fields, so no single mechanism
// suffices for the request-protection spec's "slog PII Redaction"
// requirement (both scenarios: structured attribute AND interpolated
// message).
package slogpii

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
)

const redactedPlaceholder = "[redacted]"

// sensitiveKeys is the key-based redaction list (D-J leg 1 and leg 2):
// any attribute whose key matches one of these, case-insensitively, is
// replaced wholesale regardless of its value shape.
var sensitiveKeys = map[string]struct{}{
	"email":         {},
	"phone":         {},
	"iban":          {},
	"id_document":   {},
	"token":         {},
	"authorization": {},
	"password":      {},
	"refresh_token": {},
	"csrf":          {},
	"code_hash":     {},
	"totp_secret":   {},
	"secret":        {},
}

// Value-pattern scanning of the message string (D-J leg 1): email,
// Spanish phone, and ES-IBAN shapes.
var (
	emailPattern = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	phonePattern = regexp.MustCompile(`\b(?:\+34\s?)?[6789]\d{8}\b`)
	ibanPattern  = regexp.MustCompile(`\bES\d{2}[A-Z0-9]{20}\b`)
)

func redactString(s string) string {
	s = emailPattern.ReplaceAllString(s, "[redacted:email]")
	s = ibanPattern.ReplaceAllString(s, "[redacted:iban]")
	s = phonePattern.ReplaceAllString(s, "[redacted:phone]")
	return s
}

// Handler wraps another slog.Handler and walks the WHOLE record --
// including the message, which HandlerOptions.ReplaceAttr never sees --
// redacting both key-based sensitive attributes and value-pattern
// matches in the message and in any string-valued attribute (D-J leg 1).
type Handler struct {
	next slog.Handler
}

// Wrap builds a redacting Handler around next (typically a
// slog.JSONHandler configured with ReplaceAttr as leg 2).
func Wrap(next slog.Handler) *Handler {
	return &Handler{next: next}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	nr := slog.NewRecord(r.Time, r.Level, redactString(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		nr.AddAttrs(redactAttr(a))
		return true
	})
	return h.next.Handle(ctx, nr)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i] = redactAttr(a)
	}
	return &Handler{next: h.next.WithAttrs(redacted)}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{next: h.next.WithGroup(name)}
}

// redactAttr resolves a.Value first (so a LogValuer -- leg 3 -- always
// runs, even under a key this package does not otherwise recognize),
// then applies key-based redaction, then recurses into groups, then
// scans any remaining string value for a PII pattern.
func redactAttr(a slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()

	if _, sensitive := sensitiveKeys[strings.ToLower(a.Key)]; sensitive {
		return slog.String(a.Key, redactedPlaceholder)
	}

	if a.Value.Kind() == slog.KindGroup {
		group := a.Value.Group()
		redacted := make([]slog.Attr, len(group))
		for i, ga := range group {
			redacted[i] = redactAttr(ga)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(redacted...)}
	}

	if a.Value.Kind() == slog.KindString {
		return slog.String(a.Key, redactString(a.Value.String()))
	}

	return a
}

// ReplaceAttr is D-J leg 2: a cheap second key-based leg wired into
// HandlerOptions.ReplaceAttr on the underlying slog.JSONHandler, so an
// attribute injected below Handler's own walk is still caught. It
// performs the identical key-based (plus LogValuer) redaction as leg 1.
func ReplaceAttr(_ []string, a slog.Attr) slog.Attr {
	return redactAttr(a)
}
