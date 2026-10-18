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
	// pg_dump writes part of the dump before failing, as it does on a lost
	// connection or a full disk; the partial file must not stay behind.
	writeFake(t, dir, "pg_dump", "#!/bin/sh\nwhile [ $# -gt 0 ]; do [ \"$1\" = -f ] && echo partial > \"$2\"; shift; done\necho dump-failed >&2\nexit 1\n")
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
	left, err := filepath.Glob(filepath.Join(backupDir, "db-auditor-*"))
	if err != nil || len(left) != 0 {
		t.Fatalf("dump parcial ficou depois da falha: %v %v", left, err)
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
	// The fake records the mode pg_dump's file is born with, before the
	// script's chmod: only the umask protects the dump while it is written.
	writeFake(t, dir, "pg_dump", "#!/bin/sh\nwhile [ $# -gt 0 ]; do [ \"$1\" = -f ] && { echo dump > \"$2\"; stat -c %a \"$2\" > \"$BORN_MODE\"; exit 0; }; shift; done\nexit 1\n")
	writeFake(t, dir, "psql", "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$PSQL_LOG\"\nexit 0\n")
	backupDir := filepath.Join(dir, "backup")
	if err := os.Mkdir(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(backupDir, "db-auditor-20200101T000000Z.dump")
	legacySum := legacy + ".sha256"
	for _, path := range []string{legacy, legacySum} {
		if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bornMode := filepath.Join(dir, "born-mode")
	script := filepath.Join("..", "..", "..", "deploy", "snapshot-backup.sh")
	// umask 022 is the usual default that produced 0644 dumps.
	cmd := exec.Command("sh", "-c", "umask 022; exec sh \"$0\"", script)
	cmd.Env = append(os.Environ(), "PATH="+dir+":/usr/bin:/bin", "PSQL_LOG="+filepath.Join(dir, "psql.log"),
		"BACKUP_DIR="+backupDir, "BACKUP_RETENTION_DAYS=100000", "BORN_MODE="+bornMode)
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
		switch {
		case path == legacy || path == legacySum:
		case strings.HasSuffix(path, ".sha256"):
			sums++
		default:
			fresh++
		}
	}
	if born, err := os.ReadFile(bornMode); err != nil || strings.TrimSpace(string(born)) != "600" {
		t.Fatalf("dump nasceu com modo %q (umask do script), esperado 600: %v", born, err)
	}
	if fresh != 1 || sums != 1 {
		t.Fatalf("esperado um dump novo e um checksum, achei %v", dumps)
	}
}

// #129: a dump tool that sets its own file mode (here 0644, ignoring the
// umask) still ends with a 0600 dump: the script's chmod is not optional.
func TestBackupFixesModeSetByDumpTool(t *testing.T) {
	dir := t.TempDir()
	writeFake(t, dir, "pg_dump", "#!/bin/sh\nwhile [ $# -gt 0 ]; do [ \"$1\" = -f ] && { echo dump > \"$2\"; chmod 0644 \"$2\"; exit 0; }; shift; done\nexit 1\n")
	writeFake(t, dir, "psql", "#!/bin/sh\nexit 0\n")
	backupDir := filepath.Join(dir, "backup")
	script := filepath.Join("..", "..", "..", "deploy", "snapshot-backup.sh")
	cmd := exec.Command("sh", script)
	cmd.Env = append(os.Environ(), "PATH="+dir+":/usr/bin:/bin", "BACKUP_DIR="+backupDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("backup falhou: %v %s", err, out)
	}
	dumps, err := filepath.Glob(filepath.Join(backupDir, "db-auditor-*"))
	if err != nil || len(dumps) != 2 {
		t.Fatalf("esperado dump e checksum: %v %v", dumps, err)
	}
	for _, path := range dumps {
		if got := mode(t, path); got != 0o600 {
			t.Fatalf("%s com modo %o, esperado 600", filepath.Base(path), got)
		}
	}
}
