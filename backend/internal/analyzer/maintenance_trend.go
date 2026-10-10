package analyzer

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MaintenanceTrendAnalyzer explains growth and maintenance only from two
// complete, comparable collections. It never recommends DROP, DELETE, or an
// automatic retention change.
type MaintenanceTrendAnalyzer struct{}

func (MaintenanceTrendAnalyzer) Name() string { return "maintenance-trend" }

func (MaintenanceTrendAnalyzer) Analyze(_ context.Context, facts SnapshotFacts) ([]Finding, error) {
	if !facts.CollectionComplete || !facts.PreviousComplete {
		return nil, nil
	}
	previous := map[string]TableFact{}
	for _, table := range facts.PreviousTables {
		previous[maintenanceKey(table)] = table
	}
	out := make([]Finding, 0)
	for _, table := range facts.Tables {
		key := maintenanceKey(table)
		prior, ok := previous[key]
		advice := AdviseGrowth(table, prior, ok, facts.CollectionComplete, facts.PreviousComplete)
		if advice.Skip {
			continue
		}
		f := Finding{
			EnvironmentID:  facts.EnvironmentID,
			AuditRunID:     facts.AuditRunID,
			FindingType:    "maintenance.growth_trend",
			RuleID:         "maintenance.growth_trend",
			RuleVersion:    "1",
			Category:       "maintenance",
			Confidence:     advice.Confidence,
			Impact:         advice.Impact,
			Risk:           advice.Risk,
			Recommendation: advice.Next,
			Validation:     "Confirme com uma nova coleta completa e comparável. O ganho só é medido depois dessa coleta.",
			Severity:       advice.Severity,
			Status:         StatusOpen,
			Title:          fmt.Sprintf("Tendência de manutenção: %s", key),
			Summary:        advice.Summary,
			ObjectType:     "table",
			ObjectKey:      key,
			DatabaseName:   table.Database,
			SchemaName:     table.Schema,
			ObjectName:     table.Name,
			Evidence:       advice.Evidence,
			DedupKey:       DedupKey("maintenance.growth_trend", key, "tendência"),
		}
		if strings.Contains(strings.ToUpper(f.Recommendation), "DROP") || strings.Contains(strings.ToUpper(f.Recommendation), "DELETE") {
			return nil, fmt.Errorf("maintenance advice must not propose DROP or DELETE")
		}
		out = append(out, f)
	}
	return out, nil
}

// GrowthAdvice is the pure decision for one table. Dead tuples stay an
// estimate; real bloat is not claimed without a read-only confirmation query.
type GrowthAdvice struct {
	Skip       bool
	Summary    string
	Impact     string
	Risk       string
	Next       string
	Severity   Severity
	Confidence float64
	Evidence   map[string]any
}

func AdviseGrowth(current, previous TableFact, hasPrevious, currentComplete, previousComplete bool) GrowthAdvice {
	base := map[string]any{
		"hypothesis":           true,
		"measured_gain":        "não medido",
		"dead_tuples_estimate": current.NTupDel,
		"bloat":                "não estimado",
		"confirmation_query":   "SELECT n_live_tup, n_dead_tup FROM pg_stat_user_tables WHERE relid = $1::regclass;",
		"automatic_change":     false,
	}
	if !currentComplete || !previousComplete || !hasPrevious {
		base["coverage"] = "incomplete"
		return GrowthAdvice{
			Summary:    "Sem duas coletas completas e comparáveis, a tendência de tamanho não foi calculada.",
			Impact:     "não estimado",
			Risk:       "hipótese",
			Next:       "Repita a coleta com cobertura completa. Não programe exclusão nem alteração automática; retenção depende de aprovação e de backup restaurável.",
			Severity:   SeverityInfo,
			Confidence: 0.2,
			Evidence:   base,
		}
	}
	if current.StatsReset != nil && previous.StatsReset != nil && !current.StatsReset.Equal(*previous.StatsReset) {
		base["coverage"] = "stats_reset"
		return GrowthAdvice{
			Summary:    "O contador de estatísticas foi reiniciado entre as coletas. A taxa de atividade não é comparável.",
			Impact:     "não estimado",
			Risk:       "hipótese",
			Next:       "Aguarde duas coletas completas com o mesmo stats_reset antes de estimar custo de manutenção.",
			Severity:   SeverityInfo,
			Confidence: 0.2,
			Evidence:   base,
		}
	}
	delta := current.SizeBytes - previous.SizeBytes
	window := current.CollectedAt.Sub(previous.CollectedAt)
	if window <= 0 {
		window = time.Hour
	}
	days := window.Hours() / 24
	if days <= 0 {
		days = 1
	}
	perDay := float64(delta) / days
	base["coverage"] = "complete"
	base["previous_size_bytes"] = previous.SizeBytes
	base["current_size_bytes"] = current.SizeBytes
	base["delta_bytes"] = delta
	base["window"] = window.String()
	base["bytes_per_day"] = perDay
	base["period_start"] = previous.CollectedAt.UTC().Format(time.RFC3339)
	base["period_end"] = current.CollectedAt.UTC().Format(time.RFC3339)
	stable := delta <= 0 || current.SizeBytes == 0 || float64(delta) < float64(current.SizeBytes)*0.05
	if stable {
		return GrowthAdvice{Skip: true}
	}
	return GrowthAdvice{
		Summary:    fmt.Sprintf("O tamanho cresceu %d bytes em %s. Tuplas mortas são estimativa (%d); bloat real não foi medido.", delta, window.Truncate(time.Second), current.NTupDel),
		Impact:     fmt.Sprintf("cerca de %.0f bytes/dia no período observado; custo de armazenamento não foi convertido em moeda", perDay),
		Risk:       "revisão de manutenção",
		Next:       "Confirme a tendência com o responsável. Autovacuum, compressão ou particionamento só depois de pré-condições, janela e backup. Não há exclusão automática.",
		Severity:   SeverityMedium,
		Confidence: 0.55,
		Evidence:   base,
	}
}

func maintenanceKey(t TableFact) string {
	return t.Database + "." + t.Schema + "." + t.Name
}
