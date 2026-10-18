package repository

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/osmendes/db-auditor/internal/analyzer"
	"github.com/osmendes/db-auditor/internal/guidance"
)

// Global search (UI-07). It reads only the snapshot store, inside a READ ONLY
// transaction with a statement timeout; the audited databases are never
// queried. Every group is bounded and inventory is limited to the latest
// successful or partial run of each environment.

const (
	SearchMinChars      = 2
	SearchMaxChars      = 128
	SearchDefaultLimit  = 5
	SearchMaxLimit      = 20
	searchStatementTime = "2s"
)

var (
	ErrSearchQuery = errors.New("invalid search query")
	searchUUID     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	searchHexStart = regexp.MustCompile(`^[0-9a-fA-F]{4,}(-[0-9a-fA-F-]*)?$`)
)

// SearchHit is one result. Kind selects which fields are set:
// finding (id, severity, status, rule), run (id, status, at), rule
// (rule_id) and inventory (object_kind, database, schema, name, signature,
// audit_run_id).
type SearchHit struct {
	Kind          string     `json:"kind"`
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Subtitle      string     `json:"subtitle"`
	EnvironmentID string     `json:"environment_id,omitempty"`
	AuditRunID    string     `json:"audit_run_id,omitempty"`
	RuleID        string     `json:"rule_id,omitempty"`
	Severity      string     `json:"severity,omitempty"`
	Status        string     `json:"status,omitempty"`
	ObjectKind    string     `json:"object_kind,omitempty"`
	Database      string     `json:"database,omitempty"`
	Schema        string     `json:"schema,omitempty"`
	Name          string     `json:"name,omitempty"`
	Signature     *string    `json:"signature,omitempty"`
	At            *time.Time `json:"at,omitempty"`
	Exact         bool       `json:"exact"`
}

type SearchResult struct {
	Query     string          `json:"query"`
	Exact     bool            `json:"exact"`
	Groups    map[string]int  `json:"groups"`
	Truncated map[string]bool `json:"truncated"`
	Items     []SearchHit     `json:"items"`
	Limits    map[string]int  `json:"limits"`
}

// NormalizeSearchQuery trims and validates q.
func NormalizeSearchQuery(q string) (string, error) {
	q = strings.TrimSpace(q)
	n := utf8.RuneCountInString(q)
	if n < SearchMinChars || n > SearchMaxChars || strings.ContainsAny(q, "\x00\r\n") {
		return "", ErrSearchQuery
	}
	return q, nil
}

func searchLikePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}

