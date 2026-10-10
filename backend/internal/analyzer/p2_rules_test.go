package analyzer

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestUnusedIndexNeedsTwoCollections(t *testing.T) {
	current := IndexFact{Database: "db", Schema: "public", IndexName: "idx", IdxScan: 0, CollectedAt: time.Now()}
	none, _ := P2Analyzer{}.Analyze(t.Context(), SnapshotFacts{Indexes: []IndexFact{current}})
	for _, f := range none {
		if f.FindingType == "index.unused" {
			t.Fatal("missing baseline must not emit index.unused")
		}
	}
	prev := current
	prev.IdxScan = 0
	prev.CollectedAt = current.CollectedAt.Add(-time.Hour)
	reset := current.CollectedAt.Add(-time.Minute)
	current.StatsReset = &reset
	resetFacts, _ := P2Analyzer{}.Analyze(t.Context(), SnapshotFacts{Indexes: []IndexFact{current}, PreviousIndexes: []IndexFact{prev}})
	for _, f := range resetFacts {
		if f.FindingType == "index.unused" {
			t.Fatal("stats reset between collections must not look unused")
		}
	}
	current.StatsReset = nil
	hit, _ := P2Analyzer{}.Analyze(t.Context(), SnapshotFacts{Indexes: []IndexFact{current}, PreviousIndexes: []IndexFact{prev}})
	if len(hit) != 1 || hit[0].Evidence["note"] != "Never recommend DROP automatically" {
		t.Fatalf("%#v", hit)
	}
	if strings.Contains(hit[0].Summary, "DROP") && strings.Contains(strings.ToLower(hit[0].Summary), "recommend drop") {
		t.Fatal(hit[0].Summary)
	}
}

func TestCandidateColumnsAvoidLeadingIndex(t *testing.T) {
	cols := CandidateColumns("select * from t where tenant_id = $1 and created_at > $2", []IndexFact{{KeyColumns: []string{"tenant_id"}}})
	if strings.Join(cols, ",") != "t,created_at" && !containsAll(cols, "created_at") {
		t.Fatalf("%v", cols)
	}
	for _, c := range cols {
		if c == "tenant_id" {
			t.Fatal("leading column should be skipped")
		}
	}
}

func TestSequenceNearLimitIgnoresSmall(t *testing.T) {
	small := int64(10)
	max := int64(100)
	out, _ := P2Analyzer{}.Analyze(t.Context(), SnapshotFacts{Sequences: []SequenceFact{{Name: "s", LastValue: &small, MaxValue: &max}}})
	if len(out) != 0 {
		t.Fatal("small max must be ignored")
	}
	last := int64(1_800_000_000)
	big := int64(2_147_483_647)
	out, _ = P2Analyzer{}.Analyze(t.Context(), SnapshotFacts{Sequences: []SequenceFact{{Database: "db", Schema: "public", Name: "s", DataType: "integer", LastValue: &last, MaxValue: &big, OwnedByColumn: "id"}}})
	if len(out) != 1 || out[0].Severity != SeverityHigh || !strings.Contains(out[0].Summary, "id") {
		t.Fatalf("%#v", out)
	}
	if strings.Contains(strings.ToUpper(out[0].Summary), "ALTER SEQUENCE") {
		t.Fatal(out[0].Summary)
	}
}

func TestDefinerSearchPath(t *testing.T) {
	facts := SnapshotFacts{Functions: []FunctionSecurityFact{
		{Database: "db", Schema: "public", FunctionName: "open", IsSecurityDefiner: true},
		{Database: "db", Schema: "public", FunctionName: "pinned", IsSecurityDefiner: true, SearchPathPinned: true},
		{Database: "db", Schema: "public", FunctionName: "plain", IsSecurityDefiner: false},
	}}
	out, _ := P2Analyzer{}.Analyze(t.Context(), facts)
	n := 0
	for _, f := range out {
		if f.FindingType == "security.definer_search_path" {
			n++
			if f.ObjectName != "open" {
				t.Fatal(f.ObjectName)
			}
		}
	}
	if n != 1 {
		t.Fatalf("want 1 definer finding, got %d", n)
	}
}

func TestDefinerSearchPathFindingKeepsOverloadAndRedactsConfig(t *testing.T) {
	facts := SnapshotFacts{Functions: []FunctionSecurityFact{{
		Database: "db", Schema: "public", FunctionName: "calc", IdentityArgs: "integer",
		IsSecurityDefiner: true, Config: "application.password=private-value",
	}}}
	out, _ := P2Analyzer{}.Analyze(t.Context(), facts)
	for _, finding := range out {
		if finding.FindingType != "security.definer_search_path" {
			continue
		}
		if finding.ObjectKey != "db.public.calc(integer)" || finding.Evidence["identity_arguments"] != "integer" {
			t.Fatalf("overload lost: %#v", finding)
		}
		if strings.Contains(fmt.Sprint(finding.Evidence), "private-value") {
			t.Fatalf("configuration leaked: %#v", finding.Evidence)
		}
		return
	}
	t.Fatal("expected search_path finding")
}

func TestLagAndArchive(t *testing.T) {
	lag := int64(100 << 20)
	if LagFinding(false, &lag, 1) {
		t.Fatal("unexpected replica must not fire")
	}
	if !LagFinding(true, &lag, 1) {
		t.Fatal("expected lag")
	}
	if LagFinding(true, nil, 1) {
		t.Fatal("unknown lag is not a finding")
	}
	failed := time.Now()
	archived := failed.Add(-time.Hour)
	if !ArchiveStalled(&failed, &archived, 0) {
		t.Fatal("stalled archive")
	}
}

func TestChunkDeadAndSkip(t *testing.T) {
	if !ChunkDeadTupleSample(10000, 1) || ChunkDeadTupleSample(10, 1000) {
		t.Fatal("dead tuple sample")
	}
	if strings.Contains("chunk has many dead tuples", "VACUUM") {
		t.Fatal("wording")
	}
	if !ShouldSkipStructural("fast", "abc", "abc") || ShouldSkipStructural("daily", "abc", "abc") || ShouldSkipStructural("fast", "", "abc") {
		t.Fatal("skip")
	}
}

func containsAll(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
