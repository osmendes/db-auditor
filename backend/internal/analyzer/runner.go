package analyzer

import (
	"context"
	"fmt"

	"github.com/mayconmendes-qc/db-auditor/internal/capabilities"
)

// Runner executes all registered analyzers against snapshot facts.
type Runner struct {
	Registry *Registry
}

// NewRunner builds a runner with the given registry (defaults if nil).
func NewRunner(reg *Registry) *Runner {
	if reg == nil {
		reg = DefaultRegistry()
	}
	return &Runner{Registry: reg}
}

// Run executes every analyzer and concatenates findings.
func (r *Runner) Run(ctx context.Context, facts SnapshotFacts) ([]Finding, error) {
	all := make([]Finding, 0)
	for _, a := range r.Registry.All() {
		if err := ctx.Err(); err != nil {
			return all, err
		}
		items, err := a.Analyze(ctx, facts)
		if err != nil {
			return all, fmt.Errorf("analyzer %s: %w", a.Name(), err)
		}
		timescaleObserved := facts.TimescaleVersion != "" || len(facts.Hypertables) > 0 || len(facts.Chunks) > 0 || len(facts.CAGGs) > 0 || len(facts.Policies) > 0 || len(facts.Jobs) > 0
		for _, item := range items {
			if capabilities.RuleApplicable(facts.Engine, item.FindingType, timescaleObserved) {
				all = append(all, item)
			}
		}
	}
	return EnrichFindings(facts, all), nil
}
