package repository

import (
	"context"
	"time"

	"github.com/mayconmendes-qc/db-auditor/internal/database/sqlc"
)

type InventoryFilter struct {
	EnvironmentID string
	AuditRunID    string
	Database      string
	Schema        string
	Table         string
	RelationClass string
	Q             string
	Limit         int
	Offset        int
	OrderBy       string
}

type TableSnapshotRow struct {
	ID             string    `json:"id"`
	AuditRunID     string    `json:"audit_run_id"`
	EnvironmentID  string    `json:"environment_id"`
	DatabaseName   string    `json:"database_name"`
	SchemaName     string    `json:"schema_name"`
	TableName      string    `json:"table_name"`
	OwnerName      *string   `json:"owner_name"`
	Relkind        string    `json:"relkind"`
	RelationClass  string    `json:"relation_class"`
	IsPartition    bool      `json:"is_partition"`
	TotalSizeBytes int64     `json:"total_size_bytes"`
	DataSizeBytes  int64     `json:"data_size_bytes"`
	IndexSizeBytes int64     `json:"index_size_bytes"`
	RowEstimate    int64     `json:"row_estimate"`
	ColumnCount    int       `json:"column_count"`
	HasPrimaryKey  bool      `json:"has_primary_key"`
	CollectedAt    time.Time `json:"collected_at"`
}

type ColumnSnapshotRow struct {
	ID                 string    `json:"id"`
	DatabaseName       string    `json:"database_name"`
	SchemaName         string    `json:"schema_name"`
	TableName          string    `json:"table_name"`
	ColumnName         string    `json:"column_name"`
	OrdinalPosition    int       `json:"ordinal_position"`
	DataType           string    `json:"data_type"`
	IsNullable         bool      `json:"is_nullable"`
	ColumnDefault      *string   `json:"column_default"`
	IsGenerated        bool      `json:"is_generated"`
	IdentityGeneration *string   `json:"identity_generation"`
	CollationName      *string   `json:"collation_name"`
	Comment            *string   `json:"comment,omitempty"`
	CollectedAt        time.Time `json:"collected_at"`
}

type IndexSnapshotRow struct {
	ID              string    `json:"id"`
	DatabaseName    string    `json:"database_name"`
	SchemaName      string    `json:"schema_name"`
	TableName       string    `json:"table_name"`
	IndexName       string    `json:"index_name"`
	IndexDefinition string    `json:"index_definition"`
	AccessMethod    *string   `json:"access_method"`
	IsUnique        bool      `json:"is_unique"`
	IsPrimary       bool      `json:"is_primary"`
	SizeBytes       int64     `json:"size_bytes"`
	IdxScan         int64     `json:"idx_scan"`
	CollectedAt     time.Time `json:"collected_at"`
}

type ViewSnapshotRow struct {
	ID           string    `json:"id"`
	DatabaseName string    `json:"database_name"`
	SchemaName   string    `json:"schema_name"`
	ViewName     string    `json:"view_name"`
	OwnerName    *string   `json:"owner_name"`
	Relkind      string    `json:"relkind"`
	SizeBytes    int64     `json:"size_bytes"`
	CollectedAt  time.Time `json:"collected_at"`
}

type FunctionSnapshotRow struct {
	ID                string    `json:"id"`
	DatabaseName      string    `json:"database_name"`
	SchemaName        string    `json:"schema_name"`
	FunctionName      string    `json:"function_name"`
	IdentityArguments string    `json:"identity_arguments"`
	OwnerName         *string   `json:"owner_name"`
	LanguageName      *string   `json:"language_name"`
	IsSecurityDefiner bool      `json:"is_security_definer"`
	Kind              *string   `json:"kind"`
	CollectedAt       time.Time `json:"collected_at"`
}

func (s *Store) ListTableSnapshots(ctx context.Context, f InventoryFilter) ([]TableSnapshotRow, int, error) {
	limit, offset := f.Limit, f.Offset
	if limit <= 0 {
		limit = 50
	}
	q := f.Q
	if q != "" {
		q = "%" + q + "%"
	}
	env, err := parseUUID(f.EnvironmentID)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountTableSnapshots(ctx, sqlc.CountTableSnapshotsParams{
		Column1: env,
		Column2: f.AuditRunID,
		Column3: f.Database,
		Column4: f.Schema,
		Column5: f.Table,
		Column6: f.RelationClass,
		Column7: q,
	})
	if err != nil {
		return nil, 0, err
	}
	params := sqlc.ListTableSnapshotsFilteredParams{
		Column1: env,
		Column2: f.AuditRunID,
		Column3: f.Database,
		Column4: f.Schema,
		Column5: f.Table,
		Column6: f.RelationClass,
		Column7: q,
		Limit:   int32(limit),
		Offset:  int32(offset),
	}
	var rows []sqlc.ListTableSnapshotsFilteredRow
	if f.OrderBy == "" {
		rows, err = s.q.ListTableSnapshotsFiltered(ctx, params)
	} else {
		rows, err = s.listOrderedTableSnapshots(ctx, f, env, q, limit)
	}
	if err != nil {
		return nil, 0, err
	}
	out := make([]TableSnapshotRow, 0, len(rows))
	for _, row := range rows {
		item := TableSnapshotRow{
			ID: row.ID, AuditRunID: row.AuditRunID, EnvironmentID: row.EnvironmentID,
			DatabaseName: row.DatabaseName, SchemaName: row.SchemaName, TableName: row.TableName,
			Relkind: row.Relkind, RelationClass: row.RelationClass, IsPartition: row.IsPartition,
			TotalSizeBytes: row.TotalSizeBytes, DataSizeBytes: row.DataSizeBytes, IndexSizeBytes: row.IndexSizeBytes,
			RowEstimate: row.RowEstimate, ColumnCount: int(row.ColumnCount), HasPrimaryKey: row.HasPrimaryKey,
		}
		if row.OwnerName.Valid {
			owner := row.OwnerName.String
			item.OwnerName = &owner
		}
		if row.CollectedAt.Valid {
			item.CollectedAt = row.CollectedAt.Time
		}
		out = append(out, item)
	}
	return out, int(total), nil
}
