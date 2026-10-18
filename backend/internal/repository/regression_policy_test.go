package repository

import (
	"testing"
	"time"
)

func TestRegressionRequiresTwoCompleteCollections(t *testing.T) {
	cases := []struct {
		current, previous string
		want              bool
	}{
		{"complete", "complete", true},
		{"partial", "complete", false},
		{"complete", "partial", false},
		{"partial", "partial", false},
		{"incompatible", "complete", false},
		{"", "complete", false},
		{"complete", "", false},
	}
	for _, tc := range cases {
		if got := AllowRegressionAlert(tc.current, tc.previous); got != tc.want {
			t.Errorf("AllowRegressionAlert(%q,%q)=%v want %v", tc.current, tc.previous, got, tc.want)
		}
	}
}

func TestFindingAlertSkipsPartialNeighbor(t *testing.T) {
	if !AllowFindingAlert(true, false, false) {
		t.Fatal("first complete collection may alert")
	}
	if AllowFindingAlert(false, false, false) {
		t.Fatal("partial collection must not alert findings")
	}
	if AllowFindingAlert(true, true, false) {
		t.Fatal("partial previous collection must silence the alert")
	}
	if !AllowFindingAlert(true, true, true) {
		t.Fatal("two complete collections may alert")
	}
}

func TestPairComparableAndThresholds(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	reset := t0.Add(-time.Hour)
	other := reset.Add(time.Minute)
	ok := RegressionPair{PriorStarted: t0, CurrentStarted: t0.Add(time.Hour), PriorStatsReset: &reset, CurrentStatsReset: &reset}
	if c, _ := PairComparable(ok, DefaultRegressionMaxGap, true); !c {
		t.Fatal("consecutive complete runs must compare")
	}
	gap := ok
	gap.CurrentStarted = t0.Add(DefaultRegressionMaxGap + time.Minute)
	if c, r := PairComparable(gap, DefaultRegressionMaxGap, true); c || r != RegressionGapWindow {
		t.Fatalf("gap: %v %s", c, r)
	}
	between := ok
	between.IncompleteBetween = 1
	if c, r := PairComparable(between, DefaultRegressionMaxGap, true); c || r != RegressionGapIntermediate {
		t.Fatalf("intermediate: %v %s", c, r)
	}
	counter := ok
	counter.CurrentStatsReset = &other
	if c, r := PairComparable(counter, DefaultRegressionMaxGap, true); c || r != RegressionCounterReset {
		t.Fatalf("reset: %v %s", c, r)
	}
	if c, _ := PairComparable(counter, DefaultRegressionMaxGap, false); !c {
		t.Fatal("structure comparison does not depend on counters")
	}
	if PersistentRegression(1, 2, 1) || PersistentRegression(1, 1, 3) || !PersistentRegression(1, 2, 2) {
		t.Fatal("persistent regression requires previous and current above baseline")
	}
	last := 3
	if NewerThanLastAlert(&last, 3) || !NewerThanLastAlert(&last, 4) || !NewerThanLastAlert(nil, 1) {
		t.Fatal("deduplication must hold the same level and reopen when worse")
	}
	t.Setenv("AUDITOR_REGRESSION_MAX_GAP", "10m")
	if RegressionMaxGap() != DefaultRegressionMaxGap {
		t.Fatal("a gap below one hour must fall back to the default")
	}
	t.Setenv("AUDITOR_REGRESSION_MAX_GAP", "48h")
	if RegressionMaxGap() != 48*time.Hour {
		t.Fatal("configured gap ignored")
	}
}
