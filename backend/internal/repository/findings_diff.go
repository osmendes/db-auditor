package repository

import (
	"context"
	"errors"
)

// ErrDiffRunNotFound means at least one of the compared runs does not exist in
// the requested environment.
var ErrDiffRunNotFound = errors.New("diff run not found in environment")

// FindingChange is one row of a snapshot-store diff between two runs.
type FindingChange struct {
	Change    string `json:"change"`
	ObjectKey string `json:"object_key"`
	Title     string `json:"title"`
	Severity  string `json:"severity"`
	FindingID string `json:"finding_id,omitempty"`
}

// DiffRunFindings classifies observed findings as added, removed, or unchanged.
// It reads only finding_event rows already stored for the two runs. Both runs
// must belong to environmentID; otherwise ErrDiffRunNotFound is returned.
func (s *Store) DiffRunFindings(ctx context.Context, environmentID, fromRun, toRun string) ([]FindingChange, error) {
	var found int
	if err := s.pool.QueryRow(ctx, `
SELECT count(DISTINCT id)
FROM audit_run
WHERE environment_id=$1::uuid AND id IN ($2::uuid, $3::uuid)`, environmentID, fromRun, toRun).Scan(&found); err != nil {
		return nil, err
	}
	want := 2
	if fromRun == toRun {
		want = 1
	}
	if found != want {
		return nil, ErrDiffRunNotFound
	}
	rows, err := s.pool.Query(ctx, `
WITH runs AS (
  SELECT id FROM audit_run WHERE environment_id=$1::uuid AND id IN ($2::uuid, $3::uuid)
), src AS (
  SELECT DISTINCT ON (object_name, title) finding_id::text AS finding_id,
    database_name || '.' || schema_name || '.' || object_name AS object_key,
    title, severity
  FROM finding_event
  WHERE audit_run_id=$2::uuid AND audit_run_id IN (SELECT id FROM runs) AND event_type='observed'
  ORDER BY object_name, title, recorded_at DESC
), dst AS (
  SELECT DISTINCT ON (object_name, title) finding_id::text AS finding_id,
    database_name || '.' || schema_name || '.' || object_name AS object_key,
    title, severity
  FROM finding_event
  WHERE audit_run_id=$3::uuid AND audit_run_id IN (SELECT id FROM runs) AND event_type='observed'
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
