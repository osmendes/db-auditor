package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

const columnStatsSQL = `
SELECT current_database(), schemaname, tablename, attname,
       null_frac::float8, n_distinct::float8, avg_width,
       correlation::float8
FROM pg_stats
WHERE schemaname NOT LIKE 'pg\_%' ESCAPE '\'
  AND schemaname <> 'information_schema'
  AND (COALESCE(cardinality($1::text[]), 0) = 0 OR schemaname = ANY($1::text[]))
  AND NOT COALESCE(schemaname = ANY($2::text[]), false)
ORDER BY schemaname, tablename, attname
LIMIT $3
`

const workloadAvailableSQL = `
SELECT COALESCE((SELECT extversion FROM pg_extension WHERE extname = 'pg_stat_statements'), '')
`

const workloadSQL = `
SELECT current_database(), queryid::text, query, calls, total_exec_time,
       mean_exec_time, rows, shared_blks_read, shared_blks_hit
FROM pg_stat_statements
WHERE dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
ORDER BY total_exec_time DESC
LIMIT $1
`

var (
	blockCommentPattern = regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineCommentPattern  = regexp.MustCompile(`(?m)--[^\n]*`)
	quotedPattern       = regexp.MustCompile(`'(?:''|[^'])*'`)
	dollarPattern       = regexp.MustCompile(`(?s)\$[A-Za-z_0-9]*\$.*?\$[A-Za-z_0-9]*\$`)
	numberPattern       = regexp.MustCompile(`\b\d+(?:\.\d+)?\b`)
	spacePattern        = regexp.MustCompile(`\s+`)
	objectPattern       = regexp.MustCompile(`(?i)\b(?:from|join|update|into)\s+([a-zA-Z_][\w$]*(?:\.[a-zA-Z_][\w$]*)?)`)
)

// NormalizeQuery removes comments and literal values. It is intentionally
// conservative and is never used to reconstruct or execute SQL.
func NormalizeQuery(query string) string {
	q := blockCommentPattern.ReplaceAllString(query, " ")
	q = lineCommentPattern.ReplaceAllString(q, " ")
	q = dollarPattern.ReplaceAllString(q, "?")
	q = quotedPattern.ReplaceAllString(q, "?")
	q = numberPattern.ReplaceAllString(q, "?")
	return strings.TrimSpace(spacePattern.ReplaceAllString(strings.ToLower(q), " "))
}

