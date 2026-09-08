// Package mfa implements TOTP enrollment, verification with replay
// protection, recovery codes and their shared brute-force throttle
// (D-P). github.com/pquerna/otp provides RFC 6238 generation; this
// package owns only the part no library can provide, because it needs
// storage: the monotonic step counter, and the isolation-level-aware
// classification of a concurrent verification's loser.
package mfa

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"fmt"
	"net/url"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// Period, Digits and Skew are RFC 6238's parameters as pinned by D-P
// (auth-mfa-totp: TOTP Verification Parameters): 30-second period, 6
// digits, ±1 step of drift tolerance.
const (
	Period     = 30 * time.Second
	Digits     = otp.DigitsSix
	Algorithm  = otp.AlgorithmSHA1
	secretBits = 160
)

// Clock abstracts time.Now() so verification tests never depend on the
// wall clock.
type Clock interface{ Now() time.Time }

// GenerateSecret returns a fresh 160-bit TOTP secret (D-P).
func GenerateSecret() ([]byte, error) {
	buf := make([]byte, secretBits/8)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("mfa: generate secret: %w", err)
	}
	return buf, nil
}

// base32Secret encodes secret as RFC 4648 base32 without padding, the
// form both the provisioning URI and pquerna/otp's functions expect.
func base32Secret(secret []byte) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
}

// ProvisioningURI builds the otpauth:// URI an authenticator app scans
// (D-P).
func ProvisioningURI(secret []byte, email string) string {
	v := url.Values{}
	v.Set("secret", base32Secret(secret))
	v.Set("issuer", "Vecingest")
	v.Set("algorithm", "SHA1")
	v.Set("digits", "6")
	v.Set("period", "30")
	u := url.URL{
		Scheme:   "otpauth",
		Host:     "totp",
		Path:     "/Vecingest:" + url.PathEscape(email),
		RawQuery: v.Encode(),
	}
	return u.String()
}

func stepOf(t time.Time) int64 {
	return t.Unix() / int64(Period.Seconds())
}

// MatchStep computes candidate codes for steps T-1, T and T+1 (T derived
// from now) and compares each against code with constant-time
// comparison. ALL THREE candidates are ALWAYS compared -- this loop must
// never break early, both because stopping at the first match would
// make the number of comparisons itself a timing signal, and because
// the HIGHEST matching step (not the first) is what callers must record
// to avoid leaving a higher step replayable
// (auth-mfa-totp: TOTP Verification Parameters, TOTP Replay Protection).
// ok=false means no candidate matched; matchedStep is meaningless in
// that case.
func MatchStep(secret []byte, code string, now time.Time) (matchedStep int64, ok bool) {
	current := stepOf(now)
	secretB32 := base32Secret(secret)
	matchedStep = -1

	for _, step := range [3]int64{current - 1, current, current + 1} {
		want, err := totp.GenerateCodeCustom(secretB32, time.Unix(step*int64(Period.Seconds()), 0), totp.ValidateOpts{
			Period:    uint(Period.Seconds()),
			Digits:    Digits,
			Algorithm: Algorithm,
		})
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 && step > matchedStep {
			matchedStep = step
			ok = true
		}
	}
	return matchedStep, ok
}
