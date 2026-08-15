package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/Profy256/katafasoccerpredictions/backend/migrations"
)

func gooseDB(pool *pgxpool.Pool) (*sql.DB, error) {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return nil, fmt.Errorf("set dialect: %w", err)
	}
	return stdlib.OpenDBFromPool(pool), nil
}

// MigrateUp applies every pending migration.
func MigrateUp(ctx context.Context, pool *pgxpool.Pool) error {
	db, err := gooseDB(pool)
	if err != nil {
		return err
	}
	defer db.Close()
	return goose.UpContext(ctx, db, migrations.Dir)
}

// MigrateDown rolls back the most recent migration. Development only: there is
// no path that calls this in production, because rolling back a migration that
// drops published predictions is not a recoverable operation.
func MigrateDown(ctx context.Context, pool *pgxpool.Pool) error {
	db, err := gooseDB(pool)
	if err != nil {
		return err
	}
	defer db.Close()
	return goose.DownContext(ctx, db, migrations.Dir)
}

// MigrationStatus prints applied and pending migrations.
func MigrationStatus(ctx context.Context, pool *pgxpool.Pool) error {
	db, err := gooseDB(pool)
	if err != nil {
		return err
	}
	defer db.Close()
	return goose.StatusContext(ctx, db, migrations.Dir)
}

// MigrationsCurrent reports whether the database is fully migrated. GET
// /readyz fails when it is not — an API serving against a half-migrated schema
// produces errors that look like application bugs.
func MigrationsCurrent(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	db, err := gooseDB(pool)
	if err != nil {
		return false, err
	}
	defer db.Close()

	applied, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return false, fmt.Errorf("read applied version: %w", err)
	}
	all, err := goose.CollectMigrations(migrations.Dir, 0, goose.MaxVersion)
	if err != nil {
		return false, fmt.Errorf("collect migrations: %w", err)
	}
	if len(all) == 0 {
		return false, fmt.Errorf("no migrations embedded")
	}
	return applied >= all[len(all)-1].Version, nil
}
