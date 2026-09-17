package api_test

import (
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"

	"github.com/jorgealonsodev/vecingest/internal/db"
	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
)

// validTOTPCode computes a real, currently-valid TOTP code for
// base32Secret (RFC 6238, matching internal/domain/auth/mfa's own
// Period/Digits/Algorithm), so these tests exercise the SAME
// verification path production traffic does -- never a stubbed
// always-true check.
func validTOTPCode(t *testing.T, base32Secret string) string {
	t.Helper()
	code, err := totp.GenerateCode(base32Secret, time.Now())
	if err != nil {
		t.Fatalf("generate totp code: %v", err)
	}
	return code
}

// auth-mfa-totp delta: Non-Superadmin TOTP HTTP Endpoints, scenario 1 --
// "an admin with no TOTP enrolled verifies a valid code via the
// non-superadmin endpoint and becomes active".
func TestMFA_EnrollThenVerifyActivates(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	userID := createUser(t, handlesDB, "mfa-enroll@example.com", false)
	accessToken := mintAccessToken(t, deps, handlesDB, userID, false)
	auth := map[string]string{"Authorization": "Bearer " + accessToken}

	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/me/mfa/enroll", nil, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on enroll, got %d body=%v", resp.StatusCode, body)
	}
	secret, _ := body["secret"].(string)
	if secret == "" {
		t.Fatalf("expected a non-empty base32 secret, got %v", body)
	}
	if body["provisioning_uri"] == "" || body["provisioning_uri"] == nil {
		t.Fatalf("expected a provisioning_uri, got %v", body)
	}

	// Not yet active: user_mfa.enabled_at MUST be NULL before verify.
	q := db.New(handlesDB.Write)
	row, err := q.GetUserMFA(t.Context(), userID)
	if err != nil {
		t.Fatalf("get user_mfa: %v", err)
	}
	if row.EnabledAt.Valid {
		t.Fatalf("expected TOTP to remain inactive before verification")
	}
	if string(row.TotpSecretEncrypted) == secret {
		t.Fatalf("expected the stored secret to be ENCRYPTED, never equal to the plaintext base32 secret shown to the caller")
	}

	code := validTOTPCode(t, secret)
	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/me/mfa/verify", map[string]any{
		"code": code,
	}, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on verify, got %d body=%v", resp.StatusCode, body)
	}
	if body["active"] != true {
		t.Fatalf("expected active=true, got %v", body)
	}
	codes, _ := body["recovery_codes"].([]any)
	if len(codes) == 0 {
		t.Fatalf("expected recovery codes on the activating call, got %v", body)
	}

	// Re-read via a SEPARATE query, independent of the handler's own
	// response, proving activation actually persisted.
	row, err = q.GetUserMFA(t.Context(), userID)
	if err != nil {
		t.Fatalf("get user_mfa after verify: %v", err)
	}
	if !row.EnabledAt.Valid {
		t.Fatalf("expected TOTP to be active after a valid verification")
	}
	if len(row.RecoveryCodesHashed) != len(codes) {
		t.Fatalf("expected %d persisted recovery-code hashes, got %d", len(codes), len(row.RecoveryCodesHashed))
	}
	for _, h := range row.RecoveryCodesHashed {
		if h == "" {
			t.Fatalf("expected non-empty recovery-code hashes")
		}
		for _, c := range codes {
			if h == c {
				t.Fatalf("expected recovery codes to be stored HASHED, never equal to the plaintext code shown to the caller")
			}
		}
	}
}

