// Package priority ranks findings by conservative impact, effort and evidence
// confidence (AN-02). It is deterministic and pure: the same inputs always
// produce the same order, and missing data lowers confidence instead of
// hiding a finding.
package priority

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/osmendes/db-auditor/internal/actions"
	"github.com/osmendes/db-auditor/internal/impact"
)

// FormulaVersion identifies the ranking formula documented in docs/rules.md.
const FormulaVersion = "priority-v1"

// MaxScore is reachable only with a measured impact (impact.LevelHigh),
// critical severity, full confidence and low effort. A finding without a
// metric can never reach it (AN-03 "nenhuma nota máxima sem métrica").
const MaxScore = 5 * 1.5 * 1.0 / 1.0

// Input is the minimum a finding must provide to be ranked.
type Input struct {
	FindingID  string
	RuleID     string
	Severity   string
	Confidence float64
	Impact     impact.Estimate
}

// Ranked is one prioritized item with the factors that produced its score.
type Ranked struct {
	FindingID       string  `json:"finding_id"`
	Rank            int     `json:"rank"`
	Score           float64 `json:"score"`
	MaxScore        float64 `json:"max_score"`
	Severity        string  `json:"severity"`
	ImpactLevel     string  `json:"impact_level"`
	ImpactLabel     string  `json:"impact_label"`
	Effort          string  `json:"effort"`
	Confidence      float64 `json:"confidence"`
	ConfidenceLabel string  `json:"confidence_label"`
	Risk            string  `json:"risk"`
	Reason          string  `json:"reason"`
	Formula         string  `json:"formula"`
}

func severityWeight(severity string) float64 {
	switch strings.ToLower(severity) {
	case "critical":
		return 5
	case "high":
		return 4
	case "medium":
		return 3
	case "low":
		return 2
	default:
		return 1
	}
}

func impactFactor(e impact.Estimate) float64 {
	if !e.Estimated() {
		return 0.8
	}
	switch e.Level {
	case impact.LevelHigh:
		return 1.5
	case impact.LevelMedium:
		return 1.2
	default:
		return 1.0
	}
}

func effortFactor(effort string) float64 {
	switch effort {
	case "baixo":
		return 1
	case "alto":
		return 2
	default:
		return 1.5
	}
}

// ConfidenceLabel maps the numeric evidence confidence. Missing (<= 0) is
// "baixa", never a reason to drop the finding.
func ConfidenceLabel(c float64) string {
	switch {
	case c >= 0.75:
		return "alta"
	case c >= 0.5:
		return "média"
	default:
		return "baixa"
	}
}

// Score computes one item without ranking it.
func Score(in Input) Ranked {
	plan := actions.For(in.RuleID)
	confidence := in.Confidence
	if confidence <= 0 || math.IsNaN(confidence) {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	factor := confidence
	if factor < 0.3 {
		factor = 0.3
	}
	score := severityWeight(in.Severity) * impactFactor(in.Impact) * factor / effortFactor(plan.Effort)
	score = math.Round(score*100) / 100
	label := in.Impact.Label
	if label == "" {
		label = "impacto não estimado"
	}
	level := in.Impact.Level
	if level == "" {
		level = impact.LevelNone
	}
	reason := fmt.Sprintf("severidade %s × impacto %s × confiança %s ÷ esforço %s", strings.ToLower(in.Severity), level, ConfidenceLabel(confidence), plan.Effort)
	if !in.Impact.Estimated() {
		reason += "; sem métrica, o impacto não pode elevar a prioridade"
	}
	return Ranked{FindingID: in.FindingID, Score: score, MaxScore: MaxScore, Severity: in.Severity, ImpactLevel: level, ImpactLabel: label,
		Effort: plan.Effort, Confidence: confidence, ConfidenceLabel: ConfidenceLabel(confidence), Risk: plan.Risk, Reason: reason, Formula: FormulaVersion}
}

// Top ranks items and returns at most n. Ties fall back to severity and then
// finding id so the order is stable across requests.
func Top(items []Input, n int) []Ranked {
	ranked := make([]Ranked, 0, len(items))
	for _, item := range items {
		ranked = append(ranked, Score(item))
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if severityWeight(a.Severity) != severityWeight(b.Severity) {
			return severityWeight(a.Severity) > severityWeight(b.Severity)
		}
		return a.FindingID < b.FindingID
	})
	if n > 0 && len(ranked) > n {
		ranked = ranked[:n]
	}
	for i := range ranked {
		ranked[i].Rank = i + 1
	}
	return ranked
}
