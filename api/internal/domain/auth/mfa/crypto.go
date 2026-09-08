package mfa

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// secretVersionPrefix marks the ciphertext format version so a future
// scheme change can be detected rather than silently misdecrypted (D-P:
// "AES-256-GCM ciphertext with the v1: version prefix").
const secretVersionPrefix = "v1:"

// ErrUnknownSecretVersion is returned when stored ciphertext does not
// carry a recognized version prefix.
var ErrUnknownSecretVersion = errors.New("mfa: unknown secret ciphertext version")

// EncryptSecret seals plaintext (the raw TOTP secret) with AES-256-GCM
// under key, returning "v1:" + base64(nonce‖sealed) as the exact bytes
// user_mfa.totp_secret_encrypted stores. This is the real M0 consumer
// of ENCRYPTION_KEY (D-I).
func EncryptSecret(key [32]byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("mfa: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("mfa: new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("mfa: generate nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	encoded := base64.RawStdEncoding.EncodeToString(sealed)
	return []byte(secretVersionPrefix + encoded), nil
}

// DecryptSecret reverses EncryptSecret.
func DecryptSecret(key [32]byte, stored []byte) ([]byte, error) {
	s := string(stored)
	rest, ok := strings.CutPrefix(s, secretVersionPrefix)
	if !ok {
		return nil, ErrUnknownSecretVersion
	}
	sealed, err := base64.RawStdEncoding.DecodeString(rest)
	if err != nil {
		return nil, fmt.Errorf("mfa: decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("mfa: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("mfa: new gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(sealed) < nonceSize {
		return nil, errors.New("mfa: ciphertext too short")
	}
	nonce, ciphertext := sealed[:nonceSize], sealed[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("mfa: decrypt: %w", err)
	}
	return plaintext, nil
}
