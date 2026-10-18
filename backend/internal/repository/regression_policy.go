package repository

import (
	"os"
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
type RegressionPair struct {
	PriorStarted, CurrentStarted       time.Time
	IncompleteBetween                  int
	PriorStatsReset, CurrentStatsReset *time.Time
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
	if counters && p.PriorStatsReset != nil && p.CurrentStatsReset != nil && !p.PriorStatsReset.Equal(*p.CurrentStatsReset) {
		return false, RegressionCounterReset
	}
	return true, ""
}

// PersistentRegression is the documented threshold: both the previous and the
// current complete run exceed the baseline. One noisy run never alerts.
func PersistentRegression(baseline, previous, current int) bool {
	return previous > baseline && current > baseline
}

// NewerThanLastAlert deduplicates: once an alert (open or acknowledged) exists
// for the same baseline and category, a new one is recorded only if the
// current value is worse than the last alerted value.
func NewerThanLastAlert(lastAlerted *int, current int) bool {
	return lastAlerted == nil || current > *lastAlerted
}
