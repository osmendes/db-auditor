// Package quality runs opt-in, bounded aggregate probes against an audited
// PostgreSQL database. It never reads row values into the auditor process.
package quality

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mayconmendes-qc/db-auditor/internal/config"
)

type DateRange struct {
	Column string `json:"column"`
	From   string `json:"from"`
	To     string `json:"to"`
}

type Request struct {
	Database        string      `json:"database"`
	Schema          string      `json:"schema"`
	Table           string      `json:"table"`
	Limit           int         `json:"limit"`
	ExpectedNonNull []string    `json:"expected_non_null"`
	CandidateKeys   []string    `json:"candidate_keys"`
	DateRanges      []DateRange `json:"date_ranges"`
}

type Issue struct {
	Kind         string `json:"check_kind"`
	Column       string `json:"column_name"`
	AffectedRows int    `json:"affected_rows"`
	SampledRows  int    `json:"sampled_rows"`
}

type Result struct {
	SampledRows  int     `json:"sampled_rows"`
	SampleMethod string  `json:"sample_method"`
	Issues       []Issue `json:"issues"`
}

func (r Request) Validate() error {
	if r.Database == "" || r.Schema == "" || r.Table == "" || r.Limit < 1 || r.Limit > 1000 || len(r.ExpectedNonNull)+len(r.CandidateKeys)+len(r.DateRanges) > 32 {
		return fmt.Errorf("escopo ou limite inválido")
	}
	for _, name := range append(append([]string{}, r.ExpectedNonNull...), r.CandidateKeys...) {
		if name == "" || len(name) > 63 {
			return fmt.Errorf("coluna inválida")
		}
	}
	for _, date := range r.DateRanges {
		from, e1 := time.Parse(time.RFC3339, date.From)
		to, e2 := time.Parse(time.RFC3339, date.To)
		if date.Column == "" || len(date.Column) > 63 || e1 != nil || e2 != nil || !from.Before(to) {
			return fmt.Errorf("intervalo de data inválido")
		}
	}
	return nil
}

