// Package db provides per-process-role database handles (D-R). ReadDB
// and WriteDB are distinct named types so that a read handle cannot
// reach a transaction at compile time for every caller outside this
// package: only WriteDB exposes Begin/BeginTx.
//
// Isolation level (D-P): every pool built here (NewServeHandles,
// NewWorkerHandle) runs at pgx's default transaction isolation, which is
// PostgreSQL's own default of READ COMMITTED. This is a load-bearing
// fact for internal/domain/auth/mfa.VerifyTOTP's replay protection:
// "zero rows affected means a concurrent request already consumed that
// step" is a property of READ COMMITTED specifically -- under
// REPEATABLE READ or SERIALIZABLE, PostgreSQL raises SQLSTATE 40001
// (serialization failure) on the losing UPDATE instead of silently
// affecting zero rows. Do not add WriteDB.BeginTx call sites that raise
// pgx.TxOptions.IsoLevel for the MFA verification transaction; if
// isolation is ever raised for any reason, mfa.VerifyTOTP's
// pgerrcode.SerializationFailure classification is what keeps that a
// clean rejection instead of an unhandled 500.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Default pool sizes for M0 (PRD §7.7: max_connections 40, pgx pool of
// 10 per process). The read pool defaults smaller because at M0 it
// shares the same node as the write pool (phase B is what actually
// separates read replicas), so it should not compete evenly for the
// same 40-connection budget.
const (
	DefaultWriteMaxConns int32 = 10
	DefaultReadMaxConns  int32 = 5
)

// ReadDB exposes Query/QueryRow/Exec only. It wraps its own
// *pgxpool.Pool instance rather than aliasing WriteDB's, and it does not
// expose the underlying pool, so a caller outside this package cannot
// reach Begin/BeginTx through it (D-R point 3).
type ReadDB struct {
	pool *pgxpool.Pool
}

func (r ReadDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return r.pool.Query(ctx, sql, args...)
}

func (r ReadDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return r.pool.QueryRow(ctx, sql, args...)
}

func (r ReadDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return r.pool.Exec(ctx, sql, args...)
}

func (r ReadDB) Ping(ctx context.Context) error { return r.pool.Ping(ctx) }

// Stat exposes pool-level introspection (MaxConns, acquired/idle counts)
// without exposing the pool itself, which would let a caller reach
// Begin/BeginTx directly and defeat the whole point of this split.
func (r ReadDB) Stat() *pgxpool.Stat { return r.pool.Stat() }

// Config exposes pool configuration for introspection/testing (e.g.
// asserting DefaultQueryExecMode). It returns configuration data, not
// the pool itself, so it grants no transaction capability.
func (r ReadDB) Config() *pgxpool.Config { return r.pool.Config() }

func (r ReadDB) Close() { r.pool.Close() }

// WriteDB is the only type in this package exposing Begin/BeginTx.
// Every domain service constructor takes ReadDB for pure reads and
// WriteDB for everything else: anything inside a transaction, and every
// read-then-write sequence, takes WriteDB.
type WriteDB struct {
	pool *pgxpool.Pool
}

func (w WriteDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return w.pool.Query(ctx, sql, args...)
}

func (w WriteDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return w.pool.QueryRow(ctx, sql, args...)
}

func (w WriteDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return w.pool.Exec(ctx, sql, args...)
}

func (w WriteDB) Ping(ctx context.Context) error { return w.pool.Ping(ctx) }

func (w WriteDB) Stat() *pgxpool.Stat { return w.pool.Stat() }

// Config exposes pool configuration for introspection/testing (e.g.
// asserting DefaultQueryExecMode). It returns configuration data, not
// the pool itself, so it grants no transaction capability.
func (w WriteDB) Config() *pgxpool.Config { return w.pool.Config() }

func (w WriteDB) Close() { w.pool.Close() }

// Pool exposes the underlying *pgxpool.Pool. It exists solely for
// wiring a third-party client that needs the concrete pgxpool type
// itself (D-L: riverpgxv5.New(*pgxpool.Pool) for the worker's River
// client) rather than the DBTX-shaped Query/Exec surface every domain
// service uses. It grants no capability WriteDB does not already expose
// (Begin/BeginTx are public above); it only exposes the concrete type.
func (w WriteDB) Pool() *pgxpool.Pool { return w.pool }

func (w WriteDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return w.pool.Begin(ctx)
}

func (w WriteDB) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error) {
	return w.pool.BeginTx(ctx, txOptions)
}

// Handles is the per-process pair of database handles (db.Handles).
type Handles struct {
	Write WriteDB
	Read  ReadDB
}

func (h Handles) Close() {
	h.Write.Close()
	h.Read.Close()
}

// poolConfig builds a *pgxpool.Config from dsn with maxConns applied,
// and -- only when useCacheDescribe is true -- DefaultQueryExecMode set
// to QueryExecModeCacheDescribe. When useCacheDescribe is false, the
// exec mode is left untouched at pgx's own default, which is exactly
// what "worker's do not [use QueryExecModeCacheDescribe]" (D-R point 2)
// requires: not forcing a different mode, simply not opting into this one.
func poolConfig(dsn string, maxConns int32, useCacheDescribe bool) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("db: parse pool config: %w", err)
	}
	cfg.MaxConns = maxConns
	if useCacheDescribe {
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe
	}
	return cfg, nil
}

func newPool(ctx context.Context, dsn string, maxConns int32, useCacheDescribe bool) (*pgxpool.Pool, error) {
	cfg, err := poolConfig(dsn, maxConns, useCacheDescribe)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: create pool: %w", err)
	}
	return pool, nil
}

// NewServeHandles builds serve's ReadDB/WriteDB pair: Write on writeDSN
// (DATABASE_URL) and Read on readDSN (DATABASE_URL_READ, already
// resolved to fall back to DATABASE_URL by internal/config -- D-R point
// 4). Both pools are configured with QueryExecModeCacheDescribe so
// PgBouncer transaction pooling drops in at phase B with no code change
// (§7.7; D-R point 2). Read is always a distinct *pgxpool.Pool instance
// from Write, even when both DSNs are identical (D-R point 1).
func NewServeHandles(ctx context.Context, writeDSN, readDSN string) (Handles, error) {
	writePool, err := newPool(ctx, writeDSN, DefaultWriteMaxConns, true)
	if err != nil {
		return Handles{}, fmt.Errorf("db: build serve write pool: %w", err)
	}
	readPool, err := newPool(ctx, readDSN, DefaultReadMaxConns, true)
	if err != nil {
		writePool.Close()
		return Handles{}, fmt.Errorf("db: build serve read pool: %w", err)
	}
	return Handles{
		Write: WriteDB{pool: writePool},
		Read:  ReadDB{pool: readPool},
	}, nil
}

// NewWorkerHandle builds worker's single write-capable handle from
// workerDSN (DATABASE_URL_WORKER, already resolved to fall back to
// DATABASE_URL -- D-R point 4) as a direct connection with pgx's default
// exec mode: River and LISTEN/NOTIFY cannot live behind a transaction
// pooler (§7.7; D-R point 2).
func NewWorkerHandle(ctx context.Context, workerDSN string) (WriteDB, error) {
	pool, err := newPool(ctx, workerDSN, DefaultWriteMaxConns, false)
	if err != nil {
		return WriteDB{}, fmt.Errorf("db: build worker pool: %w", err)
	}
	return WriteDB{pool: pool}, nil
}
