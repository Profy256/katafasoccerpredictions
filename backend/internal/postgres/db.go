// Package postgres owns the connection pool, the transaction helper, and every
// SQL statement in the system. Postgres is the only datastore: it also carries
// the job queue and the rate limiter, so there is one thing to back up and one
// thing to restore.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is satisfied by both *pgxpool.Pool and pgx.Tx, so every query below
// runs inside or outside a transaction without a second implementation.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// DB is the pool. Query methods hang off it and take a Querier as their first
// argument when they need to participate in a caller's transaction.
type DB struct {
	Pool *pgxpool.Pool
}

// Open connects and verifies reachability before returning. A process that
// cannot reach Postgres should fail to boot, not start serving 500s.
func Open(ctx context.Context, databaseURL string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	cfg.MaxConns = 16
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 15 * time.Minute
	cfg.HealthCheckPeriod = time.Minute

	// Every timestamp in this system is TIMESTAMPTZ in UTC. Pinning the
	// session timezone means a server with a local TZ set cannot shift a
	// kickoff time, which would silently move a settlement window.
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &DB{Pool: pool}, nil
}

func (db *DB) Close() { db.Pool.Close() }

// Ping backs GET /readyz.
func (db *DB) Ping(ctx context.Context) error { return db.Pool.Ping(ctx) }

// InTx runs fn inside a transaction, rolling back on error or panic.
//
// This is what makes settlement exactly-once rather than approximately-once: a
// River job's completion commits in the same transaction as the rows it
// produced, so a crash mid-batch replays cleanly instead of leaving half a
// matchday graded.
func (db *DB) InTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	// Rollback after a successful commit is a no-op, so this is safe on every
	// path and guarantees no connection leaks on panic.
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
