package postgres

import (
	"context"
	"embed"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const createMigrationTableSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version TEXT PRIMARY KEY,
	applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)
`

func Migrate(
	ctx context.Context,
	pool *pgxpool.Pool,
) error {
	if pool == nil {
		return ErrPoolRequired
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf(
			"read embedded migrations: %w",
			err,
		)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	err = pgx.BeginFunc(
		ctx,
		pool,
		func(tx pgx.Tx) error {
			if _, err := tx.Exec(
				ctx,
				"SELECT pg_advisory_xact_lock("+
					"hashtext('pulseops_schema_migrations')::bigint)",
			); err != nil {
				return fmt.Errorf(
					"lock schema migrations: %w",
					err,
				)
			}

			if _, err := tx.Exec(
				ctx,
				createMigrationTableSQL,
			); err != nil {
				return fmt.Errorf(
					"create schema_migrations: %w",
					err,
				)
			}

			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}

				version := entry.Name()

				var applied bool

				if err := tx.QueryRow(
					ctx,
					`
						SELECT EXISTS (
							SELECT 1
							FROM schema_migrations
							WHERE version = $1
						)
					`,
					version,
				).Scan(&applied); err != nil {
					return fmt.Errorf(
						"check migration %s: %w",
						version,
						err,
					)
				}

				if applied {
					continue
				}

				body, err := migrationFiles.ReadFile(
					"migrations/" + version,
				)
				if err != nil {
					return fmt.Errorf(
						"read migration %s: %w",
						version,
						err,
					)
				}

				if _, err := tx.Exec(
					ctx,
					string(body),
				); err != nil {
					return fmt.Errorf(
						"apply migration %s: %w",
						version,
						err,
					)
				}

				if _, err := tx.Exec(
					ctx,
					`
						INSERT INTO schema_migrations (version)
						VALUES ($1)
					`,
					version,
				); err != nil {
					return fmt.Errorf(
						"record migration %s: %w",
						version,
						err,
					)
				}
			}

			return nil
		},
	)
	if err != nil {
		return fmt.Errorf(
			"run PostgreSQL migrations: %w",
			err,
		)
	}

	return nil
}
