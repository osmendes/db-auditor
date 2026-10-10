package analyzer

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// PreviousIndexes and sequence/function extras are optional facts. A missing
// previous window is not a finding.

func unusedIndexFindings(facts SnapshotFacts) []Finding {
	prev := map[string]IndexFact{}
	for _, idx := range facts.PreviousIndexes {
		prev[indexKey(idx)] = idx
	}
	out := make([]Finding, 0)
	for _, idx := range facts.Indexes {
		if idx.IsPrimary || idx.IsUnique {
			continue
		}
		key := indexKey(idx)
		earlier, ok := prev[key]
		if !ok {
			continue
		}
		if idx.StatsReset != nil && !earlier.CollectedAt.IsZero() && idx.StatsReset.After(earlier.CollectedAt) {
			continue
		}
		if idx.IdxScan != earlier.IdxScan {
			continue
		}
		title := fmt.Sprintf("Possibly unused index: %s", key)
		out = append(out, Finding{
			EnvironmentID: facts.EnvironmentID,
			AuditRunID:    facts.AuditRunID,
			FindingType:   "index.unused",
			Severity:      SeverityMedium,
			Status:        StatusOpen,
			Title:         title,
			Summary:       "idx_scan did not grow between two collections. Manual review required; no automatic DROP is suggested.",
			ObjectType:    "index",
			ObjectKey:     key,
			DatabaseName:  idx.Database,
			SchemaName:    idx.Schema,
			ObjectName:    idx.IndexName,
			Evidence: map[string]any{
				"table_name":        idx.TableName,
				"current_idx_scan":  idx.IdxScan,
				"previous_idx_scan": earlier.IdxScan,
				"note":              "Never recommend DROP automatically",
			},
			DedupKey: DedupKey("index.unused", key, title),
		})
	}
	return out
}

func indexKey(idx IndexFact) string {
	return fmt.Sprintf("%s.%s.%s", idx.Database, idx.Schema, idx.IndexName)
}

var fingerprintIdent = regexp.MustCompile(`[a-z_][a-z0-9_]*`)

// CandidateColumns returns identifiers from a sanitized fingerprint that are
// not the leading column of an existing index. It never emits SQL.
func CandidateColumns(fingerprint string, existing []IndexFact) []string {
	lead := map[string]bool{}
	for _, idx := range existing {
		if len(idx.KeyColumns) > 0 {
			lead[strings.ToLower(idx.KeyColumns[0])] = true
		}
	}
	skip := map[string]bool{
		"select": true, "from": true, "where": true, "and": true, "or": true,
		"order": true, "by": true, "group": true, "limit": true, "join": true,
		"on": true, "as": true, "inner": true, "left": true, "right": true,
	}
	seen := map[string]bool{}
	out := []string{}
	for _, ident := range fingerprintIdent.FindAllString(strings.ToLower(fingerprint), -1) {
		if skip[ident] || lead[ident] || seen[ident] {
			continue
		}
		seen[ident] = true
		out = append(out, ident)
	}
	return out
}

func candidateFindings(facts SnapshotFacts) []Finding {
	out := make([]Finding, 0)
	for _, fp := range facts.Fingerprints {
		cols := CandidateColumns(fp.Text, facts.Indexes)
		if len(cols) == 0 || fp.Hash == "" {
			continue
		}
		key := fp.Hash
		title := "Index candidate hypothesis " + key
		out = append(out, Finding{
			EnvironmentID: facts.EnvironmentID,
			AuditRunID:    facts.AuditRunID,
			FindingType:   "index.candidate",
			Severity:      SeverityLow,
			Status:        StatusOpen,
			Confidence:    0.4,
			Title:         title,
			Summary:       "Hypothesis only: these columns appear in a sanitized fingerprint and are not the leading column of an existing index.",
			ObjectType:    "index",
			ObjectKey:     key,
			Evidence: map[string]any{
				"fingerprint": fp.Hash,
				"columns":     cols,
			},
			DedupKey: DedupKey("index.candidate", key, title),
		})
	}
	return out
}

