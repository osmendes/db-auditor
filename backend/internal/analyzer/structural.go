package analyzer

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"
)

// StructuralAnalyzer emits conservative hypotheses from one immutable run.
// Findings never contain executable DDL or assert unknown business intent.
type StructuralAnalyzer struct{}

func (StructuralAnalyzer) Name() string { return "structural" }

var numberedColumn = regexp.MustCompile(`^(.*)_([1-9][0-9]*)$`)
var snakeIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func tableKey(db, schema, table string) string { return db + "." + schema + "." + table }

func structuralFinding(f SnapshotFacts, kind, db, schema, objectType, objectName, summary string, severity Severity, evidence map[string]any) Finding {
	key := tableKey(db, schema, objectName)
	title := kind + ": " + key
	return Finding{
		EnvironmentID: f.EnvironmentID, AuditRunID: f.AuditRunID,
		FindingType: kind, Severity: severity, Status: StatusOpen,
		Title: title, Summary: summary, ObjectType: objectType, ObjectKey: key,
		DatabaseName: db, SchemaName: schema, ObjectName: objectName,
		Evidence: evidence, DedupKey: DedupKey(kind, key, title),
	}
}

func (StructuralAnalyzer) Analyze(_ context.Context, f SnapshotFacts) ([]Finding, error) {
	out := make([]Finding, 0)
	tables := map[string]TableFact{}
	columns := map[string][]ColumnFact{}
	columnByName := map[string]ColumnFact{}
	indexes := map[string][]IndexFact{}
	fkColumns := map[string]bool{}
	for _, t := range f.Tables {
		tables[tableKey(t.Database, t.Schema, t.Name)] = t
	}
	for _, c := range f.Columns {
		key := tableKey(c.Database, c.Schema, c.TableName)
		columns[key] = append(columns[key], c)
		columnByName[key+"."+c.Name] = c
	}
	for _, i := range f.Indexes {
		indexes[tableKey(i.Database, i.Schema, i.TableName)] = append(indexes[tableKey(i.Database, i.Schema, i.TableName)], i)
	}
	for _, c := range f.Constraints {
		key := tableKey(c.Database, c.Schema, c.TableName)
		if !c.Validated && (c.Type == "f" || c.Type == "c") {
			out = append(out, structuralFinding(f, "integrity.constraint_unvalidated", c.Database, c.Schema, "constraint", c.TableName+"."+c.Name,
				"Constraint is not validated; review data and validation plan.", SeverityMedium,
				map[string]any{"table": c.TableName, "constraint_type": c.Type, "validated": false}))
		}
		if c.Type != "f" {
			continue
		}
		for _, name := range c.Columns {
			fkColumns[key+"."+name] = true
		}
		if len(c.Columns) > 0 && !hasLeadingIndex(indexes[key], c.Columns) {
			out = append(out, structuralFinding(f, "integrity.fk_without_index", c.Database, c.Schema, "constraint", c.TableName+"."+c.Name,
				"No valid index starts with the referencing FK columns; investigate workload before adding one.", SeverityMedium,
				map[string]any{"table": c.TableName, "columns": c.Columns, "referenced_schema": c.ReferencedSchema, "referenced_table": c.ReferencedTable}))
		}
		refSchema := c.ReferencedSchema
		if refSchema == "" {
			refSchema = c.Schema
		}
		if len(c.Columns) == len(c.ReferencedColumns) && c.ReferencedTable != "" {
			refKey := tableKey(c.Database, refSchema, c.ReferencedTable)
			for j, name := range c.Columns {
				left, okL := columnByName[key+"."+name]
				right, okR := columnByName[refKey+"."+c.ReferencedColumns[j]]
				if okL && okR && typeFamily(left.DataType) != typeFamily(right.DataType) {
					out = append(out, structuralFinding(f, "integrity.fk_type_mismatch", c.Database, c.Schema, "constraint", c.TableName+"."+c.Name+"."+name,
						"Related columns use different type families; verify casts and intended semantics.", SeverityLow,
						map[string]any{"column": name, "column_type": left.DataType, "referenced_column": right.Name, "referenced_type": right.DataType}))
				}
			}
		}
	}
	for _, t := range f.Tables {
		key := tableKey(t.Database, t.Schema, t.Name)
		if !t.HasPrimaryKey && !t.IsPartition && t.RelationClass != "foreign_table" {
			out = append(out, structuralFinding(f, "integrity.missing_primary_key", t.Database, t.Schema, "table", t.Name,
				"Table has no primary key in this snapshot; confirm identity and write patterns before changing schema.", SeverityMedium,
				map[string]any{"row_estimate": t.RowEstimate, "relation_class": t.RelationClass}))
		}
		wideLimit := int(threshold(f, t.Schema, "model.wide_table", "min_columns", 50))
		if t.ColumnCount >= wideLimit {
			out = append(out, structuralFinding(f, "model.wide_table", t.Database, t.Schema, "table", t.Name,
				"Wide table may indicate mixed responsibilities; this is a modelling hypothesis only.", SeverityLow,
				map[string]any{"column_count": t.ColumnCount, "threshold": wideLimit}))
		}
		if t.Comment == "" && (float64(t.SizeBytes) >= threshold(f, t.Schema, "model.undocumented_critical", "min_bytes", 1<<30) || t.RowEstimate >= 1_000_000) {
			out = append(out, structuralFinding(f, "model.undocumented_critical", t.Database, t.Schema, "table", t.Name,
				"Large table has no catalog comment; document ownership and purpose.", SeverityLow,
				map[string]any{"size_bytes": t.SizeBytes, "row_estimate": t.RowEstimate}))
		}
		if !identifierConsistent(t.Name, namingConvention(f, t.Schema)) {
			out = append(out, structuralFinding(f, "model.naming_inconsistent", t.Database, t.Schema, "table", t.Name,
				"Identifier differs from the configured snake_case convention; review consistency, not correctness.", SeverityInfo,
				map[string]any{"convention": namingConvention(f, t.Schema)}))
		}
		if float64(len(indexes[key])) >= threshold(f, t.Schema, "index.write_burden", "min_indexes", 8) && t.StatsReset != nil && t.CollectedAt.Sub(*t.StatsReset) >= 30*24*time.Hour && float64(t.NTupIns+t.NTupUpd+t.NTupDel) >= threshold(f, t.Schema, "index.write_burden", "min_writes", 100_000) {
			out = append(out, structuralFinding(f, "index.write_burden", t.Database, t.Schema, "table", t.Name,
				"Many indexes coincide with substantial writes; review maintenance cost without automatic removal.", SeverityLow,
				map[string]any{"index_count": len(indexes[key]), "writes_since_reset": t.NTupIns + t.NTupUpd + t.NTupDel, "stats_reset": t.StatsReset}))
		}
		if t.LastAnalyze != nil && !t.CollectedAt.IsZero() && t.CollectedAt.Sub(*t.LastAnalyze) > time.Duration(threshold(f, t.Schema, "maintenance.stale_analyze", "max_age_days", 30))*24*time.Hour && t.RowEstimate >= 10_000 {
			out = append(out, structuralFinding(f, "maintenance.stale_analyze", t.Database, t.Schema, "table", t.Name,
				"Planner statistics have not been analyzed recently; verify autovacuum activity.", SeverityLow,
				map[string]any{"last_analyze": t.LastAnalyze, "row_estimate": t.RowEstimate, "threshold_days": 30}))
		}
		grouped := map[string]int{}
		for _, c := range columns[key] {
			if m := numberedColumn.FindStringSubmatch(c.Name); len(m) > 0 && c.Nullable {
				grouped[m[1]]++
			}
			money := strings.EqualFold(c.DataType, "money") && ruleFlag(f, c.Schema, "model.type_review", "check_money", true)
			temporal := strings.Contains(strings.ToLower(c.DataType), "timestamp") && !strings.Contains(strings.ToLower(c.DataType), "time zone") && ruleFlag(f, c.Schema, "model.type_review", "check_timestamp_without_timezone", true)
			status := (strings.HasSuffix(c.Name, "_status") || strings.HasSuffix(c.Name, "_code")) && strings.Contains(strings.ToLower(c.DataType), "character") && ruleFlag(f, c.Schema, "model.type_review", "check_status_text", true)
			if money || temporal || status {
				out = append(out, structuralFinding(f, "model.type_review", c.Database, c.Schema, "column", c.TableName+"."+c.Name,
					"Column type or status encoding warrants domain review; no replacement is assumed.", SeverityLow,
					map[string]any{"data_type": c.DataType, "column": c.Name}))
			}
			if strings.EqualFold(c.DataType, "jsonb") && t.RowEstimate >= 100_000 {
				out = append(out, structuralFinding(f, "model.jsonb_critical", c.Database, c.Schema, "column", c.TableName+"."+c.Name,
					"JSONB on a large table may hide frequently queried structure; verify access patterns.", SeverityLow,
					map[string]any{"row_estimate": t.RowEstimate, "column": c.Name}))
			}
			if strings.HasSuffix(c.Name, "_id") && !fkColumns[key+"."+c.Name] && candidateTable(f.Tables, c.Database, c.Schema, strings.TrimSuffix(c.Name, "_id")) {
				out = append(out, structuralFinding(f, "model.implicit_relationship", c.Database, c.Schema, "column", c.TableName+"."+c.Name,
					"Name suggests a relationship but no FK is recorded; confirm business intent before adding one.", SeverityInfo,
					map[string]any{"column": c.Name, "confidence_note": "naming-only hypothesis"}))
			}
			if !identifierConsistent(c.Name, namingConvention(f, c.Schema)) {
				out = append(out, structuralFinding(f, "model.naming_inconsistent", c.Database, c.Schema, "column", c.TableName+"."+c.Name,
					"Column identifier differs from the configured convention.", SeverityInfo, map[string]any{"convention": namingConvention(f, c.Schema)}))
			}
		}
		for prefix, count := range grouped {
			if count >= 2 {
				out = append(out, structuralFinding(f, "model.repeated_columns", t.Database, t.Schema, "table", t.Name+"."+prefix,
					"Numbered optional columns may represent a repeated group; investigate normalization.", SeverityInfo,
					map[string]any{"prefix": prefix, "count": count}))
			}
		}
	}
	for _, s := range f.Sequences {
		for _, c := range f.Columns {
			if c.Database != s.Database || !defaultReferencesSequence(c.Default, s) || s.OwnedByTable == "" {
				continue
			}
			if s.OwnedByTable != c.Schema+"."+c.TableName || s.OwnedByColumn != c.Name {
				out = append(out, structuralFinding(f, "integrity.sequence_default_mismatch", c.Database, c.Schema, "column", c.TableName+"."+c.Name,
					"Column default references a sequence owned by another column; verify ownership and intended sharing.", SeverityLow,
					map[string]any{"sequence": s.Schema + "." + s.Name, "owned_by_table": s.OwnedByTable, "owned_by_column": s.OwnedByColumn}))
			}
		}
		if s.OwnedByTable != "" {
			continue
		}
		referenced := false
		for _, c := range f.Columns {
			if c.Database == s.Database && strings.Contains(c.Default, s.Name) && strings.Contains(c.Default, "nextval(") {
				referenced = true
				break
			}
		}
		if !referenced {
			out = append(out, structuralFinding(f, "integrity.orphan_sequence", s.Database, s.Schema, "sequence", s.Name,
				"Sequence has no ownership or observed nextval default; verify consumers before changing it.", SeverityLow, map[string]any{"owned_by_table": s.OwnedByTable}))
		}
	}
	for _, v := range f.Vacuum {
		if float64(v.NDeadTup) < threshold(f, v.Schema, "maintenance.dead_tuple_pressure", "min_dead_tuples", 10_000) || v.NLiveTup <= 0 || float64(v.NDeadTup)/float64(v.NLiveTup) < threshold(f, v.Schema, "maintenance.dead_tuple_pressure", "min_dead_ratio", 0.2) {
			continue
		}
		out = append(out, structuralFinding(f, "maintenance.dead_tuple_pressure", v.Database, v.Schema, "table", v.Name,
			"Dead tuple estimate is high relative to live tuples; investigate vacuum and bloat with exact diagnostics.", SeverityLow,
			map[string]any{"n_dead_tup": v.NDeadTup, "n_live_tup": v.NLiveTup, "estimate_only": true,
				"impact_basis": map[string]any{"last_vacuum": v.LastVacuum, "last_autovacuum": v.LastAutovacuum}}))
	}
	workloadSeen := map[string]bool{}
	for _, q := range f.QueryStats {
		if q.EvidenceQuality != "object_reference" || len(q.ReferencedObjects) != 1 || q.QueryKind != "select" {
			continue
		}
		parts := strings.SplitN(q.ReferencedObjects[0], ".", 2)
		if len(parts) != 2 {
			continue
		}
		if float64(q.Calls) < threshold(f, parts[0], "index.investigate_missing", "min_calls", 100) || float64(q.SharedBlocksRead) < threshold(f, parts[0], "index.investigate_missing", "min_shared_reads", 1000) {
			continue
		}
		key := tableKey(q.Database, parts[0], parts[1])
		if _, ok := tables[key]; !ok || len(indexes[key]) > 0 || workloadSeen[key] {
			continue
		}
		workloadSeen[key] = true
		out = append(out, structuralFinding(f, "index.investigate_missing", q.Database, parts[0], "table", parts[1],
			"Repeated reads on a table without indexes warrant plan review; no index columns are inferred.", SeverityLow,
			map[string]any{"query_fingerprint": q.QueryFingerprint, "calls": q.Calls, "shared_blocks_read": q.SharedBlocksRead, "evidence_quality": q.EvidenceQuality}))
	}
	for _, group := range indexes {
		out = append(out, indexStructureFindings(f, group)...)
	}
	out = append(out, duplicateEntityFindings(f, columns)...)
	return out, nil
}

