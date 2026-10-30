// Package db provides PostgreSQL connectivity with per-service schemas and embedded migrations.
package db

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pool whose search_path is pinned to the service's own schema,
// so a service can only see the tables it owns.
func Connect(ctx context.Context, url, schema string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrate applies the *.sql files in migrations (sorted by name) that have not run yet.
func Migrate(ctx context.Context, pool *pgxpool.Pool, schema string, migrations fs.FS) error {
	if _, err := pool.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, pgx.Identifier{schema}.Sanitize())); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	files, err := fs.Glob(migrations, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, name := range files {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		sql, err := fs.ReadFile(migrations, name)
		if err != nil {
			return err
		}
		err = InTx(ctx, pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return fmt.Errorf("migration %s: %w", name, err)
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES ($1)`, name)
			return err
		})
		if err != nil {
			return err
		}
		slog.Info("applied migration", "schema", schema, "name", strings.TrimSuffix(name, ".sql"))
	}
	return nil
}

// InTx runs fn inside a transaction, committing on success and rolling back on error.
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Nullable converts an empty string to NULL for optional uuid/text columns.
func Nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
