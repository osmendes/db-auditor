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
	backupDir := filepath.Join(dir, "backup")
	cmd.Env = append(os.Environ(), "PATH="+dir+":/usr/bin:/bin", "PSQL_LOG="+logPath, "BACKUP_DIR="+backupDir)
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

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

// #129: the dump carries password hashes, TOTP secrets and history, so the
// directory must be 0700 and every artifact 0600, whatever the caller umask
// and whatever mode older dumps were left with.
func TestBackupArtifactsArePrivate(t *testing.T) {
	dir := t.TempDir()
	writeFake(t, dir, "pg_dump", "#!/bin/sh\nwhile [ $# -gt 0 ]; do [ \"$1\" = -f ] && { echo dump > \"$2\"; exit 0; }; shift; done\nexit 1\n")
	writeFake(t, dir, "psql", "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$PSQL_LOG\"\nexit 0\n")
	backupDir := filepath.Join(dir, "backup")
	if err := os.Mkdir(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(backupDir, "db-auditor-20200101T000000Z.dump")
	if err := os.WriteFile(legacy, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join("..", "..", "..", "deploy", "snapshot-backup.sh")
	// umask 022 is the usual default that produced 0644 dumps.
	cmd := exec.Command("sh", "-c", "umask 022; exec sh \"$0\"", script)
	cmd.Env = append(os.Environ(), "PATH="+dir+":/usr/bin:/bin", "PSQL_LOG="+filepath.Join(dir, "psql.log"),
		"BACKUP_DIR="+backupDir, "BACKUP_RETENTION_DAYS=100000")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("backup falhou: %v %s", err, out)
	}
	if got := mode(t, backupDir); got != 0o700 {
		t.Fatalf("diretório de backup com modo %o, esperado 700", got)
	}
	dumps, err := filepath.Glob(filepath.Join(backupDir, "db-auditor-*"))
	if err != nil {
		t.Fatal(err)
	}
	var fresh, sums int
	for _, path := range dumps {
		if got := mode(t, path); got != 0o600 {
			t.Fatalf("%s com modo %o, esperado 600", filepath.Base(path), got)
		}
		if strings.HasSuffix(path, ".sha256") {
			sums++
		} else if path != legacy {
			fresh++
		}
	}
	if fresh != 1 || sums != 1 {
		t.Fatalf("esperado um dump novo e um checksum, achei %v", dumps)
	}
}
