package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mayconmendes-qc/db-auditor/internal/database/sqlc"
)

type Store struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: sqlc.New(pool)}
}

func (s *Store) Queries() *sqlc.Queries {
	return s.q
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) WithTx(ctx context.Context, fn func(q *sqlc.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (s *Store) CreateEnvironment(ctx context.Context, name, envType, discoveryMode string, active bool) (sqlc.AuditEnvironment, error) {
	return s.q.CreateEnvironment(ctx, sqlc.CreateEnvironmentParams{
		Name: name, Type: envType, DiscoveryMode: discoveryMode, Active: active,
	})
}

func (s *Store) ListActiveEnvironments(ctx context.Context) ([]sqlc.AuditEnvironment, error) {
	return s.q.ListActiveEnvironments(ctx)
}

func (s *Store) StartAuditRun(ctx context.Context, environmentID pgtype.UUID, profile, serviceVersion, collectorVersion, collectorName string) (sqlc.AuditRun, sqlc.CollectorRun, error) {
	var run sqlc.AuditRun
	var collector sqlc.CollectorRun
	err := s.WithTx(ctx, func(q *sqlc.Queries) error {
		var err error
		run, err = q.CreateAuditRun(ctx, sqlc.CreateAuditRunParams{
			EnvironmentID: environmentID, Profile: profile, Status: "running",
			ServiceVersion: serviceVersion, CollectorVersion: collectorVersion,
			Warnings: []byte("[]"), Errors: []byte("[]"),
		})
		if err != nil {
			return err
		}
		collector, err = q.CreateCollectorRun(ctx, sqlc.CreateCollectorRunParams{
			AuditRunID: run.ID, CollectorName: collectorName, CollectorVersion: collectorVersion,
			Status: "running", QueryName: pgtype.Text{},
		})
		return err
	})
	return run, collector, err
}

func (s *Store) FinishAuditRun(ctx context.Context, id pgtype.UUID, status string, warnings, errorsPayload any) (sqlc.AuditRun, error) {
	w, err := json.Marshal(warnings)
	if err != nil {
		return sqlc.AuditRun{}, err
	}
	e, err := json.Marshal(errorsPayload)
	if err != nil {
		return sqlc.AuditRun{}, err
	}
	return s.q.FinishAuditRun(ctx, sqlc.FinishAuditRunParams{ID: id, Status: status, Warnings: w, Errors: e})
}

func UTCNow() time.Time { return time.Now().UTC() }
