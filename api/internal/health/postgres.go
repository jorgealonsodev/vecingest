package health

import (
	"context"
	"fmt"
)

// Pinger is satisfied by db.WriteDB/db.ReadDB (both expose Ping).
type Pinger interface {
	Ping(ctx context.Context) error
}

// PostgresCheck is M0's sole registered ReadinessCheck (D-Q): a
// pool.Ping plus implicit connectivity assertion on the serve pool.
type PostgresCheck struct {
	DB Pinger
}

func (PostgresCheck) Name() string { return "postgres" }

func (c PostgresCheck) Check(ctx context.Context) error {
	if err := c.DB.Ping(ctx); err != nil {
		return fmt.Errorf("postgres: ping failed: %w", err)
	}
	return nil
}
