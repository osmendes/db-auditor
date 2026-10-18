package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/osmendes/db-auditor/internal/collectors/postgres"
	"github.com/osmendes/db-auditor/internal/report"
)

// InsertionMappings freezes the latest confirmed insertion contracts when a
// manual collector visits each database. Invalid legacy decisions are omitted.
func (s *Store) InsertionMappings(ctx context.Context, environmentID, database string) ([]postgres.InsertionMapping, error) {
	decisions, err := s.LatestArchitectureDecisions(ctx, environmentID, database)
	if err != nil {
		return nil, err
	}
	out := make([]postgres.InsertionMapping, 0)
	for _, d := range decisions {
		if d.Subject != "insertion" {
			continue
		}
		parts := strings.Split(d.ObjectID, ".")
		contract, err := report.ParseInsertionContract(d.Value)
		if err != nil || len(parts) != 3 || parts[0] != database {
			continue
		}
		out = append(out, postgres.InsertionMapping{Database: database, Schema: parts[1], Table: parts[2], Column: contract.Column, AppendOnly: contract.AppendOnly, DecisionVersion: d.Version})
	}
	return out, nil
}

func (s *Store) SaveInsertionProbes(ctx context.Context, environmentID, auditRunID pgtype.UUID, probes []postgres.InsertionProbe) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, p := range probes {
		if p.Database == "" || p.Schema == "" || p.Table == "" || p.DecisionVersion < 1 {
			return fmt.Errorf("invalid insertion probe scope")
		}
		_, err = tx.Exec(ctx, `INSERT INTO architecture_insertion_snapshot
(environment_id,audit_run_id,database_name,schema_name,table_name,column_name,decision_version,append_only,status,reason,latest_at,collected_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (audit_run_id,database_name,schema_name,table_name) DO NOTHING`,
			environmentID, auditRunID, p.Database, p.Schema, p.Table, p.Column, p.DecisionVersion, p.AppendOnly, p.Status, p.Reason, p.Latest, p.CollectedAt)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
