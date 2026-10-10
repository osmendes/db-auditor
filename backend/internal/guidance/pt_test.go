package guidance_test

import (
	"strings"
	"testing"

	"github.com/mayconmendes-qc/db-auditor/internal/analyzer"
	"github.com/mayconmendes-qc/db-auditor/internal/guidance"
)

func TestEveryActiveRuleHasPortugueseGuidance(t *testing.T) {
	for _, rule := range analyzer.Catalog() {
		if !guidance.Has(rule.ID) {
			t.Errorf("missing Portuguese guidance for %s", rule.ID)
			continue
		}
		text := guidance.For(rule.ID)
		if !strings.HasSuffix(text.Meaning, ".") || !strings.HasSuffix(text.Next, ".") {
			t.Errorf("incomplete guidance for %s: %+v", rule.ID, text)
		}
	}
}
