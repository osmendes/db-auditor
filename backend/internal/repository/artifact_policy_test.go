package repository

import "strings"
import "testing"

func TestExpiredArtifactDoesNotDeleteJobTrail(t *testing.T) {
	if strings.Contains(deleteExpiredArtifactsSQL, "DELETE FROM report_job") {
		t.Fatal("expiração do PDF não pode apagar a trilha do job")
	}
	if !strings.Contains(deleteExpiredArtifactsSQL, "DELETE FROM report_artifact") {
		t.Fatal("expiração deve remover somente o artefato")
	}
	if !strings.Contains(deleteExpiredArtifactsSQL, "expires_at<=now()") {
		t.Fatal("expiração precisa respeitar o prazo do job")
	}
}

func TestInterruptedReportRestartsUntilAttemptLimit(t *testing.T) {
	if !strings.Contains(requeueInterruptedReportsSQL, "attempts<3") || !strings.Contains(requeueInterruptedReportsSQL, "'queued'") {
		t.Fatal("reinício deve reenfileirar jobs interrompidos")
	}
	if !strings.Contains(requeueInterruptedReportsSQL, "'failed'") || !strings.Contains(requeueInterruptedReportsSQL, "worker interrupted") {
		t.Fatal("reinício deve falhar de forma legível após três tentativas")
	}
	if strings.Contains(requeueInterruptedReportsSQL, "DELETE FROM") {
		t.Fatal("reinício não remove histórico")
	}
}
