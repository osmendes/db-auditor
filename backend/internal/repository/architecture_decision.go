package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/osmendes/db-auditor/internal/report"
)

func (s *Store) LatestArchitectureDecisions(ctx context.Context, environmentID, database string) ([]report.ArchitectureDecision, error) {
	out := []report.ArchitectureDecision{}
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ON (object_ref,subject) object_ref,subject,value,version,author,created_at
FROM architecture_decision WHERE environment_id=$1::uuid AND database_name=$2 ORDER BY object_ref,subject,version DESC LIMIT 501`, environmentID, database)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d report.ArchitectureDecision
		if err = rows.Scan(&d.ObjectID, &d.Subject, &d.Value, &d.Version, &d.Author, &d.CreatedAt); err != nil {
			break
		}
		out = append(out, d)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(out) > 500 {
		return nil, fmt.Errorf("too many architecture decisions")
	}
	return out, nil
}

func (s *Store) ArchitectureDecisionHistory(ctx context.Context, environmentID, database string) ([]report.ArchitectureDecision, error) {
	out := []report.ArchitectureDecision{}
	rows, err := s.pool.Query(ctx, `SELECT object_ref,subject,value,version,author,created_at FROM architecture_decision WHERE environment_id=$1::uuid AND database_name=$2 ORDER BY object_ref,subject,version DESC LIMIT 1001`, environmentID, database)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d report.ArchitectureDecision
		if err = rows.Scan(&d.ObjectID, &d.Subject, &d.Value, &d.Version, &d.Author, &d.CreatedAt); err != nil {
			break
		}
		out = append(out, d)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(out) > 1000 {
		return nil, fmt.Errorf("too many architecture decision revisions")
	}
	return out, nil
}

func (s *Store) RecordArchitectureDecision(ctx context.Context, environmentID, database, objectRef, subject, value, author string) (report.ArchitectureDecision, error) {
	var d report.ArchitectureDecision
	validSubject := subject == "semantics" || subject == "insertion" || subject == "unit" || subject == "retention" || subject == "consumer" || subject == "managed_by"
	if database == "" || !strings.HasPrefix(objectRef, database+".") || !validSubject || strings.TrimSpace(value) == "" || author == "" || len(database) > 128 || len(objectRef) > 384 || len(value) > 1000 {
		return d, fmt.Errorf("invalid architecture decision")
	}
	if subject == "insertion" {
		parts := strings.Split(objectRef, ".")
		if len(parts) != 3 || parts[0] != database {
			return d, fmt.Errorf("insertion decision requires database.schema.table")
		}
		if _, err := report.ParseInsertionContract(value); err != nil {
			return d, err
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return d, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, environmentID+"/"+database+"/"+objectRef+"/"+subject); err != nil {
		return d, err
	}
	d.ObjectID = objectRef
	d.Subject = subject
	d.Value = value
	d.Author = author
	err = tx.QueryRow(ctx, `INSERT INTO architecture_decision(environment_id,database_name,object_ref,subject,value,author,version)
SELECT $1::uuid,$2,$3,$4,$5,$6,COALESCE(max(version),0)+1 FROM architecture_decision WHERE environment_id=$1::uuid AND database_name=$2 AND object_ref=$3 AND subject=$4
RETURNING version,created_at`, environmentID, database, objectRef, subject, value, author).Scan(&d.Version, &d.CreatedAt)
	if err != nil {
		return d, err
	}
	return d, tx.Commit(ctx)
}
