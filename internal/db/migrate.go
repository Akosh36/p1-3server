package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type migration struct {
	version int
	name    string
	upSQL   string
}

// Migrate applies every embedded *.up.sql migration that has not yet been
// recorded in the schema_migrations table, in ascending version order.
// It is safe to call on every process start: already-applied migrations are
// skipped.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     INTEGER PRIMARY KEY,
			name        TEXT NOT NULL,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, m := range migrations {
		var alreadyApplied bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`,
			m.version,
		).Scan(&alreadyApplied)
		if err != nil {
			return fmt.Errorf("check migration %d: %w", m.version, err)
		}
		if alreadyApplied {
			continue
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for migration %d: %w", m.version, err)
		}
		if _, err := tx.Exec(ctx, m.upSQL); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("apply migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`,
			m.version, m.name,
		); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
	}
	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.Glob(migrationsFS, "migrations/*.up.sql")
	if err != nil {
		return nil, fmt.Errorf("glob migrations: %w", err)
	}

	migrations := make([]migration, 0, len(entries))
	for _, path := range entries {
		base := strings.TrimSuffix(path, ".up.sql")
		base = strings.TrimPrefix(base, "migrations/")

		var version int
		var name string
		if _, err := fmt.Sscanf(base, "%04d_%s", &version, &name); err != nil {
			return nil, fmt.Errorf("migration file %q must be named NNNN_name.up.sql: %w", path, err)
		}

		content, err := migrationsFS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", path, err)
		}

		migrations = append(migrations, migration{version: version, name: name, upSQL: string(content)})
	}

	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	return migrations, nil
}
