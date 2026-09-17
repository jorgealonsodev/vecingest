package api_test

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/db"
)

// blockingHIBP stands in for the third-party breach-check dependency
// PasswordPolicy reaches in the production wiring (cmd/vecingest/serve.go
// builds it with hibp.NewClient). It parks inside IsBreached until the
// test releases it, which is how a test can observe what the accept
// handler is HOLDING while the policy's external call is outstanding --
// the real thing would just be slow and unobservable.
type blockingHIBP struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingHIBP) IsBreached(context.Context, string) (bool, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return false, nil
}

// R3-accept-invitation-holds-write-tx-across-password-work (review
// lineage review-c4efc3f92d076299). POST /v1/auth/accept-invitation is
// unauthenticated and public. It used to open a primary-pool
// transaction, take a row lock on the invitation via the failed-attempts
// UPDATE, and only THEN run password-policy validation, password.Verify
// and password.Hash, committing at the very end -- so every request
// pinned a write connection and an invitation row lock for the full
// duration of a deliberately slow password primitive plus whatever the
// policy's third-party dependency cost. That is a cheap denial of
// service against the primary pool.
//
// The probe below is the assertion: while the handler is parked inside
// the policy, an independent connection must be able to take the
// invitation row lock. Under the old ordering it could not.
func TestInvitation_AcceptDoesNotHoldTheInvitationRowLockAcrossPasswordWork(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-lockwindow-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Lock Window Community")
	unitID := seedUnit(t, handlesDB, communityID)
	auth := map[string]string{"Authorization": "Bearer " + mintAccessToken(t, deps, handlesDB, adminID, false)}

	_, createBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": "invite-lockwindow-new@example.com", "role": "owner",
	}, auth)
	shortCode, _ := createBody["short_code"].(string)
	invitationIDStr, _ := createBody["id"].(string)
	invitationID, err := uuid.Parse(invitationIDStr)
	if err != nil {
		t.Fatalf("test setup: expected an invitation id in the creation response, got %v", createBody)
	}

	blocker := &blockingHIBP{entered: make(chan struct{}), release: make(chan struct{})}
	deps.PasswordPolicy.HIBP = blocker

	done := make(chan int, 1)
	go func() {
		resp, _ := doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, "203.0.113.90",
			map[string]any{
				"short_code": shortCode, "name": "Lock Window User", "password": testPassword,
				"consent": true, "platform": "ios",
			}, inviteHeaders("ios", "1.0.0"))
		done <- resp.StatusCode
	}()

	<-blocker.entered
	lockErr := probeInvitationRowLock(t, handlesDB, invitationID)
	close(blocker.release)

	if status := <-done; status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("test setup: expected the accept itself to succeed, got %d", status)
	}
	if lockErr != nil {
		t.Fatalf("expected the invitation row to be lockable by an independent connection while accept-invitation is inside the password policy: an unauthenticated endpoint must not pin a primary write connection and a row lock across slow password work, got %v", lockErr)
	}
}

// probeInvitationRowLock tries to take the invitation's row lock on an
// INDEPENDENT connection with a short lock_timeout, and reports the
// error instead of failing, so the caller can release the blocked
// handler before asserting (a t.Fatalf here would deadlock the
// goroutine parked in IsBreached).
func probeInvitationRowLock(t *testing.T, handlesDB db.Handles, invitationID uuid.UUID) error {
	t.Helper()
	tx, err := handlesDB.Write.Begin(t.Context())
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	if _, err := tx.Exec(t.Context(), "SET LOCAL lock_timeout = '2s'"); err != nil {
		return err
	}
	var id uuid.UUID
	return tx.QueryRow(t.Context(), `SELECT id FROM invitations WHERE id = $1 FOR UPDATE`, invitationID).Scan(&id)
}

// The invariant the transaction exists to protect, which the reordering
// above must not cost: single use. Two accepts of the SAME short code
// issued concurrently must produce exactly one success -- single use is
// enforced by the conditional UPDATE inside the transaction (design D-6:
// "Single use is enforced by the write, not by a prior read"), never by
// how long the transaction happens to be held open. No test covered
// concurrent accepts before this one.
func TestInvitation_ConcurrentAcceptsOfTheSameCodeYieldExactlyOneSuccess(t *testing.T) {
	srv, deps, handlesDB := newTestServer(t)
	client := newClient(srv, nil)

	officeID, adminID := seedOfficeWithAdmin(t, handlesDB, "invite-concurrent-admin@example.com")
	communityID := seedCommunity(t, handlesDB, officeID, "Invite Concurrent Community")
	unitID := seedUnit(t, handlesDB, communityID)
	auth := map[string]string{"Authorization": "Bearer " + mintAccessToken(t, deps, handlesDB, adminID, false)}

	newEmail := "invite-concurrent-new@example.com"
	_, createBody := doJSON(t, client, http.MethodPost, srv.URL+"/v1/communities/"+communityID.String()+"/invitations", map[string]any{
		"unit_id": unitID.String(), "email": newEmail, "role": "owner",
	}, auth)
	shortCode, _ := createBody["short_code"].(string)

	const attempts = 2
	statuses := make([]int, attempts)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resp, _ := doFromIPClient(t, client, srv.URL+"/v1/auth/accept-invitation", http.MethodPost, "203.0.113.91",
				map[string]any{
					"short_code": shortCode, "name": "Concurrent User", "password": testPassword,
					"consent": true, "platform": "ios",
				}, inviteHeaders("ios", "1.0.0"))
			statuses[i] = resp.StatusCode
		}()
	}
	close(start)
	wg.Wait()

	accepted := 0
	for _, s := range statuses {
		if s == http.StatusOK || s == http.StatusCreated {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("expected EXACTLY ONE of two concurrent accepts of the same short code to succeed, got %d (statuses %v)", accepted, statuses)
	}

	var members int
	if err := handlesDB.Write.QueryRow(t.Context(),
		`SELECT count(*) FROM unit_members WHERE unit_id = $1`, unitID,
	).Scan(&members); err != nil {
		t.Fatalf("count unit members: %v", err)
	}
	if members != 1 {
		t.Fatalf("expected exactly one unit_members row after two concurrent accepts, got %d", members)
	}
}
