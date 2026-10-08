package invitations

import (
	"crypto/rand"
	"fmt"
)

// shortCodeAlphabet excludes O/0/I/1 (design D-6, invitations spec:
// "an 8-character short code from an unambiguous alphabet excluding O,
// 0, I, 1"), so a code read aloud or handwritten on paper (the
// sequence diagram's "paper/voice" delivery path) is never ambiguous
// between similar-looking characters.
const shortCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// ShortCodeLength is the invitation short code's fixed length (design
// D-6).
const ShortCodeLength = 8

// GenerateShortCode returns a fresh, cryptographically random short
// code (crypto/rand, design D-6). Storage MUST hash it (SHA-256) and
// never persist the plaintext this function returns beyond the single
// creation response (invitations spec: "Created invitation exposes the
// short code exactly once").
func GenerateShortCode() (string, error) {
	buf := make([]byte, ShortCodeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("invitations: generate short code: %w", err)
	}
	out := make([]byte, ShortCodeLength)
	for i, b := range buf {
		out[i] = shortCodeAlphabet[int(b)%len(shortCodeAlphabet)]
	}
	return string(out), nil
}
