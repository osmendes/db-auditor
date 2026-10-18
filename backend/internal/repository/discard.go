package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/osmendes/db-auditor/internal/impact"
)

// DiscardDecision is how the next identical observation treats a discarded finding.
type DiscardDecision int

const (
	DiscardProceed DiscardDecision = iota
	DiscardHold
	DiscardReopen
)

// EvidenceSignature is the stable hash of a finding's evidence JSON. The JSON
// is canonicalized first (sorted keys, no whitespace) so evidence read back
// from jsonb hashes like evidence produced by the analyzer, and the volatile
// impact basis (denominators and windows that move every run) is excluded.
func EvidenceSignature(evidence []byte) string {
	var decoded any
	if err := json.Unmarshal(evidence, &decoded); err == nil {
		if m, ok := decoded.(map[string]any); ok {
			delete(m, impact.BasisKey)
		}
		if canonical, err := json.Marshal(decoded); err == nil {
			evidence = canonical
		}
	}
	sum := sha256.Sum256(evidence)
	return hex.EncodeToString(sum[:])
}

// DecideDiscard keeps an identical discard suppressed until the deadline or
// until the evidence signature changes. A missing deadline still holds.
func DecideDiscard(actionStatus, storedSignature, nextSignature string, until time.Time, hasUntil bool, now time.Time) DiscardDecision {
	if actionStatus != "discarded" {
		return DiscardProceed
	}
	same := storedSignature != "" && storedSignature == nextSignature
	open := hasUntil && !until.After(now)
	if same && !open {
		return DiscardHold
	}
	return DiscardReopen
}
