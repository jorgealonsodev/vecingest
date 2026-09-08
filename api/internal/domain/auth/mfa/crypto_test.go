package mfa_test

import (
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
)

func testKey() [32]byte {
	var k [32]byte
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

// D-P: the TOTP secret is stored as AES-256-GCM ciphertext with a "v1:"
// version prefix -- this is the real M0 consumer of ENCRYPTION_KEY.
func TestEncryptDecryptSecret_RoundTrips(t *testing.T) {
	key := testKey()
	secret, err := mfa.GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}

	ciphertext, err := mfa.EncryptSecret(key, secret)
	if err != nil {
		t.Fatalf("EncryptSecret: %v", err)
	}
	if len(ciphertext) < 3 || string(ciphertext[:3]) != "v1:" {
		t.Fatalf("expected a v1: version prefix, got %q", ciphertext)
	}

	plaintext, err := mfa.DecryptSecret(key, ciphertext)
	if err != nil {
		t.Fatalf("DecryptSecret: %v", err)
	}
	if string(plaintext) != string(secret) {
		t.Fatalf("round-trip mismatch: got %x, want %x", plaintext, secret)
	}
}

func TestDecryptSecret_RejectsTamperedCiphertext(t *testing.T) {
	key := testKey()
	secret, _ := mfa.GenerateSecret()
	ciphertext, err := mfa.EncryptSecret(key, secret)
	if err != nil {
		t.Fatalf("EncryptSecret: %v", err)
	}
	tampered := []byte(string(ciphertext) + "x")
	if _, err := mfa.DecryptSecret(key, tampered); err == nil {
		t.Fatalf("expected an error for tampered ciphertext, got nil")
	}
}

func TestDecryptSecret_RejectsWrongKey(t *testing.T) {
	key := testKey()
	var otherKey [32]byte
	for i := range otherKey {
		otherKey[i] = byte(255 - i)
	}
	secret, _ := mfa.GenerateSecret()
	ciphertext, err := mfa.EncryptSecret(key, secret)
	if err != nil {
		t.Fatalf("EncryptSecret: %v", err)
	}
	if _, err := mfa.DecryptSecret(otherKey, ciphertext); err == nil {
		t.Fatalf("expected an error when decrypting with the wrong key, got nil")
	}
}
