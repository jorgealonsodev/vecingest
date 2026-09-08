// Package session implements refresh-token rotation with family
// invalidation (D-D) and the transport-detection guard for
// /v1/auth/refresh (D-E's "native-client exemption without a hole").
package session

import "errors"

// TransportKind distinguishes the two mutually exclusive ways a client
// may present a refresh token to /v1/auth/refresh.
type TransportKind int

const (
	// TransportCookie is the web path: the refresh token travels in the
	// HttpOnly cookie, and a CSRF header is required.
	TransportCookie TransportKind = iota
	// TransportBody is the native path: the refresh token travels in the
	// JSON request body, and no CSRF token is required because there is
	// no ambient cookie authority for a cross-site attacker to abuse.
	TransportBody
)

// Transport is the resolved transport plus the raw refresh token value
// it carried.
type Transport struct {
	Kind  TransportKind
	Token string
}

var (
	// ErrAmbiguousTokenTransport is returned when a request carries a
	// refresh token in both the cookie and the body -- the downgrade
	// hole D-E closes: a browser attacker cannot force the body path,
	// so accepting both would let it choose the weaker one.
	ErrAmbiguousTokenTransport = errors.New("session: refresh token present in both cookie and body")
	// ErrAmbiguousTransport is an alias kept for call sites that only
	// need to compare against the sentinel, not read its message.
	ErrAmbiguousTransport = ErrAmbiguousTokenTransport
	// ErrMissingRefreshToken is returned when neither transport carries
	// a refresh token at all.
	ErrMissingRefreshToken = errors.New("session: no refresh token presented")
)

// ResolveTransport applies D-E's mutual-exclusion rule: cookie XOR body,
// never both, never neither (auth-session-tokens: Refresh Transport
// Mutual Exclusion). cookieToken and bodyToken are empty strings when
// that transport carried no refresh token.
func ResolveTransport(cookieToken, bodyToken string) (Transport, error) {
	hasCookie := cookieToken != ""
	hasBody := bodyToken != ""

	switch {
	case hasCookie && hasBody:
		return Transport{}, ErrAmbiguousTokenTransport
	case hasCookie:
		return Transport{Kind: TransportCookie, Token: cookieToken}, nil
	case hasBody:
		return Transport{Kind: TransportBody, Token: bodyToken}, nil
	default:
		return Transport{}, ErrMissingRefreshToken
	}
}
