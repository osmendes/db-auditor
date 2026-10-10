// Package migrate applies snapshot-store SQL once and records a checksum.
package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const advisoryLock int64 = 842020

// Action is what the boot migrator does with one SQL file.
type Action int

const (
	Skip Action = iota
	Stamp
	Execute
)

// Decide chooses how to treat a migration file.
// A baseline that already exists in the database is stamped, never reapplied.
func Decide(version, checksum string, applied map[string]string, schemaPresent bool) (Action, error) {
	if prev, ok := applied[version]; ok {
		if prev != checksum {
			return Skip, fmt.Errorf("checksum of %s changed after it was applied", version)
		}
		return Skip, nil
	}
	if version == "01_baseline.sql" && schemaPresent {
		return Stamp, nil
	}
	return Execute, nil
}

// Apply runs pending migrations from dir. 02_seed_demo.sql stays with the
// empty-volume entrypoint and is never executed here.
func Apply(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	if dir == "" {
		dir = "migrations"
	}
	files, err := listSQL(dir)
	if err != nil {
		return err
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, advisoryLock); err != nil {
		return fmt.Errorf("migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, advisoryLock)
	}()
	if _, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migration (
		version text PRIMARY KEY,
		checksum text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("schema_migration: %w", err)
	}
	applied, err := loadApplied(ctx, conn)
	if err != nil {
		return err
	}
	present, err := schemaPresent(ctx, conn)
	if err != nil {
		return err
	}
	for _, name := range files {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		sum := checksum(body)
		action, err := Decide(name, sum, applied, present)
		if err != nil {
			return err
		}
		switch action {
		case Skip:
			continue
		case Execute:
			if err = applyFile(ctx, conn, string(body)); err != nil {
				return fmt.Errorf("apply %s: %w", name, err)
			}
			present = true
		case Stamp:
			if err = requireCurrentSchema(ctx, conn); err != nil {
				return err
			}
		}
		if _, err = conn.Exec(ctx, `INSERT INTO schema_migration(version, checksum) VALUES ($1, $2)`, name, sum); err != nil {
			return fmt.Errorf("record %s: %w", name, err)
		}
		applied[name] = sum
	}
	return nil
}

// applyFile runs one migration. CREATE INDEX CONCURRENTLY cannot sit inside a
// transaction; every other file commits or rolls back as a unit, so a failure
// never leaves schema_migration recorded for that version.
func applyFile(ctx context.Context, conn *pgxpool.Conn, body string) error {
	if strings.Contains(strings.ToUpper(body), "CONCURRENTLY") {
		_, err := conn.Conn().PgConn().Exec(ctx, body).ReadAll()
		return err
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, body); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func listSQL(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") || name == "02_seed_demo.sql" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func checksum(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

type execQuery interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func loadApplied(ctx context.Context, conn execQuery) (map[string]string, error) {
	rows, err := conn.Query(ctx, `SELECT version, checksum FROM schema_migration`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var version, sum string
		if err = rows.Scan(&version, &sum); err != nil {
			return nil, err
		}
		out[version] = sum
	}
	return out, rows.Err()
}

func schemaPresent(ctx context.Context, conn execQuery) (bool, error) {
	rows, err := conn.Query(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'audit_environment'
	)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	var present bool
	if rows.Next() {
		if err = rows.Scan(&present); err != nil {
			return false, err
		}
	}
	return present, rows.Err()
}

func requireCurrentSchema(ctx context.Context, conn execQuery) error {
	rows, err := conn.Query(ctx, `SELECT
		EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'finding' AND column_name = 'rule_version'),
		EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'report_job')`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var finding, reports bool
	if rows.Next() {
		if err = rows.Scan(&finding, &reports); err != nil {
			return err
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if !finding || !reports {
		return fmt.Errorf("snapshot store is behind the squashed baseline; refusing to reapply 01_baseline.sql")
	}
	return nil
}
