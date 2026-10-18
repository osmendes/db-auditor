package priority

import (
	"testing"
	"time"

	"github.com/osmendes/db-auditor/internal/impact"
)

func estimated(level string) impact.Estimate {
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	ev := map[string]any{"coverage": "complete", "delta_bytes": 10.0, "previous_size_bytes": 100.0, "period_start": at.Add(-48 * time.Hour).Format(time.RFC3339), "period_end": at.Format(time.RFC3339)}
	switch level {
	case impact.LevelHigh:
		ev["delta_bytes"] = 80.0
	case impact.LevelMedium:
		ev["delta_bytes"] = 30.0
	}
	return impact.For("maintenance.growth_trend", ev, time.Time{}, at)
}

func TestTopFiveOrdersByImpactEffortAndConfidence(t *testing.T) {
	none := impact.For("security.excessive_privilege", nil, time.Time{}, time.Time{})
	items := []Input{
		{FindingID: "cosmetic", RuleID: "model.naming_inconsistent", Severity: "low", Confidence: 0.45, Impact: none},
		{FindingID: "growth-high", RuleID: "maintenance.growth_trend", Severity: "medium", Confidence: 0.55, Impact: estimated(impact.LevelHigh)},
		{FindingID: "growth-low", RuleID: "maintenance.growth_trend", Severity: "medium", Confidence: 0.55, Impact: estimated(impact.LevelLow)},
		{FindingID: "critical-no-metric", RuleID: "security.excessive_privilege", Severity: "critical", Confidence: 0.8, Impact: none},
		{FindingID: "no-data", RuleID: "unknown.rule", Severity: "high", Confidence: 0, Impact: none},
		{FindingID: "info", RuleID: "storage.top_consumer", Severity: "info", Confidence: 0.9, Impact: none},
	}
	top := Top(items, 5)
	if len(top) != 5 {
		t.Fatalf("expected 5 priorities, got %d", len(top))
	}
	for i, item := range top {
		if item.Rank != i+1 || item.Formula != FormulaVersion || item.Reason == "" || item.Effort == "" {
			t.Fatalf("incomplete ranked item: %+v", item)
		}
	}
	index := map[string]Ranked{}
	for _, item := range Top(items, 0) {
		index[item.FindingID] = item
	}
	if index["growth-high"].Score <= index["growth-low"].Score {
		t.Fatal("measured high impact must outrank measured low impact at equal severity")
	}
	if index["cosmetic"].Rank <= index["critical-no-metric"].Rank {
		t.Fatal("a cosmetic finding must not outrank a critical one")
	}
	noData, ok := index["no-data"]
	if !ok || noData.ConfidenceLabel != "baixa" {
		t.Fatalf("finding without confidence must stay listed as baixa: %+v", noData)
	}
}

func TestMaximumScoreRequiresAMetric(t *testing.T) {
	none := impact.For("security.excessive_privilege", nil, time.Time{}, time.Time{})
	critical := Score(Input{FindingID: "a", RuleID: "integrity.orphan_sequence", Severity: "critical", Confidence: 1, Impact: none})
	if critical.Score >= MaxScore {
		t.Fatalf("critical without metric reached the maximum: %+v", critical)
	}
	measured := Score(Input{FindingID: "b", RuleID: "integrity.orphan_sequence", Severity: "critical", Confidence: 1, Impact: estimated(impact.LevelHigh)})
	if measured.Score != MaxScore {
		t.Fatalf("expected the documented maximum with a high measured impact and low effort: %+v", measured)
	}
}

func TestTopIsDeterministic(t *testing.T) {
	none := impact.For("x", nil, time.Time{}, time.Time{})
	items := []Input{{FindingID: "b", RuleID: "x", Severity: "high", Impact: none}, {FindingID: "a", RuleID: "x", Severity: "high", Impact: none}}
	first, second := Top(items, 5), Top([]Input{items[1], items[0]}, 5)
	if first[0].FindingID != "a" || second[0].FindingID != "a" {
		t.Fatalf("ties must break by id: %+v %+v", first, second)
	}
}