func hasLeadingIndex(indexes []IndexFact, cols []string) bool {
	for _, idx := range indexes {
		if idx.HasValidity && (!idx.IsValid || !idx.IsReady) {
			continue
		}
		if idx.Predicate != "" || len(idx.KeyColumns) < len(cols) || (idx.HasValidity && accessMethod(idx.Definition) != "btree") {
			continue
		}
		match := true
		for i, col := range cols {
			if idx.KeyColumns[i] != col {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func defaultReferencesSequence(def string, s SequenceFact) bool {
	if !strings.Contains(def, "nextval(") {
		return false
	}
	return strings.Contains(def, "'"+s.Schema+"."+s.Name+"'::regclass") ||
		strings.Contains(def, "'"+s.Name+"'::regclass")
}

func typeFamily(raw string) string {
	s := strings.ToLower(raw)
	switch {
	case strings.Contains(s, "int"), strings.Contains(s, "numeric"), strings.Contains(s, "decimal"):
		return "number"
	case strings.Contains(s, "char"), s == "text":
		return "text"
	case strings.Contains(s, "uuid"):
		return "uuid"
	case strings.Contains(s, "date"), strings.Contains(s, "time"):
		return "time"
	default:
		return s
	}
}

func candidateTable(tables []TableFact, db, schema, base string) bool {
	for _, t := range tables {
		if t.Database == db && t.Schema == schema && (t.Name == base || t.Name == base+"s") {
			return true
		}
	}
	return false
}

func identifierConsistent(name, convention string) bool {
	switch convention {
	case "none":
		return true
	case "lowercase":
		return name == strings.ToLower(name)
	default:
		return snakeIdentifier.MatchString(name)
	}
}

func indexStructureFindings(f SnapshotFacts, group []IndexFact) []Finding {
	out := []Finding{}
	for i, a := range group {
		if a.HasValidity && (!a.IsValid || !a.IsReady) {
			out = append(out, structuralFinding(f, "index.invalid", a.Database, a.Schema, "index", a.IndexName,
				"Index is invalid or not ready; investigate concurrent-build or maintenance history.", SeverityMedium,
				map[string]any{"is_valid": a.IsValid, "is_ready": a.IsReady, "table": a.TableName}))
		}
		if a.IsPrimary || a.IsUnique || len(a.KeyColumns) == 0 || a.Predicate != "" {
			continue
		}
		for _, b := range group[i+1:] {
			if b.IsPrimary || b.IsUnique || b.Predicate != "" || a.Definition == b.Definition || a.HasValidity && !a.IsValid || b.HasValidity && !b.IsValid {
				continue
			}
			if a.Database != b.Database || a.Schema != b.Schema || a.TableName != b.TableName {
				continue
			}
			short, long := a, b
			if len(short.KeyColumns) > len(long.KeyColumns) {
				short, long = long, short
			}
			if len(short.KeyColumns) == len(long.KeyColumns) || !strings.EqualFold(accessMethod(short.Definition), accessMethod(long.Definition)) {
				continue
			}
			match := true
			for k, col := range short.KeyColumns {
				if col != long.KeyColumns[k] {
					match = false
					break
				}
			}
			if match {
				out = append(out, structuralFinding(f, "index.prefix_overlap", short.Database, short.Schema, "index", short.IndexName,
					"One non-unique index is a key prefix of another; inspect predicates, INCLUDE columns and plans before considering changes.", SeverityLow,
					map[string]any{"table": short.TableName, "short_index": short.IndexName, "long_index": long.IndexName, "prefix_columns": short.KeyColumns}))
			}
		}
	}
	return out
}

func accessMethod(def string) string {
	parts := strings.SplitN(strings.ToLower(def), " using ", 2)
	if len(parts) < 2 {
		return ""
	}
	fields := strings.Fields(parts[1])
	if len(fields) == 0 {
		return ""
	}
	return strings.SplitN(fields[0], "(", 2)[0]
}

func duplicateEntityFindings(f SnapshotFacts, cols map[string][]ColumnFact) []Finding {
	type entry struct {
		t         TableFact
		signature string
	}
	byName := map[string][]entry{}
	for _, t := range f.Tables {
		list := cols[tableKey(t.Database, t.Schema, t.Name)]
		if len(list) < 3 {
			continue
		}
		sig := make([]string, 0, len(list))
		for _, c := range list {
			sig = append(sig, c.Name+":"+typeFamily(c.DataType))
		}
		sort.Strings(sig)
		byName[t.Database+"."+t.Name] = append(byName[t.Database+"."+t.Name], entry{t, strings.Join(sig, "|")})
	}
	out := []Finding{}
	for _, group := range byName {
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				a, b := group[i], group[j]
				if a.t.Schema == b.t.Schema || a.signature != b.signature {
					continue
				}
				out = append(out, structuralFinding(f, "model.duplicate_entity", b.t.Database, b.t.Schema, "table", b.t.Name,
					"Same table name and column signature appears in another schema; verify ownership before consolidation.", SeverityInfo,
					map[string]any{"other_schema": a.t.Schema, "column_count": len(cols[tableKey(b.t.Database, b.t.Schema, b.t.Name)])}))
			}
		}
	}
	return out
}
