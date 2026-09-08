package hibp_test

import (
	"context"
	"crypto/sha1" //nolint:gosec // SHA-1 is the HIBP range-API protocol itself, not used for password storage
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/platform/hibp"
)

func sha1PrefixSuffix(password string) (prefix, suffix string) {
	sum := sha1.Sum([]byte(password)) //nolint:gosec
	hexSum := strings.ToUpper(hex.EncodeToString(sum[:]))
	return hexSum[:5], hexSum[5:]
}

// auth-credentials: HIBP k-Anonymity Breach Check -- "Breached password
// rejected", and confirms the request shape: 5-char SHA-1 prefix in the
// path, Add-Padding: true header.
func TestClient_IsBreached_BreachedPasswordDetected(t *testing.T) {
	const password = "definitely-a-breached-password"
	prefix, suffix := sha1PrefixSuffix(password)

	var gotPath string
	var gotPaddingHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotPaddingHeader = r.Header.Get("Add-Padding")
		_, _ = fmt.Fprintf(w, "%s:5\r\nAAAAAAAAAAAAAAAAAAAAAAAAAAA:0\r\n", suffix)
	}))
	defer server.Close()

	client := hibp.NewClient()
	client.BaseURL = server.URL + "/range/"

	breached, err := client.IsBreached(context.Background(), password)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !breached {
		t.Fatalf("expected the password to be reported as breached")
	}
	if gotPath != "/range/"+prefix {
		t.Fatalf("expected request path /range/%s, got %s", prefix, gotPath)
	}
	if gotPaddingHeader != "true" {
		t.Fatalf("expected Add-Padding: true header, got %q", gotPaddingHeader)
	}
}

// auth-credentials: HIBP k-Anonymity Breach Check -- "Padded zero-count
// entry discarded": a response containing ONLY a padding entry (count 0)
// matching the submitted suffix must not be treated as breached.
func TestClient_IsBreached_ZeroCountPaddingEntryDiscarded(t *testing.T) {
	const password = "a-password-with-only-padding-matches"
	_, suffix := sha1PrefixSuffix(password)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%s:0\r\nDEADBEEFDEADBEEFDEADBEEFDEADBEEFDEA:3\r\n", suffix)
	}))
	defer server.Close()

	client := hibp.NewClient()
	client.BaseURL = server.URL + "/range/"

	breached, err := client.IsBreached(context.Background(), password)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if breached {
		t.Fatalf("expected a count-0 padding entry to be discarded, not treated as breached")
	}
}

// Triangulation: a response with no matching suffix line at all is also
// not breached.
func TestClient_IsBreached_NoMatchingSuffixNotBreached(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "0000000000000000000000000000000000:1\r\n")
	}))
	defer server.Close()

	client := hibp.NewClient()
	client.BaseURL = server.URL + "/range/"

	breached, err := client.IsBreached(context.Background(), "some-unrelated-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if breached {
		t.Fatalf("expected no match to mean not breached")
	}
}

// auth-credentials: HIBP k-Anonymity Breach Check -- "HIBP transport
// error fails open" (the client leg: it must surface a non-nil error,
// not silently report not-breached, so the domain layer can fail open
// explicitly rather than by accident).
func TestClient_IsBreached_TransportErrorReturnsError(t *testing.T) {
	client := hibp.NewClient()
	client.BaseURL = "http://127.0.0.1:1/unreachable/" // nothing listens here

	_, err := client.IsBreached(context.Background(), "any-password")
	if err == nil {
		t.Fatalf("expected a transport error to be returned")
	}
}

func TestClient_IsBreached_NonOKStatusReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := hibp.NewClient()
	client.BaseURL = server.URL + "/range/"

	_, err := client.IsBreached(context.Background(), "any-password")
	if err == nil {
		t.Fatalf("expected a non-200 status to be returned as an error")
	}
}