// auth-mfa-totp delta scenario 2 -- "an admin_staff with TOTP enrolled
// verifies via that endpoint" (verification succeeds against an
// ALREADY-ACTIVE factor, no recovery codes repeated).
func TestMFA_VerifyAgainstAlreadyActiveFactorSucceeds(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	userID := createUser(t, handlesDB, "mfa-verify-active@example.com", false)
	accessToken := mintAccessToken(t, deps, handlesDB, userID, false)
	auth := map[string]string{"Authorization": "Bearer " + accessToken}

	_, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/me/mfa/enroll", nil, auth)
	secret, _ := body["secret"].(string)
	firstCode := validTOTPCode(t, secret)
	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/me/mfa/verify", map[string]any{"code": firstCode}, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 activating TOTP, got %d body=%v", resp.StatusCode, body)
	}

	// Wait one full period so the SAME code cannot be replayed (D-P
	// replay protection) -- this call must succeed on its OWN new code,
	// proving steady-state verification, not a replay.
	time.Sleep(30 * time.Second)
	secondCode := validTOTPCode(t, secret)
	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/me/mfa/verify", map[string]any{"code": secondCode}, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 verifying against an already-active factor, got %d body=%v", resp.StatusCode, body)
	}
	if body["active"] != true {
		t.Fatalf("expected active=true, got %v", body)
	}
	if _, ok := body["recovery_codes"]; ok {
		t.Fatalf("expected NO recovery_codes on a non-activating verify, got %v", body)
	}
}

// An invalid code must never activate TOTP (auth-mfa-totp: Enrollment
// requires verification).
func TestMFA_EnrollWithInvalidCodeStaysInactive(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	userID := createUser(t, handlesDB, "mfa-invalid-code@example.com", false)
	accessToken := mintAccessToken(t, deps, handlesDB, userID, false)
	auth := map[string]string{"Authorization": "Bearer " + accessToken}

	doJSON(t, client, http.MethodPost, srv.URL+"/v1/me/mfa/enroll", nil, auth)

	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/me/mfa/verify", map[string]any{"code": "000000"}, auth)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for an invalid code, got %d body=%v", resp.StatusCode, body)
	}

	q := db.New(handlesDB.Write)
	row, err := q.GetUserMFA(t.Context(), userID)
	if err != nil {
		t.Fatalf("get user_mfa: %v", err)
	}
	if row.EnabledAt.Valid {
		t.Fatalf("expected TOTP to remain inactive after an invalid code")
	}
}

// Re-enrolling an already-active factor is refused, never silently
// replacing a live secret.
func TestMFA_ReEnrollAlreadyActiveConflicts(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	userID := createUser(t, handlesDB, "mfa-reenroll@example.com", false)
	accessToken := mintAccessToken(t, deps, handlesDB, userID, false)
	auth := map[string]string{"Authorization": "Bearer " + accessToken}

	_, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/me/mfa/enroll", nil, auth)
	secret, _ := body["secret"].(string)
	code := validTOTPCode(t, secret)
	doJSON(t, client, http.MethodPost, srv.URL+"/v1/me/mfa/verify", map[string]any{"code": code}, auth)

	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/me/mfa/enroll", nil, auth)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 re-enrolling an already-active factor, got %d body=%v", resp.StatusCode, body)
	}
}

// auth-mfa-totp delta: Mandatory TOTP For Admin And Admin_staff Scope
// Access, end-to-end through a REAL admin-scoped route (not just the
// authz-package fixture used by internal/authz/scoped's own tests): an
// admin with NO TOTP is rejected creating a community; after enrolling
// and activating TOTP through THIS phase's own endpoints, the identical
// caller succeeds.
func TestMFA_AdminWithoutTOTPBlockedFromAdminScopedRoute(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	q := db.New(handlesDB.Write)
	office, err := q.InsertOffice(t.Context(), db.InsertOfficeParams{
		ID: uuid.New(), Name: "MFA Gate Office", Cif: "B99000001",
	})
	if err != nil {
		t.Fatalf("insert office: %v", err)
	}
	adminID := createUser(t, handlesDB, "mfa-gate-admin@example.com", false)
	if _, err := q.InsertOfficeMember(t.Context(), db.InsertOfficeMemberParams{
		ID: uuid.New(), OfficeID: office.ID, UserID: adminID, Role: "admin",
	}); err != nil {
		t.Fatalf("insert office member: %v", err)
	}
	// Deliberately NO seedActiveMFA call: this admin has no TOTP.
	adminToken := mintAccessToken(t, deps, handlesDB, adminID, false)
	auth := map[string]string{"Authorization": "Bearer " + adminToken}

	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities", map[string]any{
		"office_id": office.ID.String(), "name": "Blocked Community",
	}, auth)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for an admin with no TOTP, got %d body=%v", resp.StatusCode, body)
	}
	if body["code"] != "AUTH_MFA_ENROLLMENT_REQUIRED" {
		t.Fatalf("expected AUTH_MFA_ENROLLMENT_REQUIRED, got %v", body)
	}

	// Enroll and activate TOTP through this phase's own endpoints, then
	// retry the IDENTICAL request with the IDENTICAL caller.
	_, enrollBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/me/mfa/enroll", nil, auth)
	secret, _ := enrollBody["secret"].(string)
	code := validTOTPCode(t, secret)
	verifyResp, verifyBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/me/mfa/verify", map[string]any{"code": code}, auth)
	if verifyResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 activating TOTP, got %d body=%v", verifyResp.StatusCode, verifyBody)
	}

	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities", map[string]any{
		"office_id": office.ID.String(), "name": "Allowed Community",
	}, auth)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected success once TOTP is active, got %d body=%v", resp.StatusCode, body)
	}
}

