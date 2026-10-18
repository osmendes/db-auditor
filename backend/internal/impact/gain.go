package impact

import (
	"fmt"
	"strings"
)

// Measurement is a before/after observation recorded by an operator after an
// external change (finding_action_measurement).
type Measurement struct {
	Metric     string
	Before     *int64
	After      *int64
	Comparable bool
	AfterRunID string
	Note       string
}

// Gain separates the three statements BA-21 requires the UI and PDF to keep
// apart: the hypothesis, the predicted gain and the gain actually measured
// after a new complete collection.
type Gain struct {
	Hypothesis     string `json:"hypothesis"`
	Predicted      string `json:"predicted"`
	Measured       string `json:"measured"`
	MeasuredStatus string `json:"measured_status"`
}

const (
	MeasuredNone          = "not_measured"
	MeasuredDone          = "measured"
	MeasuredNotComparable = "not_comparable"
)

// BuildGain never turns a prediction into a measurement. Without a comparable
// measurement the measured gain reads "não medido".
func BuildGain(meaning string, evidence map[string]any, estimate Estimate, m *Measurement) Gain {
	g := Gain{Hypothesis: strings.TrimSpace(meaning) + " Hipótese até confirmação humana.",
		Predicted: "não estimado: teste a mudança em homologação antes de prever ganho.",
		Measured:  "não medido: exige nova coleta completa e comparável depois da ação externa.", MeasuredStatus: MeasuredNone}
	if h, ok := evidence["hypothesis"].(string); ok && strings.TrimSpace(h) != "" {
		g.Hypothesis = h
	}
	if p, ok := evidence["expected_gain"].(string); ok && strings.TrimSpace(p) != "" {
		g.Predicted = p
	} else if estimate.Estimated() {
		g.Predicted = "no máximo a condição observada (" + estimate.Label + "); o ganho real depende da ação e só é conhecido após nova coleta."
	}
	if m == nil {
		return g
	}
	if !m.Comparable || m.Before == nil || m.After == nil {
		g.MeasuredStatus = MeasuredNotComparable
		note := strings.TrimSpace(m.Note)
		if note == "" {
			note = "coletas ou carga não comparáveis"
		}
		g.Measured = "não comparável: " + note
		return g
	}
	g.MeasuredStatus = MeasuredDone
	unit := "unidades"
	if m.Metric == "table_size_bytes" {
		unit = "bytes"
	}
	g.Measured = fmt.Sprintf("%s: antes %d, depois %d %s (variação %+d) na execução %s", m.Metric, *m.Before, *m.After, unit, *m.After-*m.Before, m.AfterRunID)
	return g
}
