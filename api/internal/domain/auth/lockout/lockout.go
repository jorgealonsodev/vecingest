// Package lockout implements progressive login lockout (D-N): two
// independent counters (by hashed email, by canonicalized IP), an
// escalating block window, and a deduped, asynchronously-dispatched
// alert email -- with no channel that lets an attacker learn whether an
// email is registered.
package lockout

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// Threshold and Window are PRD §5.1's fixed lockout parameters: 5
// failures within 15 minutes.
const (
	Threshold = 5
	Window    = 15 * time.Minute

	// escalationDecay is the window over which the lock-cycle counter
	// itself decays: "decaying back to 15 after a clean hour" (D-N).
	escalationDecay = 60 * time.Minute
)

// AttemptCounter is D-N's fifth phase-A/phase-B seam: Limiter counts
// requests, this counts failures. internal/platform/attempts is the
// phase-A in-process implementation; phase B swaps it for Valkey.
type AttemptCounter interface {
	Fail(ctx context.Context, key string, window time.Duration) (int, error)
	Count(ctx context.Context, key string, window time.Duration) (int, error)
	Reset(ctx context.Context, key string) error
}

// Message is the alert email content Mailer.Send delivers.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Mailer is the domain-owned port for sending the lockout alert. The
// concrete SMTP sender and its bounded async dispatch pool live in
// internal/mail, wired in at composition time.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

// Dispatcher decouples WHEN Mailer.Send actually runs from
// RecordFailure's own call stack: sending inline would add SMTP latency
// to the response only for a real account, which is itself a timing
// oracle for account existence. Production wiring passes a hook to
// internal/mail's bounded worker pool; tests inject a synchronous
// dispatcher so assertions can observe the call deterministically
// instead of racing a goroutine.
type Dispatcher func(fn func())

func goDispatch(fn func()) { go fn() }

// FailureResult reports what RecordFailure observed. LockedByEmail and
// LockedByIP are independent: a request can be locked by either, both,
// or neither.
type FailureResult struct {
	Locked        bool
	LockedByEmail bool
	LockedByIP    bool
	// BlockDuration is the escalated window for whichever counter(s)
	// just transitioned into a lock on THIS call. It is zero when
	// nothing newly locked (e.g. an attempt while already locked).
	BlockDuration time.Duration
}

// Service is the lockout domain service. Counter and Mailer are
// required; Dispatch defaults to a real goroutine when nil.
type Service struct {
	Counter  AttemptCounter
	Mailer   Mailer
	Dispatch Dispatcher
}

func (s Service) dispatch() Dispatcher {
	if s.Dispatch != nil {
		return s.Dispatch
	}
	return goDispatch
}

// emailKey and ipKey are the two independent counter keys (D-N): the
// email is hashed so neither the in-process store nor a future shared
// Valkey ever holds a plaintext address, matching the PII discipline
// applied to logs (D-J). ip is expected to already be the resolved,
// canonicalized client IP (D-H step 2); this package does no network
// parsing of its own.
func emailKey(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	sum := sha256.Sum256([]byte(normalized))
	return "auth:fail:email:" + hex.EncodeToString(sum[:])
}

func ipKey(ip string) string {
	return "auth:fail:ip:" + ip
}

func lockCycleKey(counterKey string) string {
	return "auth:lockcycle:" + counterKey
}

func alertDedupeKey(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	sum := sha256.Sum256([]byte(normalized))
	return "auth:lockout-alert:" + hex.EncodeToString(sum[:])
}

// RecordFailure records one failed login attempt against both the
// email-scoped and IP-scoped counters, independently, and returns
// whether either just reached the lockout threshold. accountExists MUST
// be the caller's own prior existence check (RecordFailure never learns
// it any other way): the email counter still advances identically for
// an unknown address, so failure counting itself never becomes a
// user-enumeration oracle, but the alert email is sent only when the
// address is real.
func (s Service) RecordFailure(ctx context.Context, email, ip string, accountExists bool) (FailureResult, error) {
	ek, ik := emailKey(email), ipKey(ip)

	emailCount, err := s.Counter.Fail(ctx, ek, Window)
	if err != nil {
		return FailureResult{}, err
	}
	ipCount, err := s.Counter.Fail(ctx, ik, Window)
	if err != nil {
		return FailureResult{}, err
	}

	var result FailureResult
	var blockDuration time.Duration

	if emailCount == Threshold {
		result.LockedByEmail = true
		d, err := s.escalate(ctx, ek)
		if err != nil {
			return FailureResult{}, err
		}
		blockDuration = d
	} else if emailCount > Threshold {
		result.LockedByEmail = true
	}

	if ipCount == Threshold {
		result.LockedByIP = true
		d, err := s.escalate(ctx, ik)
		if err != nil {
			return FailureResult{}, err
		}
		if d > blockDuration {
			blockDuration = d
		}
	} else if ipCount > Threshold {
		result.LockedByIP = true
	}

	result.Locked = result.LockedByEmail || result.LockedByIP
	result.BlockDuration = blockDuration

	if result.Locked && accountExists {
		s.maybeAlert(ctx, email)
	}

	return result, nil
}

// escalate bumps the lock-cycle counter for counterKey and maps its new
// value to the escalating block window (D-N: "consecutive lock cycles on
// the same key extend the block 15 -> 30 -> 60 min, capped at 60,
// decaying back to 15 after a clean hour"). It is only ever called on
// the exact attempt that crosses the threshold, so repeated attempts
// while already locked never re-escalate.
func (s Service) escalate(ctx context.Context, counterKey string) (time.Duration, error) {
	n, err := s.Counter.Fail(ctx, lockCycleKey(counterKey), escalationDecay)
	if err != nil {
		return 0, err
	}
	switch {
	case n <= 1:
		return 15 * time.Minute, nil
	case n == 2:
		return 30 * time.Minute, nil
	default:
		return 60 * time.Minute, nil
	}
}

// maybeAlert sends the lockout alert at most once per lock window per
// account (deduped on alertDedupeKey), dispatched off the caller's own
// call path via s.dispatch() so an existing account never takes longer
// to answer than a non-existent one.
func (s Service) maybeAlert(ctx context.Context, email string) {
	dedupeKey := alertDedupeKey(email)
	n, err := s.Counter.Fail(ctx, dedupeKey, Window)
	if err != nil || n != 1 {
		return
	}
	if s.Mailer == nil {
		return
	}
	s.dispatch()(func() {
		_ = s.Mailer.Send(ctx, Message{
			To:      email,
			Subject: "Unusual sign-in activity on your account",
			Body:    "We blocked several failed sign-in attempts on your account.",
		})
	})
}

// RecordSuccess resets ONLY the email-scoped counter (D-N): resetting
// the IP counter too would let an attacker holding one valid account
// clear their own IP budget between bursts.
func (s Service) RecordSuccess(ctx context.Context, email string) error {
	return s.Counter.Reset(ctx, emailKey(email))
}

// IsLocked reports whether either the email-scoped or IP-scoped counter
// is currently at or above Threshold.
func (s Service) IsLocked(ctx context.Context, email, ip string) (bool, error) {
	emailCount, err := s.Counter.Count(ctx, emailKey(email), Window)
	if err != nil {
		return false, err
	}
	if emailCount >= Threshold {
		return true, nil
	}
	ipCount, err := s.Counter.Count(ctx, ipKey(ip), Window)
	if err != nil {
		return false, err
	}
	return ipCount >= Threshold, nil
}