func Run(ctx context.Context, dsn string, request Request) (Result, error) {
	result := Result{Issues: []Issue{}}
	if err := request.Validate(); err != nil {
		return result, err
	}
	if strings.TrimSpace(dsn) == "" {
		return result, fmt.Errorf("conexão do ambiente não configurada")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return result, fmt.Errorf("conexão inválida")
	}
	cfg.Database = request.Database
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return result, fmt.Errorf("falha na conexão: %s", config.SanitizeError(err))
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer closeCancel()
		_ = conn.Close(closeCtx)
	}()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, fmt.Errorf("transação somente leitura indisponível")
	}
	defer func() {
		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer rollbackCancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	if _, err = tx.Exec(ctx, `SET LOCAL statement_timeout='5000ms'`); err != nil {
		return result, fmt.Errorf("limite de tempo indisponível")
	}
	var privileged bool
	if err = tx.QueryRow(ctx, `SELECT rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&privileged); err != nil {
		return result, fmt.Errorf("não foi possível verificar o papel de leitura")
	}
	if privileged {
		return result, fmt.Errorf("use uma conta de coleta sem privilégios administrativos")
	}
	columns := map[string]string{}
	rows, err := tx.Query(ctx, `SELECT a.attname,format_type(a.atttypid,a.atttypmod) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_attribute a ON a.attrelid=c.oid WHERE n.nspname=$1 AND c.relname=$2 AND c.relkind IN ('r','p') AND a.attnum>0 AND NOT a.attisdropped AND has_table_privilege(c.oid,'SELECT') AND NOT has_table_privilege(c.oid,'INSERT,UPDATE,DELETE,TRUNCATE') ORDER BY a.attnum LIMIT 129`, request.Schema, request.Table)
	if err != nil {
		return result, fmt.Errorf("falha ao validar tabela")
	}
	for rows.Next() {
		var name, kind string
		if err = rows.Scan(&name, &kind); err != nil {
			rows.Close()
			return result, err
		}
		columns[name] = kind
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if len(columns) == 0 || len(columns) > 128 {
		return result, fmt.Errorf("tabela indisponível ou papel com permissão de escrita")
	}
	for _, name := range append(append([]string{}, request.ExpectedNonNull...), request.CandidateKeys...) {
		if _, ok := columns[name]; !ok {
			return result, fmt.Errorf("coluna fora do escopo autorizado")
		}
	}
	for _, date := range request.DateRanges {
		kind, ok := columns[date.Column]
		if !ok || (!strings.HasPrefix(kind, "date") && !strings.HasPrefix(kind, "timestamp")) {
			return result, fmt.Errorf("coluna de data fora do escopo")
		}
	}
	table := pgx.Identifier{request.Schema, request.Table}.Sanitize()
	var estimatedRows int64
	if err = tx.QueryRow(ctx, `SELECT GREATEST(c.reltuples::bigint,0) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname=$2`, request.Schema, request.Table).Scan(&estimatedRows); err != nil {
		return result, fmt.Errorf("não foi possível estimar o tamanho da tabela")
	}
	sampleSource := table
	result.SampleMethod = "limite_sem_aleatoriedade"
	if estimatedRows > int64(request.Limit*2) {
		percent := math.Min(100, math.Max(0.1, float64(request.Limit*4)/float64(estimatedRows)*100))
		sampleSource += ` TABLESAMPLE SYSTEM (` + strconv.FormatFloat(percent, 'f', 3, 64) + `) REPEATABLE (1)`
		result.SampleMethod = "paginas_aleatorias_sistema"
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM `+sampleSource+` LIMIT $1) sample`, request.Limit).Scan(&result.SampledRows); err != nil {
		return result, fmt.Errorf("falha ao contar amostra: %s", config.SanitizeError(err))
	}
	if result.SampledRows == 0 && estimatedRows > 0 {
		return result, fmt.Errorf("amostra vazia; aumente o limite ou repita o diagnóstico")
	}
	for _, name := range request.ExpectedNonNull {
		column := pgx.Identifier{name}.Sanitize()
		var count int
		query := `SELECT count(*) FROM (SELECT ` + column + ` FROM ` + sampleSource + ` LIMIT $1) sample WHERE ` + column + ` IS NULL`
		if err = tx.QueryRow(ctx, query, request.Limit).Scan(&count); err != nil {
			return result, fmt.Errorf("falha na verificação de nulos: %s", config.SanitizeError(err))
		}
		result.Issues = append(result.Issues, Issue{Kind: "null", Column: name, AffectedRows: count, SampledRows: result.SampledRows})
	}
	for _, name := range request.CandidateKeys {
		column := pgx.Identifier{name}.Sanitize()
		var nonnull, distinct, maximum int
		query := `WITH sample AS MATERIALIZED (SELECT ` + column + ` FROM ` + sampleSource + ` LIMIT $1), frequencies AS (SELECT ` + column + `::text AS value,count(*)::int AS n FROM sample WHERE ` + column + ` IS NOT NULL GROUP BY 1) SELECT (SELECT count(*) FROM sample WHERE ` + column + ` IS NOT NULL),(SELECT count(*) FROM frequencies),COALESCE((SELECT max(n) FROM frequencies),0)`
		if err = tx.QueryRow(ctx, query, request.Limit).Scan(&nonnull, &distinct, &maximum); err != nil {
			return result, fmt.Errorf("falha na verificação de duplicidade: %s", config.SanitizeError(err))
		}
		result.Issues = append(result.Issues, Issue{Kind: "duplicate", Column: name, AffectedRows: nonnull - distinct, SampledRows: result.SampledRows})
		if result.SampledRows >= 30 && maximum*100 >= 80*nonnull && nonnull > 0 {
			result.Issues = append(result.Issues, Issue{Kind: "distribution", Column: name, AffectedRows: maximum, SampledRows: result.SampledRows})
		}
	}
	for _, date := range request.DateRanges {
		column := pgx.Identifier{date.Column}.Sanitize()
		var count int
		query := `SELECT count(*) FROM (SELECT ` + column + ` FROM ` + sampleSource + ` LIMIT $1) sample WHERE ` + column + ` IS NOT NULL AND (` + column + ` < $2::timestamptz OR ` + column + ` > $3::timestamptz)`
		if err = tx.QueryRow(ctx, query, request.Limit, date.From, date.To).Scan(&count); err != nil {
			return result, fmt.Errorf("falha na verificação de datas: %s", config.SanitizeError(err))
		}
		result.Issues = append(result.Issues, Issue{Kind: "date_range", Column: date.Column, AffectedRows: count, SampledRows: result.SampledRows})
	}
	foreign, err := tx.Query(ctx, `SELECT child.attname,parent_ns.nspname,parent_table.relname,parent.attname
FROM pg_constraint fk JOIN pg_class child_table ON child_table.oid=fk.conrelid
JOIN pg_namespace child_ns ON child_ns.oid=child_table.relnamespace
JOIN pg_attribute child ON child.attrelid=child_table.oid AND child.attnum=fk.conkey[1]
JOIN pg_class parent_table ON parent_table.oid=fk.confrelid
JOIN pg_namespace parent_ns ON parent_ns.oid=parent_table.relnamespace
JOIN pg_attribute parent ON parent.attrelid=parent_table.oid AND parent.attnum=fk.confkey[1]
WHERE fk.contype='f' AND array_length(fk.conkey,1)=1 AND array_length(fk.confkey,1)=1
AND child_ns.nspname=$1 AND child_table.relname=$2 AND has_table_privilege(parent_table.oid,'SELECT')
ORDER BY fk.conname LIMIT 9`, request.Schema, request.Table)
	if err != nil {
		return result, fmt.Errorf("falha ao consultar relações")
	}
	type relation struct{ child, schema, table, parent string }
	var relations []relation
	for foreign.Next() {
		var item relation
		if err = foreign.Scan(&item.child, &item.schema, &item.table, &item.parent); err != nil {
			foreign.Close()
			return result, err
		}
		relations = append(relations, item)
	}
	err = foreign.Err()
	foreign.Close()
	if err != nil {
		return result, err
	}
	if len(relations) > 8 {
		return result, fmt.Errorf("mais de oito relações; limite o escopo")
	}
	for _, relation := range relations {
		child := pgx.Identifier{relation.child}.Sanitize()
		parent := pgx.Identifier{relation.parent}.Sanitize()
		parentTable := pgx.Identifier{relation.schema, relation.table}.Sanitize()
		query := `WITH sample AS MATERIALIZED (SELECT ` + child + ` FROM ` + sampleSource + ` LIMIT $1) SELECT count(*) FROM sample s WHERE s.` + child + ` IS NOT NULL AND NOT EXISTS(SELECT 1 FROM ` + parentTable + ` p WHERE p.` + parent + `=s.` + child + `)`
		var count int
		if err = tx.QueryRow(ctx, query, request.Limit).Scan(&count); err != nil {
			return result, fmt.Errorf("falha na verificação de órfãos: %s", config.SanitizeError(err))
		}
		result.Issues = append(result.Issues, Issue{Kind: "orphan", Column: relation.child, AffectedRows: count, SampledRows: result.SampledRows})
	}
	return result, nil
}
