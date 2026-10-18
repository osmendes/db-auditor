package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/osmendes/db-auditor/internal/guidance"
	"github.com/osmendes/db-auditor/internal/impact"
)

// GainStatement is the API shape of impact.Gain.
type GainStatement = impact.Gain

func (s *Store) latestMeasuredGain(ctx context.Context, findingID string) (*impact.Measurement, error) {
	var m impact.Measurement
	err := s.pool.QueryRow(ctx, `SELECT metric,before_value,after_value,comparable AND workload_comparable,comparison_note,after_run_id::text
FROM finding_action_measurement WHERE finding_id=$1::uuid ORDER BY recorded_at DESC,id DESC LIMIT 1`, findingID).Scan(&m.Metric, &m.Before, &m.After, &m.Comparable, &m.Note, &m.AfterRunID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// BuildGainStatement decodes evidence and keeps hypothesis, predicted and
// measured gain apart (BA-21).
func BuildGainStatement(rule string, evidence []byte, estimate impact.Estimate, measured *impact.Measurement) GainStatement {
	var decoded map[string]any
	_ = json.Unmarshal(evidence, &decoded)
	return impact.BuildGain(guidance.For(rule).Meaning, decoded, estimate, measured)
}
