package repository

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
