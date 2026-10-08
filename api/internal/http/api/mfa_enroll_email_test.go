package api_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
)

// enrollConfirmationInvalid is the one code every rejected enrollment
// confirmation returns, whichever factor was wrong.
const enrollConfirmationInvalid = "AUTH_MFA_ENROLLMENT_CONFIRMATION_INVALID"

// mfaEnrollEmailJob is the latest mfa_enroll_email river_job row queued
// for userID: the address it is going to and the code it carries,
// opened with the queue's ENCRYPTION_KEY.
//
// Reading the sealed job row is how these tests learn the code, rather
// than a code-generator seam on Deps: the row is what production
// delivers from, so the same read also proves the recipient and that
// the payload is sealed, and no test-only hook is added to the handler.
type mfaEnrollEmailJob struct {
	Email       string
	ChallengeID string
	Code        string
	Args        string
}

func latestMFAEnrollEmailJob(t *testing.T, handlesDB db.Handles, userID uuid.UUID) mfaEnrollEmailJob {
	t.Helper()
	var args []byte
	if err := handlesDB.Write.QueryRow(t.Context(),
		`SELECT args FROM river_job WHERE kind = 'mfa_enroll_email' AND args->>'user_id' = $1 ORDER BY id DESC LIMIT 1`,
		userID.String(),
	).Scan(&args); err != nil {
		t.Fatalf("read the mfa_enroll_email job row for %s: %v", userID, err)
	}
	var payload struct {
		Email         string `json:"email"`
		ChallengeID   string `json:"challenge_id"`
		CodeEncrypted []byte `json:"code_encrypted"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		t.Fatalf("decode the mfa_enroll_email job payload: %v", err)
	}
	code, err := mfa.DecryptSecret(testEncryptionKey, payload.CodeEncrypted)
	if err != nil {
		t.Fatalf("expected the enrollment code sealed under ENCRYPTION_KEY: %v", err)
	}
	return mfaEnrollEmailJob{Email: payload.Email, ChallengeID: payload.ChallengeID, Code: string(code), Args: string(args)}
}

// enrollEmailCode is the code the latest enrollment emailed to userID.
func enrollEmailCode(t *testing.T, handlesDB db.Handles, userID uuid.UUID) string {
	t.Helper()
	return latestMFAEnrollEmailJob(t, handlesDB, userID).Code
}

// wrongEmailCode returns a well-formed code guaranteed to differ from code.
func wrongEmailCode(code string) string {
	if code == "000000" {
		return "111111"
	}
	return "000000"
}

type enrollFixture struct {
	srv    *httptest.Server
	client *http.Client
	db     db.Handles
	userID uuid.UUID
	auth   map[string]string
}

func newEnrollFixture(t *testing.T, email string) enrollFixture {
	t.Helper()
	srv, deps, handlesDB := newTestServer(t)
	userID := createUser(t, handlesDB, email, false)
	return enrollFixture{
		srv: srv, client: newClient(srv, nil), db: handlesDB, userID: userID,
		auth: bearer(mintAccessToken(t, deps, handlesDB, userID, false)),
	}
}

func (f enrollFixture) enroll(t *testing.T) string {
	t.Helper()
	resp, body := doJSON(t, f.client, http.MethodPost, f.srv.URL+"/v1/me/mfa/enroll", nil, f.auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on enroll, got %d body=%v", resp.StatusCode, body)
	}
	secret, _ := body["secret"].(string)
	if secret == "" {
		t.Fatalf("expected a base32 secret from enroll, got %v", body)
	}
	return secret
}

func (f enrollFixture) verify(t *testing.T, payload map[string]any) (*http.Response, map[string]any) {
	t.Helper()
	return doJSON(t, f.client, http.MethodPost, f.srv.URL+"/v1/me/mfa/verify", payload, f.auth)
}

func (f enrollFixture) requireInactive(t *testing.T) {
	t.Helper()
	row, err := db.New(f.db.Write).GetUserMFA(t.Context(), f.userID)
	if err != nil {
		t.Fatalf("get user_mfa: %v", err)
	}
	if row.EnabledAt.Valid {
		t.Fatalf("expected TOTP to stay INACTIVE, got enabled_at=%v", row.EnabledAt.Time)
	}
}

func requireConfirmationRejected(t *testing.T, resp *http.Response, body map[string]any) {
	t.Helper()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%v", resp.StatusCode, body)
	}
	if body["code"] != enrollConfirmationInvalid {
		t.Fatalf("expected %s, got %v", enrollConfirmationInvalid, body)
	}
}

// auth-mfa-totp: Email-Confirmed Enrollment. A password and the
// caller's own authenticator are not enough: without the code mailed to
// the stored address the factor never activates -- and the rejection is
// the same whichever half was wrong.
func TestMFAEnrollEmail_ValidTOTPWithoutEmailCodeIsRejected(t *testing.T) {
	f := newEnrollFixture(t, "mfa-email-missing@example.com")
	secret := f.enroll(t)
	emailCode := enrollEmailCode(t, f.db, f.userID)

	resp, missing := f.verify(t, map[string]any{"code": validTOTPCode(t, secret)})
	requireConfirmationRejected(t, resp, missing)
	f.requireInactive(t)

	resp, wrongEmail := f.verify(t, map[string]any{"code": validTOTPCode(t, secret), "email_code": wrongEmailCode(emailCode)})
	requireConfirmationRejected(t, resp, wrongEmail)
	f.requireInactive(t)

	resp, wrongTOTP := f.verify(t, map[string]any{"code": "000000", "email_code": emailCode})
	requireConfirmationRejected(t, resp, wrongTOTP)
	f.requireInactive(t)

	if missing["message"] != wrongEmail["message"] || wrongEmail["message"] != wrongTOTP["message"] {
		t.Fatalf("expected one generic message that does not reveal which factor failed, got %q / %q / %q",
			missing["message"], wrongEmail["message"], wrongTOTP["message"])
	}
}

func TestMFAEnrollEmail_ExpiredCodeIsRejected(t *testing.T) {
	f := newEnrollFixture(t, "mfa-email-expired@example.com")
	secret := f.enroll(t)
	emailCode := enrollEmailCode(t, f.db, f.userID)

	if _, err := f.db.Write.Exec(t.Context(),
		`UPDATE otp_challenges SET expires_at = now() - interval '1 second' WHERE user_id = $1 AND purpose = 'mfa_enroll'`,
		f.userID,
	); err != nil {
		t.Fatalf("expire the challenge: %v", err)
	}

	resp, body := f.verify(t, map[string]any{"code": validTOTPCode(t, secret), "email_code": emailCode})
	requireConfirmationRejected(t, resp, body)
	f.requireInactive(t)
}

// mfa.EnrollEmailMaxAttempts wrong guesses exhaust the challenge: the
// next is refused even carrying the right code, because each
// rejection's increment was kept.
func TestMFAEnrollEmail_SixthAttemptRejectedEvenWithCorrectCode(t *testing.T) {
	f := newEnrollFixture(t, "mfa-email-attempts@example.com")
	secret := f.enroll(t)
	emailCode := enrollEmailCode(t, f.db, f.userID)

	for i := range mfa.EnrollEmailMaxAttempts {
		resp, body := f.verify(t, map[string]any{"code": validTOTPCode(t, secret), "email_code": wrongEmailCode(emailCode)})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d body=%v", i+1, resp.StatusCode, body)
		}
	}

	var attempts int32
	if err := f.db.Write.QueryRow(t.Context(),
		`SELECT attempts FROM otp_challenges WHERE user_id = $1 AND purpose = 'mfa_enroll'`, f.userID,
	).Scan(&attempts); err != nil {
		t.Fatalf("read attempts: %v", err)
	}
	if attempts != mfa.EnrollEmailMaxAttempts {
		t.Fatalf("expected %d recorded attempts surviving the rejections, got %d", mfa.EnrollEmailMaxAttempts, attempts)
	}

	resp, body := f.verify(t, map[string]any{"code": validTOTPCode(t, secret), "email_code": emailCode})
	requireConfirmationRejected(t, resp, body)
	f.requireInactive(t)
}

// Re-enrolling supersedes the previous code: it cannot confirm the new
// secret, and the new code can.
func TestMFAEnrollEmail_ReEnrollInvalidatesPriorCode(t *testing.T) {
	f := newEnrollFixture(t, "mfa-email-reenroll@example.com")
	f.enroll(t)
	firstCode := enrollEmailCode(t, f.db, f.userID)

	secret := f.enroll(t)
	secondCode := enrollEmailCode(t, f.db, f.userID)
	for secondCode == firstCode {
		// One in a million: re-enroll until the codes differ so the
		// assertion below is about the challenge, not the digits.
		secret = f.enroll(t)
		secondCode = enrollEmailCode(t, f.db, f.userID)
	}

	resp, body := f.verify(t, map[string]any{"code": validTOTPCode(t, secret), "email_code": firstCode})
	requireConfirmationRejected(t, resp, body)
	f.requireInactive(t)

	var open int
	if err := f.db.Write.QueryRow(t.Context(),
		`SELECT count(*) FROM otp_challenges WHERE user_id = $1 AND purpose = 'mfa_enroll' AND verified_at IS NULL AND expires_at > now()`,
		f.userID,
	).Scan(&open); err != nil {
		t.Fatalf("count open challenges: %v", err)
	}
	if open != 1 {
		t.Fatalf("expected exactly one open mfa_enroll challenge after re-enrolling, got %d", open)
	}

	resp, body = f.verify(t, map[string]any{"code": validTOTPCode(t, secret), "email_code": secondCode})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected the current code to confirm, got %d body=%v", resp.StatusCode, body)
	}
}

// Both factors right activates the factor and spends the challenge: it
// is marked verified and no later attempt can consume it.
func TestMFAEnrollEmail_ValidBothActivatesAndSpendsTheCode(t *testing.T) {
	f := newEnrollFixture(t, "mfa-email-valid@example.com")
	secret := f.enroll(t)
	emailCode := enrollEmailCode(t, f.db, f.userID)

	resp, body := f.verify(t, map[string]any{"code": validTOTPCode(t, secret), "email_code": emailCode})
	if resp.StatusCode != http.StatusOK || body["active"] != true {
		t.Fatalf("expected 200 active=true, got %d body=%v", resp.StatusCode, body)
	}

	row, err := db.New(f.db.Write).GetUserMFA(t.Context(), f.userID)
	if err != nil || !row.EnabledAt.Valid {
		t.Fatalf("expected an active factor, got enabled=%v err=%v", row.EnabledAt.Valid, err)
	}

	var verified bool
	if err := f.db.Write.QueryRow(t.Context(),
		`SELECT verified_at IS NOT NULL FROM otp_challenges WHERE user_id = $1 AND purpose = 'mfa_enroll'`, f.userID,
	).Scan(&verified); err != nil {
		t.Fatalf("read challenge: %v", err)
	}
	if !verified {
		t.Fatalf("expected the challenge to be marked verified")
	}

	// The confirm branch is closed once the factor is active, so the
	// second use is asserted where it would be spent: the challenge no
	// longer yields an attempt.
	_, err = db.New(f.db.Write).ConsumeOTPChallengeAttempt(t.Context(), db.ConsumeOTPChallengeAttemptParams{
		UserID: f.userID, Purpose: "mfa_enroll", Now: time.Now(), MaxAttempts: mfa.EnrollEmailMaxAttempts,
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected a verified challenge to be unusable, got err=%v", err)
	}
}

// The code goes to the address stored on the account, sealed in the
// durable job row, never in plaintext.
func TestMFAEnrollEmail_JobTargetsStoredAddressWithSealedCode(t *testing.T) {
	const stored = "mfa-email-recipient@example.com"
	f := newEnrollFixture(t, stored)
	f.enroll(t)

	job := latestMFAEnrollEmailJob(t, f.db, f.userID)
	if job.Email != stored {
		t.Fatalf("expected the enrollment email to go to the stored address %q, got %q", stored, job.Email)
	}
	if len(job.Code) != 6 || strings.Trim(job.Code, "0123456789") != "" {
		t.Fatalf("expected a 6-digit code, got %q", job.Code)
	}
	if strings.Contains(job.Args, job.Code) {
		t.Fatalf("the river_job row holds the plaintext enrollment code: %s", job.Args)
	}

	var hash []byte
	var challengeID uuid.UUID
	if err := f.db.Write.QueryRow(t.Context(),
		`SELECT id, code_hash FROM otp_challenges WHERE user_id = $1 AND purpose = 'mfa_enroll' AND channel = 'email'`, f.userID,
	).Scan(&challengeID, &hash); err != nil {
		t.Fatalf("read challenge: %v", err)
	}
	// The job names the challenge it delivers, so a retried delivery can
	// tell whether that code is still open.
	if job.ChallengeID != challengeID.String() {
		t.Fatalf("expected the job to name challenge %s, got %q", challengeID, job.ChallengeID)
	}
	if len(hash) != 32 || string(hash) == job.Code {
		t.Fatalf("expected a 32-byte keyed digest, got %d bytes", len(hash))
	}
}

// countMFAEnrollChallenges is how many mfa_enroll challenges userID has
// ever been issued.
func countMFAEnrollChallenges(t *testing.T, handlesDB db.Handles, userID uuid.UUID) int {
	t.Helper()
	var n int
	if err := handlesDB.Write.QueryRow(t.Context(),
		`SELECT count(*) FROM otp_challenges WHERE user_id = $1 AND purpose = 'mfa_enroll'`, userID,
	).Scan(&n); err != nil {
		t.Fatalf("count mfa_enroll challenges: %v", err)
	}
	return n
}

// Re-enrolling cannot mint fresh attempt budgets without limit: each
// enrollment issues a new code with its own mfa.EnrollEmailMaxAttempts,
// so only the number of codes per window bounds blind guessing. Past
// mfa.EnrollEmailIssueLimit the enrollment is refused with 429 and
// Retry-After, and nothing is issued or mailed.
func TestMFAEnrollEmail_IssuanceIsCappedPerWindow(t *testing.T) {
	f := newEnrollFixture(t, "mfa-email-cap@example.com")
	for range mfa.EnrollEmailIssueLimit {
		f.enroll(t)
	}
	lastCode := enrollEmailCode(t, f.db, f.userID)

	resp, body := doJSON(t, f.client, http.MethodPost, f.srv.URL+"/v1/me/mfa/enroll", nil, f.auth)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 past the issuance cap, got %d body=%v", resp.StatusCode, body)
	}
	if body["code"] != "AUTH_TOO_MANY_ATTEMPTS" {
		t.Fatalf("expected AUTH_TOO_MANY_ATTEMPTS, got %v", body)
	}
	if got, want := resp.Header.Get("Retry-After"), strconv.Itoa(int(mfa.EnrollEmailIssueWindow.Seconds())); got != want {
		t.Fatalf("expected Retry-After %q, got %q", want, got)
	}
	if n := countMFAEnrollChallenges(t, f.db, f.userID); n != mfa.EnrollEmailIssueLimit {
		t.Fatalf("expected no challenge issued past the cap, got %d", n)
	}
	if got := enrollEmailCode(t, f.db, f.userID); got != lastCode {
		t.Fatalf("expected no new enrollment email past the cap")
	}
}

// The cap is a sliding window: codes issued before it no longer count.
func TestMFAEnrollEmail_IssuanceCapForgetsCodesOutsideTheWindow(t *testing.T) {
	f := newEnrollFixture(t, "mfa-email-cap-window@example.com")
	for range mfa.EnrollEmailIssueLimit {
		f.enroll(t)
	}
	if _, err := f.db.Write.Exec(t.Context(),
		`UPDATE otp_challenges SET created_at = now() - make_interval(secs => $2) - interval '1 second'
		 WHERE user_id = $1 AND purpose = 'mfa_enroll'`,
		f.userID, mfa.EnrollEmailIssueWindow.Seconds(),
	); err != nil {
		t.Fatalf("age the challenges out of the window: %v", err)
	}

	f.enroll(t)
}
