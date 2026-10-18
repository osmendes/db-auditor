// Package guidance provides user-facing language without changing the
// versioned technical evidence stored by the analyzer.
//
// The text itself lives in the action catalog (internal/actions), so the
// friendly meaning shown on a finding and the meaning of its action plan can
// never diverge. This package only narrows a plan to the two fields most
// screens and reports need.
package guidance

import "github.com/osmendes/db-auditor/internal/actions"

type Text struct {
	Meaning string
	Next    string
}

// For returns the meaning and next step of rule's action plan. A rule without
// an explicit plan gets the catalog's investigation fallback.
func For(ruleID string) Text {
	plan := actions.For(ruleID)
	return Text{Meaning: plan.Meaning, Next: plan.Next}
}

// Has reports whether ruleID has rule-specific Portuguese text.
func Has(ruleID string) bool {
	return actions.Has(ruleID)
}