func sequenceNearLimit(facts SnapshotFacts) []Finding {
	ratio := threshold(facts, "", "sequence.near_limit", "sequence_near_limit_ratio", 0.70)
	high := threshold(facts, "", "sequence.near_limit", "sequence_integer_high_ratio", 0.80)
	out := make([]Finding, 0)
	for _, seq := range facts.Sequences {
		if seq.LastValue == nil || seq.MaxValue == nil || *seq.MaxValue <= 1000 || *seq.MaxValue <= 0 {
			continue
		}
		used := float64(*seq.LastValue) / float64(*seq.MaxValue)
		if used < ratio {
			continue
		}
		severity := SeverityMedium
		if used >= high && (seq.DataType == "integer" || seq.DataType == "int4" || seq.DataType == "serial") {
			severity = SeverityHigh
		}
		owner := seq.OwnedByColumn
		if owner == "" {
			owner = seq.Name
		}
		key := fmt.Sprintf("%s.%s.%s", seq.Database, seq.Schema, seq.Name)
		title := fmt.Sprintf("Sequence near limit: %s", key)
		out = append(out, Finding{
			EnvironmentID: facts.EnvironmentID,
			AuditRunID:    facts.AuditRunID,
			FindingType:   "sequence.near_limit",
			Severity:      severity,
			Status:        StatusOpen,
			Title:         title,
			Summary:       fmt.Sprintf("Column %s is at %.0f%% of the sequence maximum. Review before the counter stops; no sequence change is applied.", owner, used*100),
			ObjectType:    "sequence",
			ObjectKey:     key,
			DatabaseName:  seq.Database,
			SchemaName:    seq.Schema,
			ObjectName:    seq.Name,
			Evidence: map[string]any{
				"last_value":      *seq.LastValue,
				"max_value":       *seq.MaxValue,
				"ratio":           used,
				"owned_by_table":  seq.OwnedByTable,
				"owned_by_column": seq.OwnedByColumn,
				"data_type":       seq.DataType,
			},
			DedupKey: DedupKey("sequence.near_limit", key, title),
		})
	}
	return out
}

// SearchPathPinned reports a function config that fixes search_path.
func SearchPathPinned(config string) bool {
	for _, part := range strings.Split(strings.ToLower(config), ",") {
		if strings.HasPrefix(strings.TrimSpace(part), "search_path=") {
			return true
		}
	}
	return false
}

func definerSearchPathFindings(facts SnapshotFacts) []Finding {
	out := make([]Finding, 0)
	for _, fn := range facts.Functions {
		if !fn.IsSecurityDefiner || fn.SearchPathPinned || SearchPathPinned(fn.Config) {
			continue
		}
		key := fmt.Sprintf("%s.%s.%s", fn.Database, fn.Schema, fn.FunctionName)
		if fn.IdentityArgs != "" {
			key += "(" + fn.IdentityArgs + ")"
		}
		title := "Security definer without pinned search_path: " + key
		out = append(out, Finding{
			EnvironmentID: facts.EnvironmentID,
			AuditRunID:    facts.AuditRunID,
			FindingType:   "security.definer_search_path",
			Severity:      SeverityHigh,
			Status:        StatusOpen,
			Title:         title,
			Summary:       "Security definer uses the default search_path. Confirm the function body; no privilege change is applied.",
			ObjectType:    "function",
			ObjectKey:     key,
			DatabaseName:  fn.Database,
			SchemaName:    fn.Schema,
			ObjectName:    fn.FunctionName,
			Evidence:      map[string]any{"identity_arguments": fn.IdentityArgs, "search_path_pinned": false},
			DedupKey:      DedupKey("security.definer_search_path", key, title),
		})
	}
	return out
}

func systemSchema(schema string) bool {
	return schema == "information_schema" || strings.HasPrefix(schema, "pg_") || strings.HasPrefix(schema, "_timescaledb")
}

func rlsHypothesis(facts SnapshotFacts) []Finding {
	out := make([]Finding, 0)
	for _, t := range facts.Tables {
		if t.RelRowSecurity || systemSchema(t.Schema) || t.IsPartition {
			continue
		}
		key := fmt.Sprintf("%s.%s.%s", t.Database, t.Schema, t.Name)
		title := "RLS disabled hypothesis: " + key
		out = append(out, Finding{
			EnvironmentID: facts.EnvironmentID,
			AuditRunID:    facts.AuditRunID,
			FindingType:   "security.rls_disabled_hypothesis",
			Severity:      SeverityLow,
			Status:        StatusOpen,
			Confidence:    0.4,
			Title:         title,
			Summary:       "Hypothesis: this user table has row security off. Confirm whether the application expects it; no table change is applied.",
			ObjectType:    "table",
			ObjectKey:     key,
			DatabaseName:  t.Database,
			SchemaName:    t.Schema,
			ObjectName:    t.Name,
			DedupKey:      DedupKey("security.rls_disabled_hypothesis", key, title),
		})
	}
	return out
}

// PublicCreateOnPublic is true when a schema ACL grants CREATE to PUBLIC.
func PublicCreateOnPublic(acls []string) bool {
	for _, acl := range acls {
		if strings.Contains(acl, "=C/") || strings.Contains(acl, "=UC/") {
			return true
		}
	}
	return false
}

