package mfa

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jorgealonsodev/vecingest/internal/db"
)

// Email-confirmed enrollment (auth-mfa-totp: Email-Confirmed
// Enrollment). TOTP is optional for owners and tenants, so most of those
// accounts have no factor, and there is no disable or recovery path yet:
// whoever activates the first factor owns the account's second factor
// for good. A password alone used to be enough to do that with the
// attacker's own authenticator. Enrollment therefore also mails a
// one-time code to the address stored on the account, and activation
// requires both codes -- proof of the password, the new authenticator
// AND the mailbox.
const (
	// EnrollEmailPurpose is the otp_challenges.purpose of that code.
	EnrollEmailPurpose = "mfa_enroll"
	// EnrollEmailTTL is how long one issued code stays usable.
	EnrollEmailTTL = 10 * time.Minute
	// EnrollEmailMaxAttempts caps confirmations against one code: six
	// digits are 10^6 candidates, and five guesses per mailed code keep
	// the odds of a blind guess at 5 in a million per enrollment.
	EnrollEmailMaxAttempts = 5
	// EnrollEmailIssueLimit caps how many codes one user can be issued
	// within EnrollEmailIssueWindow. Every re-enrollment mints a fresh
	// code with its own EnrollEmailMaxAttempts, so without this cap the
	// per-code attempt limit bounded nothing: re-enrolling under the
	// per-user request limit allowed hundreds of guesses a minute. With
	// it, a caller holding only the password gets at most
	// EnrollEmailIssueLimit * EnrollEmailMaxAttempts guesses per window.
	EnrollEmailIssueLimit = 5
	// EnrollEmailIssueWindow is the sliding window EnrollEmailIssueLimit
	// counts issued codes over.
	EnrollEmailIssueWindow = time.Hour
)

// enrollEmailHashDomain separates these digests from every other HMAC
// computed under the same ENCRYPTION_KEY (the invitation short code
// uses it too), so a digest from one table can never verify in another.
const enrollEmailHashDomain = "vecingest/mfa_enroll/v1\x00"

// EnrollEmail is the enrollment-code email job's in-memory payload. Code
// is the plaintext: the queue adapter seals it before it reaches a
// durable row, exactly like invitations.EmailArgs.ShortCode.
type EnrollEmail struct {
	UserID uuid.UUID
	// ChallengeID is the otp_challenges row Code belongs to, so delivery
	// can skip a code that was superseded, expired or spent before the
	// email went out.
	ChallengeID uuid.UUID
	// Email is always users.email as stored, read inside the issuing
	// transaction -- never an address the client supplied.
	Email string
	Code  string
}

// EnrollEmailQueue enqueues the enrollment-code email on tx, so the
// email exists if and only if the challenge and the pending secret it
// confirms were committed.
type EnrollEmailQueue interface {
	EnqueueMFAEnrollEmail(ctx context.Context, tx pgx.Tx, args EnrollEmail) error
}

// GenerateEnrollEmailCode returns a uniformly random 6-digit code,
// leading zeros kept. crypto/rand.Int rejects-and-retries internally, so
// there is no modulo bias.
func GenerateEnrollEmailCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("mfa: generate enrollment code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// HashEnrollEmailCode is the keyed digest stored in
// otp_challenges.code_hash. Keyed for the same reason as the invitation
// short code: 10^6 candidates fall to an unkeyed hash instantly, so a
// table dump must not be enough to recover a live code.
func HashEnrollEmailCode(key [32]byte, code string) []byte {
	mac := hmac.New(sha256.New, key[:])
	mac.Write([]byte(enrollEmailHashDomain))
	mac.Write([]byte(code))
	return mac.Sum(nil)
}

// EnrollEmailCodeMatches compares code against a stored digest in
// constant time.
func EnrollEmailCodeMatches(key [32]byte, code string, stored []byte) bool {
	return hmac.Equal(HashEnrollEmailCode(key, code), stored)
}

// ErrEnrollEmailIssueLimited is IssueEnrollEmailChallenge's refusal once
// userID was issued EnrollEmailIssueLimit codes within
// EnrollEmailIssueWindow.
var ErrEnrollEmailIssueLimited = errors.New("mfa: enrollment code issuance limit reached")

// IssueEnrollEmailChallenge closes every open enrollment challenge for
// userID and inserts a fresh one, returning its ID and plaintext code
// for the caller to enqueue on the same transaction. Closing the old ones is
// what makes a re-enrollment supersede the previous email: its code was
// issued for a secret that no longer exists.
//
// It refuses with ErrEnrollEmailIssueLimited, issuing nothing, once the
// cap is reached. The count is only race-free when the caller already
// holds the user's row lock (LockUserForMFAEnrollment) on tx.
func IssueEnrollEmailChallenge(ctx context.Context, tx pgx.Tx, clock Clock, key [32]byte, userID uuid.UUID) (challengeID uuid.UUID, code string, err error) {
	q := db.New(tx)
	issued, err := q.CountOTPChallengesIssuedSince(ctx, db.CountOTPChallengesIssuedSinceParams{
		// Whole seconds, not Duration.Seconds(): the query multiplies an
		// integer into an interval, and EnrollEmailIssueWindow is an exact
		// number of seconds, so this truncating division loses nothing.
		UserID: userID, Purpose: EnrollEmailPurpose, WindowSeconds: int32(EnrollEmailIssueWindow / time.Second),
	})
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("mfa: count issued enrollment challenges: %w", err)
	}
	if issued >= EnrollEmailIssueLimit {
		return uuid.Nil, "", ErrEnrollEmailIssueLimited
	}

	now := clock.Now()
	if err := q.InvalidateOpenOTPChallenges(ctx, db.InvalidateOpenOTPChallengesParams{
		Now: now, UserID: userID, Purpose: EnrollEmailPurpose,
	}); err != nil {
		return uuid.Nil, "", fmt.Errorf("mfa: invalidate open enrollment challenges: %w", err)
	}

	code, err = GenerateEnrollEmailCode()
	if err != nil {
		return uuid.Nil, "", err
	}
	challengeID = uuid.New()
	if _, err := q.InsertOTPChallenge(ctx, db.InsertOTPChallengeParams{
		ID:        challengeID,
		UserID:    userID,
		Purpose:   EnrollEmailPurpose,
		CodeHash:  HashEnrollEmailCode(key, code),
		Channel:   "email",
		ExpiresAt: now.Add(EnrollEmailTTL),
	}); err != nil {
		return uuid.Nil, "", fmt.Errorf("mfa: insert enrollment challenge: %w", err)
	}
	return challengeID, code, nil
}
