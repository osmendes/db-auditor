package ops

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeFake(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestBackupFailureIsRecorded(t *testing.T) {
	dir := t.TempDir()
	writeFake(t, dir, "pg_dump", "#!/bin/sh\necho dump-failed >&2\nexit 1\n")
	writeFake(t, dir, "psql", "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$PSQL_LOG\"\nexit 0\n")
	logPath := filepath.Join(dir, "psql.log")
	script := filepath.Join("..", "..", "..", "deploy", "snapshot-backup.sh")
	cmd := exec.Command("sh", script)
	cmd.Env = append(os.Environ(), "PATH="+dir+":/usr/bin:/bin", "PSQL_LOG="+logPath)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("backup deveria falhar: %s", out)
	}
	logged, readErr := os.ReadFile(logPath)
	if readErr != nil || !strings.Contains(string(logged), "last_status='failed'") {
		t.Fatalf("falha de backup sem registro: %s\nlog: %s", out, logged)
	}
}

func TestRestoreFailureStaysIsolated(t *testing.T) {
	dir := t.TempDir()
	writeFake(t, dir, "pg_restore", "#!/bin/sh\necho restore-failed >&2\nexit 1\n")
	script := filepath.Join("..", "..", "..", "deploy", "snapshot-restore.sh")
	cmd := exec.Command("sh", script, filepath.Join(dir, "missing.dump"))
	cmd.Env = append(os.Environ(), "PATH="+dir+":/usr/bin:/bin")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "restauração falhou") {
		t.Fatalf("falha de restauração pouco clara: %v %s", err, out)
	}
}
