package guidance_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/osmendes/db-auditor/internal/actions"
	"github.com/osmendes/db-auditor/internal/analyzer"
	"github.com/osmendes/db-auditor/internal/guidance"
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

// AR-06: the friendly text of a finding and the meaning of its action plan come
// from the same place, so the API can never answer two texts for one rule.
func TestGuidanceAndActionPlanShareOneSource(t *testing.T) {
	ids := []string{"unknown.rule", "security.not_a_rule"}
	for _, rule := range analyzer.Catalog() {
		ids = append(ids, rule.ID)
	}
	for _, id := range ids {
		text, plan := guidance.For(id), actions.For(id)
		if text.Meaning != plan.Meaning || text.Next != plan.Next {
			t.Errorf("%s: guidance %+v diverges from plan meaning=%q next=%q", id, text, plan.Meaning, plan.Next)
		}
	}
}

// AR-06: the frontend renders meaning/next/plan from the API and keeps no
// parallel rule dictionary. A rule id used as a key in the application source
// (object or Map literal entry, or a switch case; tests and generated types
// excluded) is a parallel map. Filtering by rule id is allowed.
func TestFrontendKeepsNoParallelRuleDictionary(t *testing.T) {
	root := filepath.Join("..", "..", "..", "frontend", "src")
	if _, err := os.Stat(root); err != nil {
		t.Skipf("frontend source not available: %v", err)
	}
	var quoted []*regexp.Regexp
	for _, rule := range analyzer.Catalog() {
		q := `["'` + "`" + `]` + regexp.QuoteMeta(rule.ID) + `["'` + "`" + `]`
		quoted = append(quoted, regexp.MustCompile(q+`\s*:|\[\s*`+q+`\s*,`))
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name := d.Name()
		source := strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".tsx")
		if !source || strings.Contains(name, ".test.") || name == "openapi.ts" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, pattern := range quoted {
			if loc := pattern.FindIndex(body); loc != nil {
				t.Errorf("%s hardcodes rule id %s; read the text from the API instead", path, body[loc[0]:loc[1]])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
