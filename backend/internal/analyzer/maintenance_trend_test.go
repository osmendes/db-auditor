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

// BA-21 acceptance: no data, partial, engine without capability, regression
// (growth), stable, gap and counter reset, plus hypothesis vs measured gain.
func TestGrowthTrendAcceptanceMatrix(t *testing.T) {
	t0 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	reset := t0.Add(-time.Hour)
	prev := TableFact{Database: "db", Schema: "public", Name: "events", SizeBytes: 1 << 30, CollectedAt: t0, StatsReset: &reset}
	cur := prev
	cur.SizeBytes = 2 << 30
	cur.CollectedAt = t0.Add(48 * time.Hour)
	stable := cur
	stable.SizeBytes = prev.SizeBytes + 10
	otherReset := reset.Add(time.Minute)
	resetCur := cur
	resetCur.StatsReset = &otherReset
	cases := []struct {
		name     string
		in       GrowthInput
		skip     bool
		coverage string
	}{
		{"sem dados", GrowthInput{Current: cur}, false, "incomplete"},
		{"coleta parcial", GrowthInput{Current: cur, Previous: prev, HasPrevious: true, CurrentComplete: true}, false, "incomplete"},
		{"mecanismo sem capacidade", GrowthInput{Current: cur, Previous: prev, HasPrevious: true, CurrentComplete: true, PreviousComplete: true, Engine: "mongodb"}, true, "unsupported_engine"},
		{"regressão (crescimento)", GrowthInput{Current: cur, Previous: prev, HasPrevious: true, CurrentComplete: true, PreviousComplete: true, Engine: "postgresql"}, false, "complete"},
		{"estável", GrowthInput{Current: stable, Previous: prev, HasPrevious: true, CurrentComplete: true, PreviousComplete: true}, true, "complete"},
		{"lacuna por execução parcial no meio", GrowthInput{Current: cur, Previous: prev, HasPrevious: true, CurrentComplete: true, PreviousComplete: true, Gap: true}, false, "gap"},
		{"lacuna por intervalo longo", GrowthInput{Current: cur, Previous: prev, HasPrevious: true, CurrentComplete: true, PreviousComplete: true, MaxWindow: time.Hour}, false, "gap"},
		{"reset de contador", GrowthInput{Current: resetCur, Previous: prev, HasPrevious: true, CurrentComplete: true, PreviousComplete: true}, false, "stats_reset"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AdviseGrowthWith(tc.in)
			if got.Skip != tc.skip || got.Evidence["coverage"] != tc.coverage {
				t.Fatalf("skip=%v coverage=%v; want %v %v (%+v)", got.Skip, got.Evidence["coverage"], tc.skip, tc.coverage, got)
			}
			if tc.coverage != "complete" && got.Impact != "não estimado" && !got.Skip {
				t.Fatalf("incomparable pair stated an impact: %s", got.Impact)
			}
			if got.Evidence["measured_gain"] != "não medido" || got.Evidence["expected_gain"] == nil || got.Evidence["hypothesis"] == nil {
				t.Fatalf("hypothesis, expected and measured gain must stay distinct: %+v", got.Evidence)
			}
			if strings.Contains(strings.ToUpper(got.Next), "DROP") || strings.Contains(strings.ToUpper(got.Next), "DELETE") || got.Evidence["automatic_change"] != false {
				t.Fatal("advice proposed a destructive or automatic change")
			}
		})
	}
	growth := AdviseGrowthWith(cases[3].in)
	if growth.Severity != SeverityMedium || !strings.Contains(growth.Impact, "bytes/dia") || growth.Evidence["period_start"] == nil {
		t.Fatalf("growth must carry rate and period: %+v", growth)
	}
	options, _ := growth.Evidence["options"].([]GrowthOption)
	for _, o := range options {
		if o.Kind == "compression" {
			t.Fatal("plain PostgreSQL table offered Timescale compression")
		}
	}
	hyper := cases[3].in
	hyper.Hypertable = true
	hyperOptions, _ := AdviseGrowthWith(hyper).Evidence["options"].([]GrowthOption)
	found := false
	for _, o := range hyperOptions {
		found = found || o.Kind == "compression" && o.Capability == "timescaledb"
	}
	if !found {
		t.Fatal("hypertable should list compression as a TimescaleDB option")
	}
	items, err := MaintenanceTrendAnalyzer{}.Analyze(context.Background(), SnapshotFacts{Engine: "mongodb", CollectionComplete: true, PreviousComplete: true,
		Tables: []TableFact{cur}, PreviousTables: []TableFact{prev}})
	if err != nil || len(items) != 0 {
		t.Fatalf("engine without table statistics produced trend findings: %+v %v", items, err)
	}
	items, err = MaintenanceTrendAnalyzer{}.Analyze(context.Background(), SnapshotFacts{Engine: "postgresql", CollectionComplete: true, PreviousComplete: true, PreviousGap: true,
		Tables: []TableFact{cur}, PreviousTables: []TableFact{prev}})
	if err != nil || len(items) != 1 || items[0].Evidence["coverage"] != "gap" || items[0].Severity != SeverityInfo {
		t.Fatalf("gap must be identified as not comparable: %+v %v", items, err)
	}
	enriched := EnrichFindings(SnapshotFacts{}, items)
	if enriched[0].Confidence > 0.2 || enriched[0].Impact != "não estimado" {
		t.Fatalf("catalog enrichment overwrote the conservative trend: %+v", enriched[0])
	}
}
