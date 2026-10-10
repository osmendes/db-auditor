package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// DiscardDecision is how the next identical observation treats a discarded finding.
type DiscardDecision int

const (
	DiscardProceed DiscardDecision = iota
	DiscardHold
	DiscardReopen
)

// EvidenceSignature is the stable hash of a finding's evidence JSON.
func EvidenceSignature(evidence []byte) string {
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
