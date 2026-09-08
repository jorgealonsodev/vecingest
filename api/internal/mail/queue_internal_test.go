package mail

import (
	"context"
	"log/slog"
	"testing"
)

// Deterministic, non-network unit test for the bounded queue's drop
// behaviour (design D-N: "capped queue"). It constructs an AsyncMailer
// with no running workers, so the channel is never drained -- this is
// a white-box test (package mail, not mail_test) purely to reach the
// unexported jobs field directly and avoid any timing-dependent
// server/goroutine synchronization.
func TestAsyncMailer_SendRaw_DropsWhenQueueIsFull(t *testing.T) {
	m := &AsyncMailer{
		jobs:   make(chan sendJob, 1),
		logger: slog.Default(),
	}

	if err := m.SendRaw(context.Background(), "user@example.com", "s1", "b1"); err != nil {
		t.Fatalf("first SendRaw should have filled the queue slot without error, got: %v", err)
	}
	if err := m.SendRaw(context.Background(), "user@example.com", "s2", "b2"); err == nil {
		t.Fatalf("expected the second SendRaw to be dropped once the bounded queue is full")
	}
}
