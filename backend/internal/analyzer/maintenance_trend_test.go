package analyzer

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestGrowthAdviceCases(t *testing.T) {
	reset := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	current := TableFact{Database: "db", Schema: "public", Name: "events", SizeBytes: 2 << 30, NTupDel: 100, CollectedAt: reset.Add(48 * time.Hour), StatsReset: &reset}
	previous := current
	previous.SizeBytes = 1 << 30
	previous.CollectedAt = reset.Add(24 * time.Hour)

	missing := AdviseGrowth(current, TableFact{}, false, false, false)
	if missing.Impact != "não estimado" || missing.Evidence["bloat"] != "não estimado" {
		t.Fatalf("missing data: %+v", missing)
	}
	partial := AdviseGrowth(current, previous, true, true, false)
	if partial.Impact != "não estimado" {
		t.Fatalf("partial: %+v", partial)
	}
	otherReset := reset.Add(time.Hour)
	current.StatsReset = &otherReset
	resetGap := AdviseGrowth(current, previous, true, true, true)
	if resetGap.Impact != "não estimado" || resetGap.Evidence["coverage"] != "stats_reset" {
		t.Fatalf("reset: %+v", resetGap)
	}
	current.StatsReset = &reset
	stablePrev := current
	stablePrev.SizeBytes = current.SizeBytes - 10
	stablePrev.CollectedAt = current.CollectedAt.Add(-24 * time.Hour)
	stable := AdviseGrowth(current, stablePrev, true, true, true)
	if !stable.Skip {
		t.Fatalf("stable table should not raise a finding: %+v", stable)
	}
	grown := AdviseGrowth(current, previous, true, true, true)
	if grown.Skip || !strings.Contains(grown.Next, "Não há exclusão automática") {
		t.Fatalf("growth: %+v", grown)
	}
	if strings.Contains(strings.ToUpper(grown.Next), "DROP") || strings.Contains(strings.ToUpper(grown.Next), "DELETE") {
		t.Fatal("advice proposed a destructive statement")
	}
	items, err := MaintenanceTrendAnalyzer{}.Analyze(context.Background(), SnapshotFacts{
		EnvironmentID: "env", CollectionComplete: true, PreviousComplete: true,
		Tables: []TableFact{current}, PreviousTables: []TableFact{previous},
	})
	if err != nil || len(items) != 1 || items[0].FindingType != "maintenance.growth_trend" {
		t.Fatalf("%v %+v", err, items)
	}
	if items[0].Evidence["measured_gain"] != "não medido" {
		t.Fatal("predicted gain was presented as measured")
	}
}
