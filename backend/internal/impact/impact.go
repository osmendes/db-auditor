// Package impact turns a finding's evidence into a conservative impact
// estimate. A number appears only when the evidence carries a measured value,
// a denominator and a period. Anything else is "não estimado" with a reason.
// Nothing here queries the audited database.
package impact

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	StatusEstimated    = "estimated"
	StatusNotEstimated = "not_estimated"
	LevelHigh          = "alto"
	LevelMedium        = "médio"
	LevelLow           = "baixo"
	LevelNone          = "não estimado"
)

// BasisKey is the evidence sub-object that carries denominators and windows.
// It is excluded from the discard signature because it changes every run.
const BasisKey = "impact_basis"

// Estimate is the uniform impact schema (AN-03): value, unit, denominator,
// ratio, period and the level derived from them.
type Estimate struct {
	Status          string     `json:"status"`
	Metric          string     `json:"metric,omitempty"`
	Value           *float64   `json:"value,omitempty"`
	Unit            string     `json:"unit,omitempty"`
	Denominator     *float64   `json:"denominator,omitempty"`
	DenominatorUnit string     `json:"denominator_unit,omitempty"`
	Ratio           *float64   `json:"ratio,omitempty"`
	PeriodStart     *time.Time `json:"period_start,omitempty"`
	PeriodEnd       *time.Time `json:"period_end,omitempty"`
	Period          string     `json:"period"`
	Level           string     `json:"level"`
	Basis           string     `json:"basis"`
	Reason          string     `json:"reason,omitempty"`
	Label           string     `json:"label"`
}

// Estimated reports whether the estimate carries a measured number.
func (e Estimate) Estimated() bool { return e.Status == StatusEstimated }

type thresholds struct{ high, medium float64 }

func notEstimated(reason string) Estimate {
	return Estimate{Status: StatusNotEstimated, Level: LevelNone, Period: "não informado", Reason: reason,
		Basis: "Sem valor, denominador e período comparáveis.", Label: "impacto não estimado: " + reason}
}

// For estimates the impact of one finding. observedAt is the collection
// instant of the finding (last_seen_at) and firstSeen its first observation;
// both bound windows the evidence does not carry itself.
func For(rule string, evidence map[string]any, firstSeen, observedAt time.Time) Estimate {
	basis, _ := evidence[BasisKey].(map[string]any)
	if basis == nil {
		basis = map[string]any{}
	}
	switch rule {
	case "index.unused":
		size, ok1 := number(basis["size_bytes"])
		total, ok2 := number(basis["table_index_bytes"])
		if !ok1 || !ok2 || total <= 0 {
			return notEstimated("faltam os bytes do índice e o total de índices da tabela")
		}
		start, okStart := when(basis["window_start"])
		end, okEnd := when(basis["window_end"])
		if !okStart || !okEnd {
			if firstSeen.IsZero() || observedAt.IsZero() || !observedAt.After(firstSeen) {
				return notEstimated("falta a janela de idx_scan entre duas coletas")
			}
			start, end = firstSeen, observedAt
		}
		if !firstSeen.IsZero() && firstSeen.Before(start) {
			start = firstSeen
		}
		return build("index_bytes_without_scans", size, "bytes", total, "bytes de todos os índices da tabela", start, end,
			thresholds{0.3, 0.1}, "Bytes do índice sem idx_scan na janela, sobre o total de índices da tabela.")
	case "vacuum.high_dead_tuples", "maintenance.dead_tuple_pressure", "timescale.chunk_dead_tuples":
		dead, ok1 := firstNumber(evidence, "n_dead_tup", "dead_tuples")
		live, ok2 := firstNumber(evidence, "n_live_tup", "live_tuples")
		if !ok1 || !ok2 || dead+live <= 0 {
			return notEstimated("faltam n_dead_tup e n_live_tup")
		}
		start, ok := latest(evidence["last_vacuum"], evidence["last_autovacuum"], evidence["last_automatic_maintenance"], basis["last_vacuum"], basis["last_autovacuum"])
		if !ok || observedAt.IsZero() || !observedAt.After(start) {
			return notEstimated("sem o instante do último (auto)vacuum, a janela de acúmulo é desconhecida")
		}
		return build("dead_tuples_estimate", dead, "tuplas mortas (estimativa)", dead+live, "tuplas vivas + mortas", start, observedAt,
			thresholds{0.5, 0.2}, "Estimativa de pg_stat_user_tables desde o último (auto)vacuum; não é bloat medido.")
	case "performance.workload_scan":
		read, ok1 := number(evidence["shared_blocks_read"])
		hit, ok2 := number(evidence["shared_blocks_hit"])
		start, okStart := when(evidence["observation_window_start"])
		end, okEnd := when(evidence["observation_window_end"])
		if !ok1 || !ok2 || read+hit <= 0 {
			return notEstimated("faltam blocos lidos e acessados do fingerprint")
		}
		if !okStart || !okEnd || !end.After(start) {
			return notEstimated("a janela de pg_stat_statements é desconhecida (stats_reset não coletado)")
		}
		return build("shared_blocks_read", read, "blocos de 8 KiB lidos fora do cache", read+hit, "blocos acessados", start, end,
			thresholds{0.8, 0.5}, "shared_blks_read sobre shared_blks_read + shared_blks_hit desde stats_reset.")
	case "storage.large_table", "storage.top_consumer":
		size, ok1 := number(evidence["size_bytes"])
		total, ok2 := number(basis["database_size_bytes"])
		if !ok1 || !ok2 || total <= 0 {
			return notEstimated("falta o tamanho total do banco na mesma coleta")
		}
		at, ok := when(basis["collected_at"])
		if !ok {
			if observedAt.IsZero() {
				return notEstimated("falta o instante da coleta")
			}
			at = observedAt
		}
		return build("relation_bytes", size, "bytes", total, "bytes de todas as tabelas do banco", at, at,
			thresholds{0.3, 0.1}, "pg_total_relation_size da relação sobre a soma das tabelas do banco na mesma coleta.")
	case "maintenance.growth_trend":
		if coverage, _ := evidence["coverage"].(string); coverage != "complete" {
			return notEstimated("sem duas coletas completas e comparáveis")
		}
		delta, ok1 := number(evidence["delta_bytes"])
		previous, ok2 := number(evidence["previous_size_bytes"])
		start, okStart := when(evidence["period_start"])
		end, okEnd := when(evidence["period_end"])
		if !ok1 || !ok2 || previous <= 0 || !okStart || !okEnd || !end.After(start) {
			return notEstimated("faltam tamanhos ou período das duas coletas")
		}
		return build("size_delta_bytes", delta, "bytes", previous, "bytes na coleta anterior", start, end,
			thresholds{0.5, 0.2}, "Crescimento entre duas coletas completas sobre o tamanho anterior.")
	}
	return notEstimated("a regra não coleta métrica com denominador e período")
}

