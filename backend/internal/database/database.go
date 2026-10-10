// Package database owns the auditor's internal PostgreSQL connection pool.
package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

func NewPool(ctx context.Context, settings config.Database) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(settings.URL)
	if err != nil {
		return nil, fmt.Errorf("parse snapshot store URL: %w", err)
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = settings.ApplicationName
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprintf("%d", settings.StatementTimeout.Milliseconds())
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = fmt.Sprintf("%d", settings.LockTimeout.Milliseconds())
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open snapshot store pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping snapshot store: %w", err)
	}
	return pool, nil
}
