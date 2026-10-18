package repository

import (
	"strings"
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
	legacy := &LastRegressionAlert{Current: 3}
	if RegressionIsNew(legacy, nil, 3, false) || !RegressionIsNew(legacy, nil, 4, false) || !RegressionIsNew(nil, nil, 1, false) {
		t.Fatal("legacy deduplication must hold the same level and reopen when worse")
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

// #128: a cluster never reset reports NULL; the first reset (NULL -> value)
// and a recreated cluster (value -> NULL) both change the counter epoch. The
// pg_stat_statements reset counts the same way.
func TestPairComparableNullStatsReset(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	reset := t0.Add(-time.Minute)
	base := RegressionPair{PriorStarted: t0, CurrentStarted: t0.Add(time.Hour)}
	if c, _ := PairComparable(base, DefaultRegressionMaxGap, true); !c {
		t.Fatal("NULL -> NULL is the same epoch")
	}
	for name, pair := range map[string]RegressionPair{
		"tabela NULL->valor":   {PriorStarted: base.PriorStarted, CurrentStarted: base.CurrentStarted, CurrentStatsReset: &reset},
		"tabela valor->NULL":   {PriorStarted: base.PriorStarted, CurrentStarted: base.CurrentStarted, PriorStatsReset: &reset},
		"workload NULL->valor": {PriorStarted: base.PriorStarted, CurrentStarted: base.CurrentStarted, CurrentWorkloadReset: &reset},
		"workload valor->outro": {PriorStarted: base.PriorStarted, CurrentStarted: base.CurrentStarted,
			PriorWorkloadReset: &t0, CurrentWorkloadReset: &reset},
	} {
		if c, r := PairComparable(pair, DefaultRegressionMaxGap, true); c || r != RegressionCounterReset {
			t.Errorf("%s: %v %s", name, c, r)
		}
		if c, _ := PairComparable(pair, DefaultRegressionMaxGap, false); !c {
			t.Errorf("%s: structure must ignore counters", name)
		}
	}
	same := base
	other := reset
	same.PriorWorkloadReset, same.CurrentWorkloadReset = &reset, &other
	if c, _ := PairComparable(same, DefaultRegressionMaxGap, true); !c {
		t.Fatal("equal resets are the same epoch")
	}
}

// #127: identity deduplication. The same findings never re-alert while the
// episode lasts; a different one alerts; after recovery the same one alerts.
func TestRegressionIsNewByIdentity(t *testing.T) {
	last := &LastRegressionAlert{Current: 2, Keys: []string{"a", "b"}, HasKeys: true}
	if RegressionIsNew(last, []string{"a", "b"}, 2, false) || RegressionIsNew(last, []string{"a"}, 1, false) {
		t.Fatal("same or fewer findings must not re-alert")
	}
	if !RegressionIsNew(last, []string{"c"}, 1, false) {
		t.Fatal("a different finding at the same count must alert")
	}
	if !RegressionIsNew(last, []string{"a", "c"}, 2, false) {
		t.Fatal("a new finding alongside an old one must alert")
	}
	if !RegressionIsNew(last, []string{"a", "b"}, 2, true) {
		t.Fatal("the same findings after recovery start a new episode")
	}
	if got := PersistentKeys([]string{"x", "b", "a", "a"}, []string{"a", "c", "b", "a"}); strings.Join(got, ",") != "a,b" {
		t.Fatalf("PersistentKeys: %v", got)
	}
	if got := PersistentKeys([]string{"x"}, []string{"y"}); len(got) != 0 {
		t.Fatalf("unrelated changes must not persist: %v", got)
	}
	if !Overlaps([]string{"q", "a"}, []string{"a"}) || Overlaps([]string{"q"}, []string{"a"}) || Overlaps(nil, []string{"a"}) {
		t.Fatal("Overlaps")
	}
}
