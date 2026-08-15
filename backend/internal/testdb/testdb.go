// Package testdb gives tests a real, migrated Postgres.
//
// Repository tests run against real Postgres because the invariants are
// enforced by triggers and constraints — a mocked database would test nothing
// that matters. The tests must be able to assert that inserting a post-kickoff
// prediction *fails*, and only Postgres can tell them that.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
)

// EnvURL points at a Postgres *server* the test run may create databases on.
// Each test gets its own database, so parallel packages cannot see each
// other's rows and a failed test leaves an inspectable corpse behind only if
// KATAFA_TEST_KEEP_DB is set.
const EnvURL = "TEST_DATABASE_URL"

// New returns a migrated, empty database scoped to this test.
//
// Skips — rather than fails — when TEST_DATABASE_URL is unset, so `go test ./...`
// on a machine with no Docker still runs every pure test. CI sets it.
//
//	docker run --rm -d --name katafa-pg -p 5433:5432 \
//	  -e POSTGRES_USER=katafa -e POSTGRES_PASSWORD=katafa -e POSTGRES_DB=katafa postgres:16
//	export TEST_DATABASE_URL='postgres://katafa:katafa@localhost:5433/katafa?sslmode=disable'
func New(t *testing.T) *postgres.DB {
	t.Helper()

	adminURL := os.Getenv(EnvURL)
	if adminURL == "" {
		t.Skipf("%s not set; skipping tests that need real Postgres", EnvURL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	name := "katafa_test_" + randomSuffix(t)

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to %s: %v", EnvURL, err)
	}
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+quoteIdent(name)); err != nil {
		admin.Close(ctx)
		t.Fatalf("create database %s: %v", name, err)
	}
	if err := admin.Close(ctx); err != nil {
		t.Fatalf("close admin connection: %v", err)
	}

	db, err := postgres.Open(ctx, replaceDatabase(t, adminURL, name))
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	if err := postgres.MigrateUp(ctx, db.Pool); err != nil {
		db.Close()
		t.Fatalf("migrate %s: %v", name, err)
	}

	t.Cleanup(func() {
		db.Close()
		if os.Getenv("KATAFA_TEST_KEEP_DB") != "" {
			t.Logf("kept test database %s", name)
			return
		}
		// A fresh context: the test's may already be cancelled, and leaking
		// databases across a long run eventually fills the volume.
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dropCancel()

		conn, err := pgx.Connect(dropCtx, adminURL)
		if err != nil {
			t.Logf("could not connect to drop %s: %v", name, err)
			return
		}
		defer func() { _ = conn.Close(dropCtx) }()
		if _, err := conn.Exec(dropCtx, `DROP DATABASE IF EXISTS `+quoteIdent(name)+` WITH (FORCE)`); err != nil {
			t.Logf("could not drop %s: %v", name, err)
		}
	})

	return db
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("random suffix: %v", err)
	}
	return hex.EncodeToString(b[:])
}

func replaceDatabase(t *testing.T, raw, name string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", EnvURL, err)
	}
	u.Path = "/" + name
	return u.String()
}

// quoteIdent quotes a generated identifier. The names here are built from hex
// and a fixed prefix, but interpolating an identifier without quoting is a
// habit worth not having.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// MustExec fails the test on error. For arranging fixtures, where a failed
// setup statement is a broken test rather than an assertion.
func MustExec(t *testing.T, db *postgres.DB, sql string, args ...any) {
	t.Helper()
	if _, err := db.Pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %s: %v", firstLine(sql), err)
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " …"
	}
	return s
}
