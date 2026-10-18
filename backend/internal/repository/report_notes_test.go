package repository

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/osmendes/db-auditor/internal/report"
)

// #107: QA found "Limite:" lines without accents ("Execucao parcial;
// ausencia ... nao e prova de resolucao") and English words in them.
var unaccentedPortuguese = regexp.MustCompile(`(?i)\b(execucao|ausencia|nao|resolucao|relatorio|findings)\b`)

func TestReportLimitNotesArePortuguese(t *testing.T) {
	notes := map[string]string{
		"partial run":     reportPartialRunNote,
		"truncated lists": reportTruncatedListsNotes,
		"detail limit":    fmt.Sprintf(reportDetailLimitNote, report.MaxDetailedFindings),
	}
	for name, note := range notes {
		if unaccentedPortuguese.MatchString(note) {
			t.Errorf("%s note is not accented Portuguese: %q", name, note)
		}
	}
	if reportPartialRunNote != "Execução parcial; ausência de objetos/achados não é prova de resolução." {
		t.Fatalf("partial run note changed: %q", reportPartialRunNote)
	}
}