func build(metric string, value float64, unit string, denominator float64, denominatorUnit string, start, end time.Time, t thresholds, basis string) Estimate {
	ratio := value / denominator
	level := LevelLow
	if ratio >= t.high {
		level = LevelHigh
	} else if ratio >= t.medium {
		level = LevelMedium
	}
	s, e := start.UTC(), end.UTC()
	e1 := Estimate{Status: StatusEstimated, Metric: metric, Value: &value, Unit: unit, Denominator: &denominator, DenominatorUnit: denominatorUnit,
		Ratio: &ratio, PeriodStart: &s, PeriodEnd: &e, Period: PeriodLabel(s, e), Level: level, Basis: basis}
	e1.Label = fmt.Sprintf("%s %s de %s %s (%.1f%%) · período %s · impacto %s", formatNumber(value), unit, formatNumber(denominator), denominatorUnit, ratio*100, e1.Period, level)
	return e1
}

// PeriodLabel renders a UTC period; a single instant is a point-in-time collection.
func PeriodLabel(start, end time.Time) string {
	const layout = "02/01/2006 15:04 UTC"
	if start.Equal(end) {
		return "coleta de " + end.UTC().Format(layout)
	}
	return start.UTC().Format(layout) + " a " + end.UTC().Format(layout)
}

func formatNumber(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func firstNumber(m map[string]any, keys ...string) (float64, bool) {
	for _, k := range keys {
		if v, ok := number(m[k]); ok {
			return v, true
		}
	}
	return 0, false
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, !math.IsNaN(n) && !math.IsInf(n, 0)
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

func when(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, !t.IsZero()
	case *time.Time:
		if t == nil {
			return time.Time{}, false
		}
		return *t, !t.IsZero()
	case string:
		s := strings.TrimSpace(t)
		if s == "" || s == "unknown" {
			return time.Time{}, false
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999-07", "2006-01-02 15:04:05.999999-07:00", "2006-01-02 15:04:05-07"} {
			if parsed, err := time.Parse(layout, s); err == nil {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}

func latest(values ...any) (time.Time, bool) {
	var best time.Time
	found := false
	for _, v := range values {
		if t, ok := when(v); ok && (!found || t.After(best)) {
			best, found = t, true
		}
	}
	return best, found
}
