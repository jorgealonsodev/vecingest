package session_test

import (
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/session"
)

// auth-session-tokens: Refresh Transport Mutual Exclusion.
func TestResolveTransport(t *testing.T) {
	tests := []struct {
		name      string
		cookie    string
		body      string
		want      session.TransportKind
		wantErr   bool
		wantErrIs error
	}{
		{name: "cookie only is the web path", cookie: "cookie-token", body: "", want: session.TransportCookie},
		{name: "body only is the native path", cookie: "", body: "body-token", want: session.TransportBody},
		{name: "both present is ambiguous", cookie: "cookie-token", body: "body-token", wantErr: true, wantErrIs: session.ErrAmbiguousTransport},
		{name: "neither present is missing", cookie: "", body: "", wantErr: true, wantErrIs: session.ErrMissingRefreshToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := session.ResolveTransport(tt.cookie, tt.body)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveTransport(%q, %q): expected an error, got nil", tt.cookie, tt.body)
				}
				if tt.wantErrIs != nil && err != tt.wantErrIs {
					t.Fatalf("ResolveTransport(%q, %q): error = %v, want %v", tt.cookie, tt.body, err, tt.wantErrIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveTransport(%q, %q): unexpected error: %v", tt.cookie, tt.body, err)
			}
			if got.Kind != tt.want {
				t.Errorf("ResolveTransport(%q, %q).Kind = %v, want %v", tt.cookie, tt.body, got.Kind, tt.want)
			}
		})
	}
}
