package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/jorgealonsodev/vecingest/internal/db"
)

const fakeDSN = "postgres://user:pass@localhost:5432/vecingest" //nolint:gosec // G101: fake fixture DSN, pgxpool.NewWithConfig never connects eagerly so no real credential is needed

// platform-bootstrap: Per-Role Database Write and Read Handles --
// "Write and read handles are distinct pool objects". pgxpool.NewWithConfig
// does not eagerly connect (MinConns defaults to 0), so this needs no
// real database and is not Testcontainers-gated.
func TestNewServeHandles_ReadAndWriteHaveDistinctMaxConns(t *testing.T) {
	handles, err := db.NewServeHandles(context.Background(), fakeDSN, fakeDSN)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer handles.Close()

	writeMax := handles.Write.Stat().MaxConns()
	readMax := handles.Read.Stat().MaxConns()
	if writeMax == readMax {
		t.Fatalf("expected distinct MaxConns for write vs read pools, both were %d", writeMax)
	}
	if writeMax != db.DefaultWriteMaxConns {
		t.Fatalf("expected write pool MaxConns %d, got %d", db.DefaultWriteMaxConns, writeMax)
	}
	if readMax != db.DefaultReadMaxConns {
		t.Fatalf("expected read pool MaxConns %d, got %d", db.DefaultReadMaxConns, readMax)
	}
}

// platform-bootstrap: Per-Role Database Write and Read Handles -- serve
// pools use QueryExecModeCacheDescribe (D-R point 2).
func TestNewServeHandles_UsesCacheDescribeExecMode(t *testing.T) {
	handles, err := db.NewServeHandles(context.Background(), fakeDSN, fakeDSN)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer handles.Close()

	if mode := handles.Write.Config().ConnConfig.DefaultQueryExecMode; mode != pgx.QueryExecModeCacheDescribe {
		t.Fatalf("expected serve write pool to use QueryExecModeCacheDescribe, got %v", mode)
	}
	if mode := handles.Read.Config().ConnConfig.DefaultQueryExecMode; mode != pgx.QueryExecModeCacheDescribe {
		t.Fatalf("expected serve read pool to use QueryExecModeCacheDescribe, got %v", mode)
	}
}

// platform-bootstrap: Per-Role Database Write and Read Handles -- worker's
// pool does NOT use QueryExecModeCacheDescribe: River and LISTEN/NOTIFY
// cannot live behind a transaction pooler (D-R point 2).
func TestNewWorkerHandle_DoesNotUseCacheDescribeExecMode(t *testing.T) {
	handle, err := db.NewWorkerHandle(context.Background(), fakeDSN)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer handle.Close()

	if mode := handle.Config().ConnConfig.DefaultQueryExecMode; mode == pgx.QueryExecModeCacheDescribe {
		t.Fatalf("expected worker pool to NOT use QueryExecModeCacheDescribe, got %v", mode)
	}
}

// D-R point 3: "A read handle cannot reach a transaction -- at compile
// time -- for every caller outside internal/db." Go has no positive way
// to assert "this type does NOT implement an interface", so the
// guarantee is proven the only way Go allows: the converse compiles
// (WriteDB satisfies transactor) and the negative case is the exact line
// that must fail to compile if ever uncommented.
type transactor interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

var _ transactor = db.WriteDB{}

// The following, if uncommented, MUST NOT compile -- ReadDB has no
// Begin method, by design, for every caller outside internal/db:
//
//	var _ transactor = db.ReadDB{}
