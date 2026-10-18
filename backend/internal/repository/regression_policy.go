package repository

import (
	"os"
	"sort"
	"strings"
	"time"
)

// AllowRegressionAlert is true only when both collections are complete.
// A partial side, a gap, or a missing run must not emit a regression.
func AllowRegressionAlert(current, previous string) bool {
	return current == "complete" && previous == "complete"
}

// AllowFindingAlert reports whether a newly observed finding may be notified.
// The first complete collection may alert. A later alert requires the previous
// successful collection to be complete as well, so a partial neighbor cannot
// look like a regression. Job failures are not regressions and stay alertable.
func AllowFindingAlert(currentComplete, hasPrevious, previousComplete bool) bool {
	if !currentComplete {
		return false
	}
	if !hasPrevious {
		return true
	}
	return previousComplete
}

// DefaultRegressionMaxGap is the longest interval between the previous and the
// current complete run that still compares as consecutive collections.
const DefaultRegressionMaxGap = 7 * 24 * time.Hour

// Regression silence reasons. They are logged and returned to tests; none of
// them creates an alert.
const (
	RegressionRecorded        = "recorded"
	RegressionPartial         = "partial_collection"
	RegressionNoPrevious      = "no_previous_run"
	RegressionNoBaseline      = "no_baseline"
	RegressionIncompatible    = "incompatible_versions"
	RegressionGapWindow       = "gap_window"
	RegressionGapIntermediate = "intermediate_incomplete_run"
	RegressionCounterReset    = "stats_reset"
	RegressionNotPersistent   = "below_threshold"
	RegressionAlreadyAlerted  = "deduplicated"
)

// RegressionMaxGap reads AUDITOR_REGRESSION_MAX_GAP (Go duration such as
// "168h"). Invalid or shorter than one hour falls back to the default.
func RegressionMaxGap() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("AUDITOR_REGRESSION_MAX_GAP")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d >= time.Hour {
			return d
		}
	}
	return DefaultRegressionMaxGap
}

// RegressionPair is the temporal context of the previous/current pair.
// StatsReset is the table counters reset (pg_stat_database.stats_reset) and
// WorkloadReset the pg_stat_statements reset seen by each run.
type RegressionPair struct {
	PriorStarted, CurrentStarted             time.Time
	IncompleteBetween                        int
	PriorStatsReset, CurrentStatsReset       *time.Time
	PriorWorkloadReset, CurrentWorkloadReset *time.Time
}

// statsResetChanged is true when the two runs saw different counter epochs.
// A cluster never reset reports NULL, so the first reset is NULL -> value
// (and a recreated cluster value -> NULL); both are resets (#128).
func statsResetChanged(prior, current *time.Time) bool {
	if prior == nil || current == nil {
		return prior != current
	}
	return !prior.Equal(*current)
}

// PairComparable rejects a temporal gap, an incomplete run in between and a
// counter reset. Counter-based findings (idx_scan, pg_stat_statements) change
// meaning after stats_reset, so a reset silences the findings category.
func PairComparable(p RegressionPair, maxGap time.Duration, counters bool) (bool, string) {
	if p.IncompleteBetween > 0 {
		return false, RegressionGapIntermediate
	}
	if p.PriorStarted.IsZero() || p.CurrentStarted.IsZero() || !p.CurrentStarted.After(p.PriorStarted) || p.CurrentStarted.Sub(p.PriorStarted) > maxGap {
		return false, RegressionGapWindow
	}
	if counters && (statsResetChanged(p.PriorStatsReset, p.CurrentStatsReset) || statsResetChanged(p.PriorWorkloadReset, p.CurrentWorkloadReset)) {
		return false, RegressionCounterReset
	}
	return true, ""
}

// PersistentRegression is the documented threshold: both the previous and the
// current complete run exceed the baseline. One noisy run never alerts.
func PersistentRegression(baseline, previous, current int) bool {
	return previous > baseline && current > baseline
}

// LastRegressionAlert is the latest alert (open or acknowledged) for the same
// baseline and category. Keys is the identity of what it alerted on; alerts
// recorded before identities existed have HasKeys=false.
type LastRegressionAlert struct {
	Current int
	Keys    []string
	HasKeys bool
}

// RegressionIsNew deduplicates by identity (#127). Without a previous alert,
// or once a complete run showed the alerted regression gone (recovered), a
// persistent regression alerts. Otherwise it alerts only if it contains
// something the last alert did not: a different finding, table, index or
// query. A legacy alert without identity falls back to "worse than the last
// alerted count".
func RegressionIsNew(last *LastRegressionAlert, keys []string, current int, recovered bool) bool {
	if last == nil || recovered {
		return true
	}
	if !last.HasKeys {
		return current > last.Current
	}
	seen := make(map[string]struct{}, len(last.Keys))
	for _, key := range last.Keys {
		seen[key] = struct{}{}
	}
	for _, key := range keys {
		if _, ok := seen[key]; !ok {
			return true
		}
	}
	return false
}

// PersistentKeys is the identity-based threshold used for structure and
// workload: the same table, index or query must regress against the baseline
// in both the previous and the current complete run. One noisy run, or two
// unrelated changes, never alert.
func PersistentKeys(previous, current []string) []string {
	inPrevious := make(map[string]struct{}, len(previous))
	for _, key := range previous {
		inPrevious[key] = struct{}{}
	}
	out := []string{}
	for _, key := range current {
		if _, ok := inPrevious[key]; ok {
			out = append(out, key)
			delete(inPrevious, key)
		}
	}
	sort.Strings(out)
	return out
}

// Overlaps reports whether any key of a is in b.
func Overlaps(a, b []string) bool {
	in := make(map[string]struct{}, len(b))
	for _, key := range b {
		in[key] = struct{}{}
	}
	for _, key := range a {
		if _, ok := in[key]; ok {
			return true
		}
	}
	return false
}