func publicSchemaCreate(facts SnapshotFacts) []Finding {
	out := make([]Finding, 0)
	for _, schema := range facts.Schemas {
		if schema.Name != "public" || !PublicCreateOnPublic(schema.ACLs) {
			continue
		}
		key := schema.Database + ".public"
		title := "PUBLIC can create objects in schema public"
		out = append(out, Finding{
			EnvironmentID: facts.EnvironmentID,
			AuditRunID:    facts.AuditRunID,
			FindingType:   "security.public_schema_create",
			Severity:      SeverityMedium,
			Status:        StatusOpen,
			Title:         title,
			Summary:       "Schema public grants CREATE to PUBLIC. Review who should create objects; no privilege change is applied.",
			ObjectType:    "schema",
			ObjectKey:     key,
			DatabaseName:  schema.Database,
			SchemaName:    "public",
			Evidence:      map[string]any{"acls": schema.ACLs},
			DedupKey:      DedupKey("security.public_schema_create", key, title),
		})
	}
	return out
}

// LagFinding is true only when the environment expects a replica and lag is known and above the threshold.
func LagFinding(expectsReplica bool, lagBytes *int64, threshold int64) bool {
	if !expectsReplica || lagBytes == nil {
		return false
	}
	return *lagBytes > threshold
}

// ArchiveStalled is distinct from replication lag.
func ArchiveStalled(failed *time.Time, archived *time.Time, failedCount int64) bool {
	if failed != nil && (archived == nil || failed.After(*archived)) {
		return true
	}
	return failedCount > 0 && archived == nil
}

// P2Analyzer emits the P2 hypothesis and comparison rules.
type P2Analyzer struct{}

func (P2Analyzer) Name() string { return "p2" }

func (P2Analyzer) Analyze(_ context.Context, facts SnapshotFacts) ([]Finding, error) {
	out := make([]Finding, 0)
	out = append(out, unusedIndexFindings(facts)...)
	out = append(out, candidateFindings(facts)...)
	out = append(out, sequenceNearLimit(facts)...)
	out = append(out, definerSearchPathFindings(facts)...)
	out = append(out, rlsHypothesis(facts)...)
	out = append(out, publicSchemaCreate(facts)...)
	if LagFinding(facts.ExpectsReplica, facts.ReplayLagBytes, 64<<20) {
		out = append(out, Finding{
			EnvironmentID: facts.EnvironmentID, AuditRunID: facts.AuditRunID,
			FindingType: "replication.lag_high", Severity: SeverityHigh, Status: StatusOpen,
			Title:    "Replica apply lag is above the threshold",
			Summary:  "The environment expects a replica and replay lag exceeded 64MiB. This is not a missing-replica finding.",
			DedupKey: DedupKey("replication.lag_high", facts.EnvironmentID, "lag"),
		})
	}
	if facts.ExpectsReplica && ArchiveStalled(facts.LastFailedArchive, facts.LastArchived, facts.ArchiveFailedCount) {
		out = append(out, Finding{
			EnvironmentID: facts.EnvironmentID, AuditRunID: facts.AuditRunID,
			FindingType: "replication.archive_stalled", Severity: SeverityHigh, Status: StatusOpen,
			Title:    "WAL archive looks stalled",
			Summary:  "Archive failures are newer than the last success, or failures exist without an archived WAL. This is separate from replica lag.",
			DedupKey: DedupKey("replication.archive_stalled", facts.EnvironmentID, "archive"),
		})
	}
	for _, chunk := range facts.ChunkVacuum {
		if !ChunkDeadTupleSample(chunk.DeadTuples, chunk.LiveTuples) {
			continue
		}
		key := fmt.Sprintf("%s.%s.%s", chunk.Database, chunk.Schema, chunk.Chunk)
		title := "Chunk has local dead-tuple pressure: " + key
		out = append(out, Finding{
			EnvironmentID: facts.EnvironmentID, AuditRunID: facts.AuditRunID,
			FindingType: "timescale.chunk_dead_tuples", Severity: SeverityMedium, Status: StatusOpen,
			Title:      title,
			Summary:    "This chunk is dirty even if the hypertable average is fine. Check automatic maintenance settings. No maintenance command is applied.",
			ObjectType: "chunk", ObjectKey: key, DatabaseName: chunk.Database, SchemaName: chunk.Schema, ObjectName: chunk.Chunk,
			Evidence: map[string]any{
				"hypertable": chunk.Hypertable, "dead_tuples": chunk.DeadTuples, "live_tuples": chunk.LiveTuples,
				"last_autoanalyze": chunk.LastAnalyze, "last_automatic_maintenance": chunk.LastAutovacuum,
			},
			DedupKey: DedupKey("timescale.chunk_dead_tuples", key, title),
		})
	}
	return out, nil
}

// ChunkDeadTupleSample flags a chunk with local pressure even when the hypertable average is fine.
// The text never asks for a vacuum.
func ChunkDeadTupleSample(dead, live int64) bool {
	if dead >= 10000 {
		return true
	}
	return live > 0 && float64(dead)/float64(live) > 0.2
}

// ShouldSkipStructural is true for the fast profile when the previous structure hash matches.
func ShouldSkipStructural(profile, previousHash, currentHash string) bool {
	return profile == "fast" && previousHash != "" && previousHash == currentHash
}
