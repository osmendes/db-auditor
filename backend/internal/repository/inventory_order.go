package repository

import (
	"context"
	"strings"

	"github.com/mayconmendes-qc/db-auditor/internal/database/sqlc"
)

// inventoryOrder accepts only known snapshot columns. The final id tie breaker
// makes pagination deterministic even when many objects have the same value.
func inventoryOrder(raw string, columns map[string]string, fallback string) string {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return fallback
	}
	column, ok := columns[parts[0]]
	if !ok || (parts[1] != "asc" && parts[1] != "desc") {
		return fallback
	}
	direction := " ASC"
	if parts[1] == "desc" {
		direction = " DESC"
	}
	return column + direction + " NULLS LAST, id ASC"
}

func orderedInventorySQL(query, raw string, columns map[string]string) string {
	if raw == "" {
		return query
	}
	start := strings.LastIndex(query, "\nORDER BY ")
	end := strings.LastIndex(query, "\nLIMIT ")
	if start < 0 || end < start {
		return query
	}
	fallback := strings.TrimSpace(strings.TrimPrefix(query[start:end], "\nORDER BY "))
	return query[:start] + "\nORDER BY " + inventoryOrder(raw, columns, fallback) + query[end:]
}

var tableOrderColumns = map[string]string{
	"database": "database_name", "schema": "schema_name", "name": "table_name",
	"type": "relation_class", "cols": "column_count", "rows": "row_estimate",
	"size": "total_size_bytes", "owner": "owner_name", "pk": "has_primary_key",
}

// The generated sqlc query remains the default path. This bounded variant is
// used only for an allowlisted user-selected order.
func (s *Store) listOrderedTableSnapshots(ctx context.Context, f InventoryFilter, env any, q string, limit int) ([]sqlc.ListTableSnapshotsFilteredRow, error) {
	const base = `SELECT id::text,audit_run_id::text,environment_id::text,database_name,schema_name,table_name,
owner_name,COALESCE(relkind,''),COALESCE(relation_class,''),COALESCE(is_partition,false),
total_size_bytes,data_size_bytes,index_size_bytes,COALESCE(row_estimate,0),COALESCE(column_count,0),
COALESCE(has_primary_key,false),collected_at FROM table_snapshot
WHERE environment_id=$1::uuid AND audit_run_id=COALESCE(NULLIF($2,'')::uuid,
(SELECT id FROM audit_run WHERE environment_id=$1::uuid AND status IN ('success','partial_success') ORDER BY started_at DESC,id DESC LIMIT 1))
AND ($3='' OR database_name=$3) AND ($4='' OR schema_name=$4)
AND ($5='' OR table_name=$5) AND ($6='' OR relation_class=$6)
AND ($7='' OR table_name ILIKE $7 OR schema_name ILIKE $7 OR database_name ILIKE $7)
ORDER BY `
	order := inventoryOrder(f.OrderBy, tableOrderColumns, "database_name,schema_name,table_name,id")
	rows, err := s.pool.Query(ctx, base+order+` LIMIT $8 OFFSET $9`, env, f.AuditRunID, f.Database, f.Schema, f.Table, f.RelationClass, q, limit, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []sqlc.ListTableSnapshotsFilteredRow{}
	for rows.Next() {
		var item sqlc.ListTableSnapshotsFilteredRow
		if err := rows.Scan(&item.ID, &item.AuditRunID, &item.EnvironmentID, &item.DatabaseName,
			&item.SchemaName, &item.TableName, &item.OwnerName, &item.Relkind, &item.RelationClass,
			&item.IsPartition, &item.TotalSizeBytes, &item.DataSizeBytes, &item.IndexSizeBytes,
			&item.RowEstimate, &item.ColumnCount, &item.HasPrimaryKey, &item.CollectedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
