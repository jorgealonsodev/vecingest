package mfa_test

import (
	"strings"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
)

// D-P: the otpauth:// provisioning URI carries the issuer, algorithm,
// digits and period an authenticator app needs to scan.
func TestProvisioningURI_CarriesRequiredFields(t *testing.T) {
	secret, err := mfa.GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	uri := mfa.ProvisioningURI(secret, "user@example.com")

	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Fatalf("expected an otpauth://totp/ URI, got %q", uri)
	}
	for _, want := range []string{"issuer=Vecingest", "algorithm=SHA1", "digits=6", "period=30", "secret="} {
		if !strings.Contains(uri, want) {
			t.Errorf("provisioning URI %q missing %q", uri, want)
		}
	}
}
