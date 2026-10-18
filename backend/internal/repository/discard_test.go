package repository

import (
	"testing"
	"time"
)

func TestDecideDiscard(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	later := now.Add(24 * time.Hour)
	past := now.Add(-time.Minute)
	if got := DecideDiscard("suggested", "a", "a", later, true, now); got != DiscardProceed {
		t.Fatalf("open action must proceed: %v", got)
	}
	if got := DecideDiscard("discarded", "abc", "abc", later, true, now); got != DiscardHold {
		t.Fatalf("identical discard must hold: %v", got)
	}
	if got := DecideDiscard("discarded", "abc", "abc", time.Time{}, false, now); got != DiscardHold {
		t.Fatalf("discard without deadline must hold: %v", got)
	}
	if got := DecideDiscard("discarded", "abc", "xyz", later, true, now); got != DiscardReopen {
		t.Fatalf("new evidence must reopen: %v", got)
	}
	if got := DecideDiscard("discarded", "abc", "abc", past, true, now); got != DiscardReopen {
		t.Fatalf("expired discard must reopen: %v", got)
	}
}

func TestEvidenceSignatureStable(t *testing.T) {
	payload := []byte(`{"a":1}`)
	changed := []byte(`{"a":2}`)
	first := EvidenceSignature(payload)
	again := EvidenceSignature(payload)
	if first != again {
		t.Fatal("signature drifted")
	}
	if first == EvidenceSignature(changed) {
		t.Fatal("different evidence collided")
	}
}

func TestEvidenceSignatureIsCanonicalAndIgnoresImpactBasis(t *testing.T) {
	analyzerBytes := []byte(`{"b":2,"a":{"y":1,"x":"z"}}`)
	jsonbText := []byte(`{"a": {"x": "z", "y": 1}, "b": 2}`)
	if EvidenceSignature(analyzerBytes) != EvidenceSignature(jsonbText) {
		t.Fatal("evidence read back from jsonb must hash like the analyzer output")
	}
	withBasis := []byte(`{"a": {"x": "z", "y": 1}, "b": 2, "impact_basis": {"window_end": "2026-10-01T00:00:00Z"}}`)
	if EvidenceSignature(withBasis) != EvidenceSignature(analyzerBytes) {
		t.Fatal("the volatile impact basis must not reopen a discarded finding")
	}
}
