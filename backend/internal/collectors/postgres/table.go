package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

// CollectTables lists user relations (table, partitioned, partition, foreign) and applies schema scope.
func CollectTables(ctx context.Context, conn *pgx.Conn, scope config.Scope) ([]TableFacts, error) {
	rows, err := conn.Query(ctx, tablesSQL)
	if err != nil {
		return nil, fmt.Errorf("table collector: %w", err)
	}
	defer rows.Close()

	out := make([]TableFacts, 0)
	for rows.Next() {
		var f TableFacts
		var lastVacuum, lastAutovacuum, lastAnalyze, lastAutoanalyze *time.Time
		var parentSchema, parentTable, partitionBound, tablespace, tableComment *string
		if err := rows.Scan(
			&f.DatabaseName,
			&f.SchemaName,
			&f.TableName,
			&f.Owner,
			&f.Relkind,
			&f.IsPartition,
			&parentSchema,
			&parentTable,
			&partitionBound,
			&tablespace,
			&f.RelPersistence,
			&f.RelRowSecurity,
			&f.RelForceRowSecurity,
			&tableComment,
			&f.StorageParameters,
			&f.DataSizeBytes,
			&f.IndexSizeBytes,
			&f.TotalSizeBytes,
			&f.RowEstimate,
			&f.NLiveTup,
			&f.NDeadTup,
			&f.NTupIns,
			&f.NTupUpd,
			&f.NTupDel,
			&f.SeqScan,
			&f.IdxScan,
			&lastVacuum,
			&lastAutovacuum,
			&lastAnalyze,
			&lastAutoanalyze,
			&f.ColumnCount,
			&f.HasPrimaryKey,
			&f.StatsReset,
		); err != nil {
			return nil, fmt.Errorf("table collector scan: %w", err)
		}
		f.ParentSchemaName = parentSchema
		f.ParentTableName = parentTable
		f.PartitionBound = partitionBound
		f.TablespaceName = tablespace
		f.TableComment = tableComment
		f.RelationClass = ClassifyRelation(f.Relkind, f.IsPartition)
		f.LastVacuum = lastVacuum
		f.LastAutovacuum = lastAutovacuum
		f.LastAnalyze = lastAnalyze
		f.LastAutoanalyze = lastAutoanalyze
		if f.DataSizeBytes < 0 {
			f.DataSizeBytes = 0
		}
		if f.IndexSizeBytes < 0 {
			f.IndexSizeBytes = 0
		}
		if f.TotalSizeBytes < 0 {
			f.TotalSizeBytes = 0
		}
		if !scope.AllowsSchema(f.SchemaName) {
			continue
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("table collector rows: %w", err)
	}
	return out, nil
}