func queryFingerprint(normalized string) string {
	sum := sha256.Sum256([]byte(normalized))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// QueryKind is a coarse, privacy-safe operation class. Complex/unknown SQL
// remains "other"; no raw query text is persisted.
func QueryKind(normalized string) string {
	fields := strings.Fields(normalized)
	if len(fields) == 0 {
		return "other"
	}
	switch fields[0] {
	case "select", "insert", "update", "delete":
		return fields[0]
	default:
		return "other"
	}
}

func referencedObjects(normalized string) []string {
	seen := map[string]struct{}{}
	for _, match := range objectPattern.FindAllStringSubmatch(normalized, -1) {
		// Unqualified names are ambiguous across schemas and must not be
		// attributed to a specific table or used for table-level findings.
		if len(match) > 1 && strings.Contains(match[1], ".") {
			seen[strings.ToLower(match[1])] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func CollectColumnStats(ctx context.Context, conn *pgx.Conn, scope config.Scope, schemas []string, limit int) ([]ColumnStatFacts, error) {
	return CollectColumnStatsWithBudget(ctx, conn, scope, schemas, limit, 0)
}

func CollectColumnStatsWithBudget(ctx context.Context, conn *pgx.Conn, scope config.Scope, schemas []string, limit int, maxPlanCost float64) ([]ColumnStatFacts, error) {
	_, _ = conn.Exec(ctx, "SET statement_timeout = '120s'")
	if limit <= 0 || limit > 50000 {
		limit = 5000
	}
	allowed := schemas
	if len(scope.SchemaAllowlist) > 0 {
		allowed = make([]string, 0, len(scope.SchemaAllowlist))
		for _, global := range scope.SchemaAllowlist {
			if len(schemas) == 0 {
				allowed = append(allowed, global)
				continue
			}
			for _, specific := range schemas {
				if global == specific {
					allowed = append(allowed, global)
					break
				}
			}
		}
		if len(allowed) == 0 {
			return []ColumnStatFacts{}, nil
		}
	}
	if err := checkPlanBudget(ctx, conn, maxPlanCost, columnStatsSQL, allowed, scope.SchemaDenylist, limit); err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx, columnStatsSQL, allowed, scope.SchemaDenylist, limit)
	if err != nil {
		return nil, fmt.Errorf("column stats collector: %w", err)
	}
	defer rows.Close()
	out := make([]ColumnStatFacts, 0)
	for rows.Next() {
		var fact ColumnStatFacts
		if err := rows.Scan(&fact.DatabaseName, &fact.SchemaName, &fact.TableName, &fact.ColumnName,
			&fact.NullFraction, &fact.DistinctEstimate, &fact.AverageWidth, &fact.Correlation); err != nil {
			return nil, fmt.Errorf("column stats collector scan: %w", err)
		}
		if !scope.AllowsSchema(fact.SchemaName) {
			continue
		}
		fact.Source = "pg_stats"
		fact.Quality = "estimate"
		out = append(out, fact)
	}
	return out, rows.Err()
}

func CollectWorkload(ctx context.Context, conn *pgx.Conn, limit int) ([]WorkloadFacts, bool, error) {
	return CollectWorkloadWithBudget(ctx, conn, limit, 0)
}

func CollectWorkloadWithBudget(ctx context.Context, conn *pgx.Conn, limit int, maxPlanCost float64) ([]WorkloadFacts, bool, error) {
	_, _ = conn.Exec(ctx, "SET statement_timeout = '120s'")
	var version string
	if err := conn.QueryRow(ctx, workloadAvailableSQL).Scan(&version); err != nil {
		return nil, false, fmt.Errorf("detect pg_stat_statements: %w", err)
	}
	if version == "" {
		return nil, false, nil
	}
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	if err := checkPlanBudget(ctx, conn, maxPlanCost, workloadSQL, limit); err != nil {
		return nil, true, err
	}
	// pg_stat_database has an independent reset clock. If the extension's
	// own clock cannot be read, retain an unknown observation start.
	var reset *time.Time
	_ = conn.QueryRow(ctx, `SELECT stats_reset FROM pg_stat_statements_info`).Scan(&reset)
	rows, err := conn.Query(ctx, workloadSQL, limit)
	if err != nil {
		return nil, true, fmt.Errorf("workload collector: %w", err)
	}
	defer rows.Close()
	out := make([]WorkloadFacts, 0)
	for rows.Next() {
		var fact WorkloadFacts
		var query, queryID *string
		if err := rows.Scan(&fact.DatabaseName, &queryID, &query, &fact.Calls,
			&fact.TotalExecTimeMS, &fact.MeanExecTimeMS, &fact.RowsTotal,
			&fact.SharedBlocksRead, &fact.SharedBlocksHit); err != nil {
			return nil, true, fmt.Errorf("workload collector scan: %w", err)
		}
		if query == nil || queryID == nil || *query == "" {
			continue
		}
		fact.QueryID = *queryID
		normalized := NormalizeQuery(*query)
		fact.QueryFingerprint = queryFingerprint(normalized)
		fact.ReferencedObjects = referencedObjects(normalized)
		fact.StatsReset = reset
		fact.ExtensionVersion = version
		fact.QueryKind = QueryKind(normalized)
		fact.EvidenceQuality = "aggregate"
		if len(fact.ReferencedObjects) > 0 {
			fact.EvidenceQuality = "object_reference"
		}
		out = append(out, fact)
	}
	return out, true, rows.Err()
}

func checkPlanBudget(ctx context.Context, conn *pgx.Conn, maxCost float64, sql string, args ...any) error {
	if maxCost <= 0 {
		return nil
	}
	var raw []byte
	if err := conn.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+sql, args...).Scan(&raw); err != nil {
		return fmt.Errorf("optional collector plan unavailable: %w", err)
	}
	var plan []struct {
		Plan struct {
			TotalCost float64 `json:"Total Cost"`
		} `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil || len(plan) != 1 {
		return fmt.Errorf("optional collector plan invalid")
	}
	if plan[0].Plan.TotalCost > maxCost {
		return fmt.Errorf("optional collector plan cost %.0f exceeds budget %.0f", plan[0].Plan.TotalCost, maxCost)
	}
	return nil
}
