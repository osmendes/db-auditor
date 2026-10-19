package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/osmendes/db-auditor/internal/analyzer"
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

// #135: the index.unused "note" was translated to pt-BR. A finding discarded
// with the English evidence must keep the same signature (DiscardHold), so the
// translation alone never reopens it. The legacy hash is computed here from
// the canonical bytes, independently of EvidenceSignature.
func TestEvidenceSignatureSurvivesNoteTranslation(t *testing.T) {
	now := time.Now()
	current := analyzer.IndexFact{Database: "erp", Schema: "public", TableName: "log_evento", IndexName: "log_evento_old_idx", SizeBytes: 8192, CollectedAt: now}
	previous := current
	previous.CollectedAt = now.Add(-24 * time.Hour)
	out, err := analyzer.P2Analyzer{}.Analyze(t.Context(), analyzer.SnapshotFacts{Indexes: []analyzer.IndexFact{current}, PreviousIndexes: []analyzer.IndexFact{previous}})
	if err != nil || len(out) != 1 || out[0].FindingType != "index.unused" {
		t.Fatalf("index.unused fixture: %+v %v", out, err)
	}
	note, _ := out[0].Evidence["note"].(string)
	if note != analyzer.UnusedIndexNote || !strings.Contains(note, "nunca recomenda DROP") {
		t.Fatalf("note must be the pt-BR text: %q", note)
	}
	translated, err := json.Marshal(out[0].Evidence)
	if err != nil {
		t.Fatal(err)
	}
	legacyEvidence := map[string]any{}
	for key, value := range out[0].Evidence {
		legacyEvidence[key] = value
	}
	legacyEvidence["note"] = "Never recommend DROP automatically"
	legacy, err := json.Marshal(legacyEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if EvidenceSignature(translated) != EvidenceSignature(legacy) {
		t.Fatal("the pt-BR note must hash like the English note stored with older discards")
	}
	if got := DecideDiscard("discarded", EvidenceSignature(legacy), EvidenceSignature(translated), time.Time{}, false, now); got != DiscardHold {
		t.Fatalf("translation reopened a discarded finding: %v", got)
	}
	// Byte-level reference: the old signature of a fixed English payload.
	fixed := []byte(`{"current_idx_scan":0,"note":"Never recommend DROP automatically","previous_idx_scan":0,"table_name":"log_evento"}`)
	sum := sha256.Sum256(fixed)
	if want := hex.EncodeToString(sum[:]); EvidenceSignature(fixed) != want ||
		EvidenceSignature([]byte(`{"table_name":"log_evento","previous_idx_scan":0,"current_idx_scan":0,"note":"`+analyzer.UnusedIndexNote+`"}`)) != want {
		t.Fatal("translated note must keep the byte-identical legacy signature")
	}
	// Any other change of the note is new evidence.
	other := []byte(`{"current_idx_scan":0,"note":"outro texto","previous_idx_scan":0,"table_name":"log_evento"}`)
	if EvidenceSignature(other) == EvidenceSignature(fixed) {
		t.Fatal("only the known translation is aliased")
	}
}
