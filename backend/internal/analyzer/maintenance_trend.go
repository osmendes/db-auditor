package analyzer

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MaxComparableWindow is the longest interval between two complete collections
// that still counts as one trend. A longer interval is a gap (BA-21).
const MaxComparableWindow = 14 * 24 * time.Hour

// MaintenanceTrendAnalyzer explains growth and maintenance only from two
// complete, comparable collections. It never recommends DROP, DELETE, or an
// automatic retention change.
type MaintenanceTrendAnalyzer struct{}

func (MaintenanceTrendAnalyzer) Name() string { return "maintenance-trend" }

func (MaintenanceTrendAnalyzer) Analyze(_ context.Context, facts SnapshotFacts) ([]Finding, error) {
	if !facts.CollectionComplete || !facts.PreviousComplete {
		return nil, nil
	}
	if !engineHasTableStats(facts.Engine) {
		return nil, nil
	}
	previous := map[string]TableFact{}
	for _, table := range facts.PreviousTables {
		previous[maintenanceKey(table)] = table
	}
	hypertables := map[string]bool{}
	for _, h := range facts.Hypertables {
		hypertables[h.Database+"."+h.Schema+"."+h.Name] = true
	}
	out := make([]Finding, 0)
	for _, table := range facts.Tables {
		key := maintenanceKey(table)
		prior, ok := previous[key]
		advice := AdviseGrowthWith(GrowthInput{Current: table, Previous: prior, HasPrevious: ok,
			CurrentComplete: facts.CollectionComplete, PreviousComplete: facts.PreviousComplete,
			Gap: facts.PreviousGap, Engine: facts.Engine, Hypertable: hypertables[key]})
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

// GrowthInput is everything the trend decision may use.
type GrowthInput struct {
	Current, Previous TableFact
	HasPrevious       bool
	CurrentComplete   bool
	PreviousComplete  bool
	// Gap is true when a partial or failed run sits between the two complete
	// collections, so the interval is not one continuous observation.
	Gap        bool
	Engine     string
	Hypertable bool
	MaxWindow  time.Duration
}

// GrowthOption is one external maintenance path the operator may evaluate.
// Each one maps to an actions.MaintenanceCost of the same kind.
type GrowthOption struct {
	Kind       string `json:"kind"`
	Capability string `json:"capability"`
	Note       string `json:"note"`
}

func engineHasTableStats(engine string) bool {
	return engine == "" || engine == "postgresql" || engine == "timescaledb"
}

func growthOptions(engine string, hypertable bool) []GrowthOption {
	if !engineHasTableStats(engine) {
		return nil
	}
	options := []GrowthOption{{Kind: "autovacuum", Capability: "postgresql", Note: "Revise autovacuum/analyze da tabela; tuplas mortas são estimativa."}}
	if hypertable {
		options = append(options,
			GrowthOption{Kind: "compression", Capability: "timescaledb", Note: "Compressão de chunks fechados; razão só é conhecida após teste."},
			GrowthOption{Kind: "retention", Capability: "timescaledb", Note: "Retenção depende de prazo aprovado pelo negócio/compliance e backup restaurável."})
	} else {
		options = append(options,
			GrowthOption{Kind: "partitioning", Capability: "postgresql", Note: "Particionamento declarativo exige chave estável e janela de migração."},
			GrowthOption{Kind: "retention", Capability: "postgresql", Note: "Arquivamento/retenção externo depende de aprovação do negócio e backup restaurável."})
	}
	return options
}

// AdviseGrowth keeps the original signature for callers without engine context.
func AdviseGrowth(current, previous TableFact, hasPrevious, currentComplete, previousComplete bool) GrowthAdvice {
	return AdviseGrowthWith(GrowthInput{Current: current, Previous: previous, HasPrevious: hasPrevious, CurrentComplete: currentComplete, PreviousComplete: previousComplete})
}

func AdviseGrowthWith(in GrowthInput) GrowthAdvice {
	current, previous := in.Current, in.Previous
	maxWindow := in.MaxWindow
	if maxWindow <= 0 {
		maxWindow = MaxComparableWindow
	}
	base := map[string]any{
		"hypothesis":           "Crescimento observado pode indicar necessidade de manutenção, particionamento ou retenção; é hipótese até confirmação humana.",
		"expected_gain":        "não estimado: depende de teste da opção escolhida em homologação",
		"measured_gain":        "não medido",
		"measured_gain_note":   "O ganho só é medido por uma nova coleta completa e comparável depois da ação externa.",
		"dead_tuples_estimate": current.NTupDel,
		"bloat":                "não estimado",
		"confirmation_query":   "SELECT n_live_tup, n_dead_tup FROM pg_stat_user_tables WHERE relid = $1::regclass;",
		"automatic_change":     false,
	}
	notEstimated := func(coverage, summary, next string) GrowthAdvice {
		base["coverage"] = coverage
		return GrowthAdvice{Summary: summary, Impact: "não estimado", Risk: "hipótese", Next: next, Severity: SeverityInfo, Confidence: 0.2, Evidence: base}
	}
	if !engineHasTableStats(in.Engine) {
		base["capability"] = "unsupported"
		advice := notEstimated("unsupported_engine", "O mecanismo não expõe tamanho e estatísticas comparáveis; a tendência não foi calculada.",
			"Peça confirmação humana com as ferramentas do mecanismo; o auditor não estima manutenção aqui.")
		advice.Skip = true
		return advice
	}
	base["options"] = growthOptions(in.Engine, in.Hypertable)
	if !in.CurrentComplete || !in.PreviousComplete || !in.HasPrevious {
		return notEstimated("incomplete", "Sem duas coletas completas e comparáveis, a tendência de tamanho não foi calculada.",
			"Repita a coleta com cobertura completa. Não programe exclusão nem alteração automática; retenção depende de aprovação e de backup restaurável.")
	}
	delta := current.SizeBytes - previous.SizeBytes
	grew := delta > 0 && current.SizeBytes > 0 && float64(delta) >= float64(current.SizeBytes)*0.05
	window := current.CollectedAt.Sub(previous.CollectedAt)
	if current.StatsReset != nil && previous.StatsReset != nil && !current.StatsReset.Equal(*previous.StatsReset) {
		if !grew {
			return GrowthAdvice{Skip: true, Evidence: base}
		}
		return notEstimated("stats_reset", "O contador de estatísticas foi reiniciado entre as coletas. A taxa de atividade não é comparável.",
			"Aguarde duas coletas completas com o mesmo stats_reset antes de estimar custo de manutenção.")
	}
	if in.Gap || current.CollectedAt.IsZero() || previous.CollectedAt.IsZero() || window <= 0 || window > maxWindow {
		if !grew {
			return GrowthAdvice{Skip: true, Evidence: base}
		}
		base["gap"] = true
		return notEstimated("gap", "Há lacuna entre as coletas (execução parcial ou intervalo longo demais). O crescimento não é tratado como tendência.",
			"Faça duas coletas completas consecutivas antes de estimar taxa ou custo de manutenção.")
	}
	days := window.Hours() / 24
	perDay := float64(delta) / days
	base["coverage"] = "complete"
	base["gap"] = false
	base["previous_size_bytes"] = previous.SizeBytes
	base["current_size_bytes"] = current.SizeBytes
	base["delta_bytes"] = delta
	base["window"] = window.String()
	base["bytes_per_day"] = perDay
	base["period_start"] = previous.CollectedAt.UTC().Format(time.RFC3339)
	base["period_end"] = current.CollectedAt.UTC().Format(time.RFC3339)
	if !grew {
		return GrowthAdvice{Skip: true, Evidence: base}
	}
	confidence := 0.55
	if window < 24*time.Hour {
		confidence = 0.35
	}
	return GrowthAdvice{
		Summary:    fmt.Sprintf("O tamanho cresceu %d bytes em %s. Tuplas mortas são estimativa (%d); bloat real não foi medido.", delta, window.Truncate(time.Second), current.NTupDel),
		Impact:     fmt.Sprintf("cerca de %.0f bytes/dia no período observado; custo de armazenamento não foi convertido em moeda", perDay),
		Risk:       "revisão de manutenção",
		Next:       "Confirme a tendência com o responsável. Autovacuum, compressão ou particionamento só depois de pré-condições, janela e backup. Não há exclusão automática.",
		Severity:   SeverityMedium,
		Confidence: confidence,
		Evidence:   base,
	}
}

func maintenanceKey(t TableFact) string {
	return t.Database + "." + t.Schema + "." + t.Name
}
