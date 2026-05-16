package migrate

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.up.sql
var migrationFS embed.FS

const provisionalMigrationFloor int64 = 900000

var finalizedMigrationAliases = map[int64]int64{
	900001: 371,
}

type schemaVersionQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func ensureSchemaMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY NOT NULL,
			dirty   BOOLEAN NOT NULL DEFAULT false
		)`)
	if err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}
	return nil
}

func repairFinalizedMigrationAliases(ctx context.Context, pool *pgxpool.Pool) error {
	for provisional, final := range finalizedMigrationAliases {
		if _, err := pool.Exec(ctx, `
			UPDATE schema_migrations
			   SET version = $2
			 WHERE version = $1
			   AND dirty = false
			   AND NOT EXISTS (
				   SELECT 1 FROM schema_migrations WHERE version = $2
			   )`, provisional, final); err != nil {
			return fmt.Errorf("repair finalized migration alias %d -> %d: %w", provisional, final, err)
		}
		if _, err := pool.Exec(ctx, `
			DELETE FROM schema_migrations
			 WHERE version = $1
			   AND dirty = false
			   AND EXISTS (
				   SELECT 1 FROM schema_migrations WHERE version = $2
			   )`, provisional, final); err != nil {
			return fmt.Errorf("remove finalized migration alias %d after %d exists: %w", provisional, final, err)
		}
	}
	return nil
}

func normalizeFinalizedMigrationAlias(version int64) int64 {
	if final, ok := finalizedMigrationAliases[version]; ok {
		return final
	}
	return version
}

// CurrentVersion returns the effective schema version for a live database,
// repairing known finalized provisional migration numbers first.
func CurrentVersion(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	if err := ensureSchemaMigrations(ctx, pool); err != nil {
		return 0, err
	}
	if err := repairFinalizedMigrationAliases(ctx, pool); err != nil {
		return 0, err
	}
	return ReadSchemaVersion(ctx, pool)
}

// ReadSchemaVersion returns the effective schema version visible to q. It does
// not mutate state, so it is safe inside read-only transactions.
func ReadSchemaVersion(ctx context.Context, q schemaVersionQuerier) (int64, error) {
	var current int64
	if err := q.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return 0, fmt.Errorf("read current migration version: %w", err)
	}

	var dirty bool
	if err := q.QueryRow(ctx, `SELECT dirty FROM schema_migrations WHERE version = $1`, current).Scan(&dirty); err != nil {
		if current != 0 {
			return 0, fmt.Errorf("check dirty state for version %d: %w", current, err)
		}
	} else if dirty {
		return 0, fmt.Errorf("migration version %d is dirty; manual intervention required", current)
	}

	current = normalizeFinalizedMigrationAlias(current)
	if current >= provisionalMigrationFloor {
		return 0, fmt.Errorf("schema_migrations contains unfinalized provisional version %d; finalize or repair migration numbering before continuing", current)
	}
	return current, nil
}

// Run reads the current schema version from schema_migrations and applies
// all embedded *.up.sql files with a higher version number, each inside
// its own transaction. It creates the schema_migrations table if missing.
func Run(ctx context.Context, pool *pgxpool.Pool) error {
	current, err := CurrentVersion(ctx, pool)
	if err != nil {
		return err
	}

	// List and sort embedded migration files
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}

	type migration struct {
		version int64
		name    string
	}
	var pending []migration
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		// Parse version from filename like "000018_autotag_system.up.sql"
		parts := strings.SplitN(name, "_", 2)
		if len(parts) < 2 {
			continue
		}
		v, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			continue
		}
		if v > current {
			pending = append(pending, migration{version: v, name: name})
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].version < pending[j].version })

	// L49: Warn about gaps in migration numbering (may indicate missing files).
	if len(pending) > 0 {
		expected := current + 1
		for _, m := range pending {
			if m.version != expected {
				slog.Warn("migration number gap detected", "expected", expected, "found", m.version)
			}
			expected = m.version + 1
		}
	}

	if len(pending) == 0 {
		slog.Info("migrations up to date", "version", current)
		return nil
	}

	slog.Info("pending migrations", "current", current, "pending", len(pending))

	for _, m := range pending {
		sql, err := migrationFS.ReadFile(path.Join("migrations", m.name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", m.name, err)
		}

		// Mark version as dirty before executing
		if _, err := pool.Exec(ctx,
			`INSERT INTO schema_migrations (version, dirty) VALUES ($1, true)
			 ON CONFLICT (version) DO UPDATE SET dirty = true`, m.version); err != nil {
			return fmt.Errorf("mark migration %d dirty: %w", m.version, err)
		}

		// Execute migration in a transaction
		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for migration %d: %w", m.version, err)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("execute migration %d (%s): %w", m.version, m.name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}

		// Mark clean and remove old version row
		if _, err := pool.Exec(ctx, `DELETE FROM schema_migrations WHERE version < $1`, m.version); err != nil {
			return fmt.Errorf("clean old versions after %d: %w", m.version, err)
		}
		if _, err := pool.Exec(ctx,
			`UPDATE schema_migrations SET dirty = false WHERE version = $1`, m.version); err != nil {
			return fmt.Errorf("mark migration %d clean: %w", m.version, err)
		}

		slog.Info("migration applied", "version", m.version, "file", m.name)
	}

	slog.Info("all migrations applied", "version", pending[len(pending)-1].version)
	return nil
}
