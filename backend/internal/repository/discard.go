package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/osmendes/db-auditor/internal/analyzer"
	"github.com/osmendes/db-auditor/internal/impact"
)

// DiscardDecision is how the next identical observation treats a discarded finding.
type DiscardDecision int

const (
	DiscardProceed DiscardDecision = iota
	DiscardHold
	DiscardReopen
)

// signatureTextAliases maps evidence text that was only translated back to
// the text it replaced, per top-level key. Hashing the old text keeps the
// signature of a discarded finding unchanged across the translation, so the
// discard still holds (#135: index.unused "note" moved to pt-BR).
var signatureTextAliases = map[string]map[string]string{
	"note": {analyzer.UnusedIndexNote: "Never recommend DROP automatically"},
}

// EvidenceSignature is the stable hash of a finding's evidence JSON. The JSON
// is canonicalized first (sorted keys, no whitespace) so evidence read back
// from jsonb hashes like evidence produced by the analyzer, the volatile
// impact basis (denominators and windows that move every run) is excluded,
// and translated fixed texts hash as their previous wording.
func EvidenceSignature(evidence []byte) string {
	var decoded any
	if err := json.Unmarshal(evidence, &decoded); err == nil {
		if m, ok := decoded.(map[string]any); ok {
			delete(m, impact.BasisKey)
			for key, aliases := range signatureTextAliases {
				if text, ok := m[key].(string); ok {
					if previous, ok := aliases[text]; ok {
						m[key] = previous
					}
				}
			}
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
