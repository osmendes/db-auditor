package repository

import (
	"strings"
	"testing"
)

func TestNormalizeSearchQuery(t *testing.T) {
	if q, err := NormalizeSearchQuery("  pedidos "); err != nil || q != "pedidos" {
		t.Fatalf("got %q %v", q, err)
	}
	for _, bad := range []string{"", " a ", strings.Repeat("x", 129), "ab\ncd", "ab\x00"} {
		if _, err := NormalizeSearchQuery(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, err := NormalizeSearchQuery(strings.Repeat("é", 128)); err != nil {
		t.Fatalf("128 runes must be accepted: %v", err)
	}
}

func TestSearchLikePatternEscapesWildcards(t *testing.T) {
	if got := searchLikePattern(`50%_a\b`); got != `%50\%\_a\\b%` {
		t.Fatalf("got %q", got)
	}
}

func TestSearchRules(t *testing.T) {
	hits, _ := SearchRules("perf", 20)
	if len(hits) == 0 {
		t.Fatal("expected performance rules")
	}
	for _, h := range hits {
		if h.Kind != "rule" || h.RuleID == "" {
			t.Fatalf("bad hit %+v", h)
		}
	}
	limited, truncated := SearchRules("perf", 1)
	if len(limited) != 1 || (len(hits) > 1 && !truncated) {
		t.Fatalf("limit not applied: %d %v", len(limited), truncated)
	}
	exact, _ := SearchRules(hits[len(hits)-1].RuleID, 5)
	if len(exact) == 0 || !exact[0].Exact {
		t.Fatalf("exact rule id must rank first: %+v", exact)
	}
	if none, _ := SearchRules("zzzz-nao-existe", 5); len(none) != 0 {
		t.Fatalf("unexpected %+v", none)
	}
}
