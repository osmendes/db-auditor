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
	if EvidenceSignature([]byte(`{"a":1}`)) != EvidenceSignature([]byte(`{"a":1}`)) {
		t.Fatal("signature drifted")
	}
	if EvidenceSignature([]byte(`{"a":1}`)) == EvidenceSignature([]byte(`{"a":2}`)) {
		t.Fatal("different evidence collided")
	}
}
