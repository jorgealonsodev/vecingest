package mfa

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	// RecoveryCodeCount is how many one-time recovery codes enrollment
	// issues (auth-mfa-totp: One-Time Recovery Codes).
	RecoveryCodeCount = 10
	recoveryCodeBits  = 128
)

// HashRecoveryCode returns the SHA-256 hex digest of a raw recovery
// code -- the only form user_mfa.recovery_codes_hashed ever stores.
func HashRecoveryCode(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// GenerateRecoveryCodes returns 10 distinct 128-bit recovery codes
// (hex-encoded) and their SHA-256 hashes, in matching order.
func GenerateRecoveryCodes() (raw []string, hashed []string, err error) {
	raw = make([]string, RecoveryCodeCount)
	hashed = make([]string, RecoveryCodeCount)
	for i := 0; i < RecoveryCodeCount; i++ {
		buf := make([]byte, recoveryCodeBits/8)
		if _, err := rand.Read(buf); err != nil {
			return nil, nil, fmt.Errorf("mfa: generate recovery code: %w", err)
		}
		code := hex.EncodeToString(buf)
		raw[i] = code
		hashed[i] = HashRecoveryCode(code)
	}
	return raw, hashed, nil
}