// auth-mfa-totp (security correction, review lineage
// review-0e1833930adf141a: R3-mfa-activation-not-atomic-with-recovery-
// codes). Activation and recovery-code issuance are ONE unit: a
// failure after the enabled_at write must roll it back, so the retry
// still takes the activating branch and still hands the caller its
// codes. Otherwise the account keeps an active second factor with an
// empty recovery-code set and no endpoint that can ever refill it.
func TestMFA_ActivationIsAtomicWithRecoveryCodeIssuance(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	userID := createUser(t, handlesDB, "mfa-atomic@example.com", false)
	accessToken := mintAccessToken(t, deps, handlesDB, userID, false)
	auth := map[string]string{"Authorization": "Bearer " + accessToken}

	// Installed before ANY request so the field itself is never written
	// concurrently with a server goroutine reading it; the failure is
	// toggled through the atomic flag instead.
	var failRecoveryCodes atomic.Bool
	deps.RecoveryCodes = func() (raw, hashed []string, err error) {
		if failRecoveryCodes.Load() {
			return nil, nil, errors.New("injected recovery-code failure")
		}
		return mfa.GenerateRecoveryCodes()
	}

	_, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/me/mfa/enroll", nil, auth)
	secret, _ := body["secret"].(string)
	if secret == "" {
		t.Fatalf("expected a base32 secret from enroll, got %v", body)
	}

	failRecoveryCodes.Store(true)
	resp, body := doJSON(t, client, http.MethodPost, srv.URL+"/v1/me/mfa/verify", map[string]any{
		"code": validTOTPCode(t, secret),
	}, auth)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 when recovery-code issuance fails, got %d body=%v", resp.StatusCode, body)
	}

	q := db.New(handlesDB.Write)
	row, err := q.GetUserMFA(t.Context(), userID)
	if err != nil {
		t.Fatalf("get user_mfa: %v", err)
	}
	if row.EnabledAt.Valid {
		t.Fatalf("expected TOTP to remain INACTIVE after a failed activation, got enabled_at=%v -- the account now has an active factor and no recovery path", row.EnabledAt.Time)
	}

	failRecoveryCodes.Store(false)
	resp, body = doJSON(t, client, http.MethodPost, srv.URL+"/v1/me/mfa/verify", map[string]any{
		"code": validTOTPCode(t, secret),
	}, auth)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected the retry to activate TOTP, got %d body=%v", resp.StatusCode, body)
	}
	codes, _ := body["recovery_codes"].([]any)
	if len(codes) == 0 {
		t.Fatalf("expected the retry to still hand out recovery codes, got %v", body)
	}
	row, err = q.GetUserMFA(t.Context(), userID)
	if err != nil {
		t.Fatalf("get user_mfa after retry: %v", err)
	}
	if !row.EnabledAt.Valid || len(row.RecoveryCodesHashed) != len(codes) {
		t.Fatalf("expected an active factor with %d persisted recovery-code hashes, got enabled=%v hashes=%d", len(codes), row.EnabledAt.Valid, len(row.RecoveryCodesHashed))
	}
}
