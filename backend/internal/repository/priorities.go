package repository

import (
	"context"
	"fmt"

	"github.com/osmendes/db-auditor/internal/priority"
)

// PriorityCandidateLimit bounds the open findings ranked per request. The pool
// is taken by severity first, so a cosmetic finding past this bound cannot
// displace a severe one.
const PriorityCandidateLimit = 500

// PriorityItem pairs a finding with the factors that ranked it.
type PriorityItem struct {
	Priority priority.Ranked `json:"priority"`
	Finding  Finding         `json:"finding"`
}

// ListPriorities ranks open findings by impact × effort × confidence (AN-02).
// A finding without metric or confidence stays in the pool with low
// confidence; it is never filtered out for missing data.
func (s *Store) ListPriorities(ctx context.Context, environmentID string, limit int) ([]PriorityItem, int, error) {
	if limit < 1 || limit > 20 {
		return nil, 0, fmt.Errorf("invalid priority limit")
	}
	candidates, err := s.listFindings(ctx, environmentID, "", "", "open", "", false, PriorityCandidateLimit, 0)
	if err != nil {
		return nil, 0, err
	}
	inputs := make([]priority.Input, 0, len(candidates))
	byID := make(map[string]Finding, len(candidates))
	for _, f := range candidates {
		byID[f.ID] = f
		inputs = append(inputs, priority.Input{FindingID: f.ID, RuleID: f.FindingType, Severity: f.Severity, Confidence: f.Confidence, Impact: f.ImpactEstimate})
	}
	ranked := priority.Top(inputs, limit)
	out := make([]PriorityItem, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, PriorityItem{Priority: r, Finding: byID[r.FindingID]})
	}
	return out, len(candidates), nil
}