// SearchRules matches rule id, category and Portuguese meaning.
func SearchRules(q string, limit int) ([]SearchHit, bool) {
	needle := strings.ToLower(q)
	out := []SearchHit{}
	for _, def := range analyzer.Catalog() {
		meaning := guidance.For(def.ID).Meaning
		if strings.Contains(strings.ToLower(def.ID), needle) || strings.Contains(strings.ToLower(meaning), needle) || strings.EqualFold(def.Category, needle) {
			out = append(out, SearchHit{Kind: "rule", ID: def.ID, RuleID: def.ID, Title: def.ID, Subtitle: meaning, Exact: strings.EqualFold(def.ID, q)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Exact != out[j].Exact {
			return out[i].Exact
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > limit {
		return out[:limit], true
	}
	return out, false
}

// Search runs the bounded global search. env == "" searches every
// environment (operators only; the API enforces it).
func (s *Store) Search(ctx context.Context, env, query string, limit int) (SearchResult, error) {
	q, err := NormalizeSearchQuery(query)
	if err != nil {
		return SearchResult{}, err
	}
	if limit < 1 || limit > SearchMaxLimit {
		limit = SearchDefaultLimit
	}
	res := SearchResult{Query: q, Groups: map[string]int{}, Truncated: map[string]bool{}, Items: []SearchHit{},
		Limits: map[string]int{"per_group": limit, "min_chars": SearchMinChars, "max_chars": SearchMaxChars}}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return res, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SET LOCAL statement_timeout = '`+searchStatementTime+`'`); err != nil {
		return res, err
	}
	add := func(group string, hits []SearchHit, truncated bool) {
		res.Items = append(res.Items, hits...)
		res.Groups[group] = len(hits)
		res.Truncated[group] = truncated
	}
	if searchUUID.MatchString(q) {
		id := strings.ToLower(q)
		findings, err := s.searchFindings(ctx, tx, env, `f.id=$2::uuid`, id, 1)
		if err != nil {
			return res, err
		}
		runs, err := s.searchRuns(ctx, tx, env, `r.id=$2::uuid AND $3=''`, id, 1, "")
		if err != nil {
			return res, err
		}
		for i := range findings {
			findings[i].Exact = true
		}
		for i := range runs {
			runs[i].Exact = true
		}
		add("findings", findings, false)
		add("runs", runs, false)
		res.Exact = len(findings)+len(runs) > 0
		return res, tx.Commit(ctx)
	}
	pattern := searchLikePattern(q)
	findings, err := s.searchFindings(ctx, tx, env, `(f.title ILIKE $2 OR f.finding_type ILIKE $2 OR f.object_name ILIKE $2 OR f.object_key ILIKE $2 OR f.schema_name ILIKE $2 OR f.database_name ILIKE $2)`, pattern, limit+1)
	if err != nil {
		return res, err
	}
	add("findings", trimHits(findings, limit), len(findings) > limit)
	prefix := ""
	if searchHexStart.MatchString(q) {
		prefix = strings.ToLower(q) + "%"
	}
	runs, err := s.searchRuns(ctx, tx, env, `(r.id::text LIKE $3 OR r.profile ILIKE $2 OR r.status ILIKE $2 OR r.service_version ILIKE $2 OR e.name ILIKE $2)`, pattern, limit+1, prefix)
	if err != nil {
		return res, err
	}
	add("runs", trimHits(runs, limit), len(runs) > limit)
	inventory, err := s.searchInventory(ctx, tx, env, pattern, q, limit+1)
	if err != nil {
		return res, err
	}
	add("inventory", trimHits(inventory, limit), len(inventory) > limit)
	rules, more := SearchRules(q, limit)
	res.Items = append(res.Items, rules...)
	res.Groups["rules"], res.Truncated["rules"] = len(rules), more
	return res, tx.Commit(ctx)
}

func trimHits(hits []SearchHit, limit int) []SearchHit {
	if len(hits) > limit {
		return hits[:limit]
	}
	return hits
}

func (s *Store) searchFindings(ctx context.Context, tx pgx.Tx, env, filter, arg string, limit int) ([]SearchHit, error) {
	rows, err := tx.Query(ctx, `SELECT f.id::text,f.environment_id::text,COALESCE(f.audit_run_id::text,''),f.finding_type,f.severity,f.status,left(f.title,256),
left(concat_ws('.',NULLIF(f.database_name,''),NULLIF(f.schema_name,''),NULLIF(f.object_name,'')),384),f.last_seen_at
FROM finding f WHERE ($1='' OR f.environment_id=$1::uuid) AND `+filter+`
ORDER BY CASE f.status WHEN 'open' THEN 0 WHEN 'acknowledged' THEN 1 ELSE 2 END,
 CASE f.severity WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END,f.last_seen_at DESC,f.id
LIMIT $3`, env, arg, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SearchHit{}
	for rows.Next() {
		var h SearchHit
		var at time.Time
		if err = rows.Scan(&h.ID, &h.EnvironmentID, &h.AuditRunID, &h.RuleID, &h.Severity, &h.Status, &h.Title, &h.Subtitle, &at); err != nil {
			return nil, err
		}
		h.Kind, h.At = "finding", &at
		if h.Subtitle == "" {
			h.Subtitle = h.RuleID
		} else {
			h.Subtitle = h.RuleID + " · " + h.Subtitle
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) searchRuns(ctx context.Context, tx pgx.Tx, env, filter, arg string, limit int, prefix string) ([]SearchHit, error) {
	rows, err := tx.Query(ctx, `SELECT r.id::text,r.environment_id::text,r.status,r.profile,e.name,r.started_at
FROM audit_run r JOIN audit_environment e ON e.id=r.environment_id
WHERE ($1='' OR r.environment_id=$1::uuid) AND `+filter+`
ORDER BY r.started_at DESC,r.id LIMIT $4`, env, arg, prefix, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SearchHit{}
	for rows.Next() {
		var h SearchHit
		var profile, envName string
		var at time.Time
		if err = rows.Scan(&h.ID, &h.EnvironmentID, &h.Status, &profile, &envName, &at); err != nil {
			return nil, err
		}
		h.Kind, h.At, h.AuditRunID = "run", &at, h.ID
		h.Title = "Execução " + h.ID[:8]
		h.Subtitle = envName + " · " + profile + " · " + h.Status + " · " + at.UTC().Format("02/01/2006 15:04 UTC")
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) searchInventory(ctx context.Context, tx pgx.Tx, env, pattern, raw string, limit int) ([]SearchHit, error) {
	rows, err := tx.Query(ctx, `WITH latest AS (
  SELECT DISTINCT ON (environment_id) id, environment_id FROM audit_run
  WHERE status IN ('success','partial_success') AND ($1='' OR environment_id=$1::uuid)
  ORDER BY environment_id, started_at DESC, id
), hits AS (
  (SELECT 'tables' kind, t.environment_id, t.audit_run_id, t.database_name, t.schema_name, t.table_name AS name, NULL::text AS signature
   FROM table_snapshot t JOIN latest l ON l.id=t.audit_run_id WHERE t.table_name ILIKE $2 ORDER BY t.table_name LIMIT $3)
  UNION ALL
  (SELECT 'indexes', i.environment_id, i.audit_run_id, i.database_name, i.schema_name, i.index_name, NULL
   FROM index_snapshot i JOIN latest l ON l.id=i.audit_run_id WHERE i.index_name ILIKE $2 ORDER BY i.index_name LIMIT $3)
  UNION ALL
  (SELECT 'views', v.environment_id, v.audit_run_id, v.database_name, v.schema_name, v.view_name, NULL
   FROM view_snapshot v JOIN latest l ON l.id=v.audit_run_id WHERE v.view_name ILIKE $2 ORDER BY v.view_name LIMIT $3)
  UNION ALL
  (SELECT 'functions', fn.environment_id, fn.audit_run_id, fn.database_name, fn.schema_name, fn.function_name, fn.identity_arguments
   FROM function_snapshot fn JOIN latest l ON l.id=fn.audit_run_id WHERE fn.function_name ILIKE $2 ORDER BY fn.function_name LIMIT $3)
)
SELECT kind, environment_id::text, audit_run_id::text, database_name, schema_name, name, signature FROM hits
ORDER BY lower(name)=lower($4) DESC, length(name), name, kind, database_name, schema_name LIMIT $3`, env, pattern, limit, raw)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SearchHit{}
	labels := map[string]string{"tables": "Tabela", "indexes": "Índice", "views": "View", "functions": "Função"}
	for rows.Next() {
		var h SearchHit
		if err = rows.Scan(&h.ObjectKind, &h.EnvironmentID, &h.AuditRunID, &h.Database, &h.Schema, &h.Name, &h.Signature); err != nil {
			return nil, err
		}
		h.Kind = "inventory"
		h.ID = h.ObjectKind + ":" + h.EnvironmentID + ":" + h.Database + "." + h.Schema + "." + h.Name
		h.Title = h.Schema + "." + h.Name
		if h.Signature != nil {
			h.Title += "(" + *h.Signature + ")"
			h.ID += "(" + *h.Signature + ")"
		}
		h.Subtitle = labels[h.ObjectKind] + " · banco " + h.Database
		out = append(out, h)
	}
	return out, rows.Err()
}
