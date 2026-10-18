package impact

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func decode(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestEstimatesRequireValueDenominatorAndPeriod(t *testing.T) {
	seen := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name, rule, evidence string
		estimated            bool
		unit, level          string
	}{
		{"índice sem uso com janela", "index.unused", `{"impact_basis":{"size_bytes":400,"table_index_bytes":1000,"window_start":"2026-09-30T12:00:00Z","window_end":"2026-10-01T12:00:00Z"}}`, true, "bytes", LevelHigh},
		{"índice sem denominador", "index.unused", `{"current_idx_scan":0}`, false, "", LevelNone},
		{"tuplas mortas com vacuum conhecido", "vacuum.high_dead_tuples", `{"n_dead_tup":300,"n_live_tup":700,"last_autovacuum":"2026-09-29 08:00:00.5+00"}`, true, "tuplas mortas (estimativa)", LevelMedium},
		{"tuplas mortas sem janela", "vacuum.high_dead_tuples", `{"n_dead_tup":300,"n_live_tup":700,"last_vacuum":"","last_autovacuum":""}`, false, "", LevelNone},
		{"seq scan com janela", "performance.workload_scan", `{"shared_blocks_read":900,"shared_blocks_hit":100,"observation_window_start":"2026-09-01T00:00:00Z","observation_window_end":"2026-10-01T00:00:00Z"}`, true, "blocos de 8 KiB lidos fora do cache", LevelHigh},
		{"seq scan sem stats_reset", "performance.workload_scan", `{"shared_blocks_read":900,"shared_blocks_hit":100,"observation_window_start":"unknown","observation_window_end":"2026-10-01T00:00:00Z"}`, false, "", LevelNone},
		{"espaço com denominador", "storage.large_table", `{"size_bytes":50,"impact_basis":{"database_size_bytes":1000,"collected_at":"2026-10-01T12:00:00Z"}}`, true, "bytes", LevelLow},
		{"espaço sem total do banco", "storage.large_table", `{"size_bytes":50}`, false, "", LevelNone},
		{"crescimento completo", "maintenance.growth_trend", `{"coverage":"complete","delta_bytes":600,"previous_size_bytes":1000,"period_start":"2026-09-29T00:00:00Z","period_end":"2026-10-01T00:00:00Z"}`, true, "bytes", LevelHigh},
		{"crescimento com lacuna", "maintenance.growth_trend", `{"coverage":"gap"}`, false, "", LevelNone},
		{"regra sem métrica", "security.excessive_privilege", `{"role":"app"}`, false, "", LevelNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := For(tc.rule, decode(t, tc.evidence), seen.Add(-time.Hour), seen)
			if got.Estimated() != tc.estimated || got.Level != tc.level {
				t.Fatalf("estimated=%v level=%s; want %v %s (%+v)", got.Estimated(), got.Level, tc.estimated, tc.level, got)
			}
			if tc.estimated {
				if got.Value == nil || got.Denominator == nil || got.Ratio == nil || got.PeriodStart == nil || got.PeriodEnd == nil || got.Unit != tc.unit {
					t.Fatalf("estimate lacks value/denominator/period/unit: %+v", got)
				}
				if !strings.Contains(got.Label, got.Unit) || !strings.Contains(got.Label, "período") {
					t.Fatalf("label must show unit and period: %s", got.Label)
				}
			} else if got.Value != nil || got.Reason == "" || !strings.HasPrefix(got.Label, "impacto não estimado") {
				t.Fatalf("missing data must read 'não estimado' with a reason: %+v", got)
			}
		})
	}
}

func TestIndexWindowFallsBackToObservationSpan(t *testing.T) {
	first := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	last := first.Add(72 * time.Hour)
	got := For("index.unused", map[string]any{BasisKey: map[string]any{"size_bytes": int64(10), "table_index_bytes": int64(100)}}, first, last)
	if !got.Estimated() || !got.PeriodStart.Equal(first) || !got.PeriodEnd.Equal(last) {
		t.Fatalf("expected first_seen..last_seen window: %+v", got)
	}
	if none := For("index.unused", map[string]any{BasisKey: map[string]any{"size_bytes": 10, "table_index_bytes": 100}}, last, last); none.Estimated() {
		t.Fatal("a single observation has no window and must not be estimated")
	}
}

func TestPeriodLabel(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 30, 0, 0, time.UTC)
	if got := PeriodLabel(at, at); got != "coleta de 01/10/2026 09:30 UTC" {
		t.Fatal(got)
	}
	if got := PeriodLabel(at.Add(-24*time.Hour), at); got != "30/09/2026 09:30 UTC a 01/10/2026 09:30 UTC" {
		t.Fatal(got)
	}
}

func TestGainKeepsHypothesisPredictedAndMeasuredApart(t *testing.T) {
	ev := map[string]any{"hypothesis": "Crescimento é hipótese.", "expected_gain": "não estimado: depende de teste"}
	g := BuildGain("Tabela cresce.", ev, notEstimated("x"), nil)
	if g.Hypothesis != "Crescimento é hipótese." || g.Predicted != "não estimado: depende de teste" || g.MeasuredStatus != MeasuredNone || !strings.HasPrefix(g.Measured, "não medido") {
		t.Fatalf("unmeasured gain: %+v", g)
	}
	before, after := int64(1000), int64(600)
	g = BuildGain("Tabela cresce.", ev, notEstimated("x"), &Measurement{Metric: "table_size_bytes", Before: &before, After: &after, Comparable: true, AfterRunID: "run-2"})
	if g.MeasuredStatus != MeasuredDone || !strings.Contains(g.Measured, "variação -400") || !strings.Contains(g.Measured, "run-2") {
		t.Fatalf("measured gain: %+v", g)
	}
	g = BuildGain("Tabela cresce.", ev, notEstimated("x"), &Measurement{Metric: "table_size_bytes", Before: &before, After: &after, Comparable: false, Note: "carga diferente"})
	if g.MeasuredStatus != MeasuredNotComparable || !strings.Contains(g.Measured, "carga diferente") {
		t.Fatalf("non comparable gain must not be presented as measured: %+v", g)
	}
}
