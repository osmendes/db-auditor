package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/capabilities"
)

type EnvironmentCapabilities struct {
	EnvironmentID string                    `json:"environment_id"`
	Engine        string                    `json:"engine"`
	AuditRunID    string                    `json:"audit_run_id,omitempty"`
	Items         []capabilities.Capability `json:"items"`
}

func (s *Store) GetEnvironmentEngine(ctx context.Context, environmentID string) (string, error) {
	var engine string
	err := s.pool.QueryRow(ctx, `SELECT engine FROM audit_environment WHERE id=$1::uuid`, environmentID).Scan(&engine)
	return engine, err
}

func (s *Store) GetEnvironmentCapabilities(ctx context.Context, environmentID string, qualityEnabled bool) (*EnvironmentCapabilities, error) {
	engine, err := s.GetEnvironmentEngine(ctx, environmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := &EnvironmentCapabilities{EnvironmentID: environmentID, Engine: engine}
	var timescale bool
	err = s.pool.QueryRow(ctx, `SELECT r.id::text,EXISTS(SELECT 1 FROM timescale_version_snapshot v WHERE v.audit_run_id=r.id AND v.extension_name='timescaledb') FROM audit_run r WHERE r.environment_id=$1::uuid AND r.status IN ('success','partial_success') ORDER BY r.started_at DESC,r.id DESC LIMIT 1`, environmentID).Scan(&out.AuditRunID, &timescale)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	out.Items = capabilities.Matrix(engine, timescale, qualityEnabled)
	return out, nil
}
