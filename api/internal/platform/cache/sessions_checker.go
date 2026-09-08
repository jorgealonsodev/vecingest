package cache

import (
	"context"

	"github.com/google/uuid"

	"github.com/jorgealonsodev/vecingest/internal/db"
)

// SessionsFamilyChecker adapts internal/db's existing
// ListLiveSessionsByFamilyID query to the FamilyLiveChecker port, so
// production wiring needs no extra sqlc query.
type SessionsFamilyChecker struct {
	DB db.DBTX
}

func (c SessionsFamilyChecker) HasLiveSession(ctx context.Context, familyID uuid.UUID) (bool, error) {
	q := db.New(c.DB)
	rows, err := q.ListLiveSessionsByFamilyID(ctx, familyID)
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}
