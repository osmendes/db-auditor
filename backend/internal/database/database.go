// Package database owns the auditor's internal PostgreSQL connection pool.
package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/osmendes/db-auditor/internal/config"
)

// poolDriver holds the pgxpool entry points so tests can inject errors.
type poolDriver struct {
	parse func(string) (*pgxpool.Config, error)
	open  func(context.Context, *pgxpool.Config) (*pgxpool.Pool, error)
	ping  func(context.Context, *pgxpool.Pool) error
}

var pgxpoolDriver = poolDriver{
	parse: pgxpool.ParseConfig,
	open:  pgxpool.NewWithConfig,
	ping:  func(ctx context.Context, pool *pgxpool.Pool) error { return pool.Ping(ctx) },
}

func NewPool(ctx context.Context, settings config.Database) (*pgxpool.Pool, error) {
	return newPool(ctx, settings, pgxpoolDriver)
}

// Every error is redacted against the snapshot store URL: cmd/auditor logs it
// at boot.
func newPool(ctx context.Context, settings config.Database, driver poolDriver) (*pgxpool.Pool, error) {
	cfg, err := driver.parse(settings.URL)
	if err != nil {
		return nil, fmt.Errorf("parse snapshot store URL: %w", config.RedactError(err, settings.URL))
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = settings.ApplicationName
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprintf("%d", settings.StatementTimeout.Milliseconds())
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = fmt.Sprintf("%d", settings.LockTimeout.Milliseconds())
	pool, err := driver.open(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open snapshot store pool: %w", config.RedactError(err, settings.URL))
	}
	if err := driver.ping(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping snapshot store: %w", config.RedactError(err, settings.URL))
	}
	return pool, nil
}
