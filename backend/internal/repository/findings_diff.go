package repository

import (
	"context"
)

// FindingChange is one row of a snapshot-store diff between two runs.
type FindingChange struct {
	Change    string `json:"change"`
	ObjectKey string `json:"object_key"`
	Title     string `json:"title"`
	Severity  string `json:"severity"`
	FindingID string `json:"finding_id,omitempty"`
}

// DiffRunFindings classifies observed findings as added, removed, or unchanged.
// It reads only finding_event rows already stored for the two runs.
func (s *Store) DiffRunFindings(ctx context.Context, environmentID, fromRun, toRun string) ([]FindingChange, error) {
	rows, err := s.pool.Query(ctx, `
WITH src AS (
  SELECT DISTINCT ON (object_name, title) finding_id::text AS finding_id,
    database_name || '.' || schema_name || '.' || object_name AS object_key,
    title, severity
  FROM finding_event
  WHERE audit_run_id=$2::uuid AND event_type='observed'
  ORDER BY object_name, title, recorded_at DESC
), dst AS (
  SELECT DISTINCT ON (object_name, title) finding_id::text AS finding_id,
    database_name || '.' || schema_name || '.' || object_name AS object_key,
    title, severity
  FROM finding_event
  WHERE audit_run_id=$3::uuid AND event_type='observed'
  ORDER BY object_name, title, recorded_at DESC
)
SELECT 'removed', src.object_key, src.title, src.severity, src.finding_id
FROM src LEFT JOIN dst USING (object_key, title) WHERE dst.title IS NULL
UNION ALL
SELECT 'added', dst.object_key, dst.title, dst.severity, dst.finding_id
FROM dst LEFT JOIN src USING (object_key, title) WHERE src.title IS NULL
UNION ALL
SELECT 'unchanged', dst.object_key, dst.title, dst.severity, dst.finding_id
FROM dst JOIN src USING (object_key, title)
ORDER BY 1, 2, 3`, environmentID, fromRun, toRun)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FindingChange{}
	for rows.Next() {
		var item FindingChange
		if err = rows.Scan(&item.Change, &item.ObjectKey, &item.Title, &item.Severity, &item.FindingID); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
