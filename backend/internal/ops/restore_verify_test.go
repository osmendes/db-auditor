package ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

var verifyScript = filepath.Join("..", "..", "..", "deploy", "snapshot-restore-verify.sh")

type drillEvidence struct {
	Kind        string          `json:"kind"`
	Label       string          `json:"environment_label"`
	SHA         string          `json:"artifact_sha256"`
	Succeeded   bool            `json:"succeeded"`
	RTO         *int            `json:"rto_seconds"`
	Integrity   json.RawMessage `json:"integrity"`
	Note        string          `json:"note"`
	PerformedAt time.Time       `json:"performed_at"`
}

const fakeCounts = `{"audit_run" : 3, "report_job" : 2, "auditor_user_roles" : {"operator" : 1}}`

func runVerify(t *testing.T, dir string, env []string, args ...string) (drillEvidence, string, error) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{verifyScript}, args...)...)
	cmd.Env = append(append(os.Environ(), "PATH="+dir+":/usr/bin:/bin"), env...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var ev drillEvidence
	if strings.TrimSpace(stdout.String()) != "" {
		if jerr := json.Unmarshal([]byte(stdout.String()), &ev); jerr != nil {
			t.Fatalf("evidence is not JSON: %v\n%s", jerr, stdout.String())
		}
	}
	return ev, stderr.String(), err
}

func fakeTools(t *testing.T, restoreExit, listExit int) (string, string) {
	t.Helper()
	dir := t.TempDir()
	writeFake(t, dir, "pg_restore", fmt.Sprintf("#!/bin/sh\nfor a in \"$@\"; do [ \"$a\" = --list ] && exit %d; done\nexit %d\n", listExit, restoreExit))
	writeFake(t, dir, "psql", "#!/bin/sh\necho '"+fakeCounts+"'\n")
	dump := filepath.Join(dir, "store.dump")
	if err := os.WriteFile(dump, []byte("PGDMP fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, dump
}

var isolated = []string{"RESTORE_TARGET_CONFIRM=isolated", "ENV_LABEL=staging"}

func TestRestoreVerifyProducesEvidence(t *testing.T) {
	dir, dump := fakeTools(t, 0, 0)
	expected := filepath.Join(dir, "source-counts.json")
	if err := os.WriteFile(expected, []byte(fakeCounts+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ev, stderr, err := runVerify(t, dir, isolated, "verify", dump, "postgresql://restore-target/auditor", expected)
	if err != nil || !ev.Succeeded || ev.Kind != "restore_drill" || ev.Label != "staging" || ev.RTO == nil || ev.PerformedAt.IsZero() {
		t.Fatalf("drill evidence: %+v %v %s", ev, err, stderr)
	}
	sum := sha256.Sum256([]byte("PGDMP fake"))
	if ev.SHA != hex.EncodeToString(sum[:]) || !strings.Contains(string(ev.Integrity), `"report_job"`) {
		t.Fatalf("evidence must carry dump sha256 and integrity counts: %+v", ev)
	}
}

func TestRestoreVerifyFailures(t *testing.T) {
	t.Run("restauração falha", func(t *testing.T) {
		dir, dump := fakeTools(t, 1, 0)
		ev, stderr, err := runVerify(t, dir, isolated, "verify", dump, "postgresql://restore-target/auditor")
		if err == nil || ev.Succeeded || !strings.Contains(stderr, "restauração falhou") {
			t.Fatalf("restore failure must fail with evidence succeeded=false: %+v %v %s", ev, err, stderr)
		}
	})
	t.Run("arquivo ilegível", func(t *testing.T) {
		dir, dump := fakeTools(t, 0, 1)
		ev, stderr, err := runVerify(t, dir, isolated, "verify", dump, "postgresql://restore-target/auditor")
		if err == nil || ev.Succeeded || !strings.Contains(stderr, "ilegível") {
			t.Fatalf("unreadable archive: %+v %v %s", ev, err, stderr)
		}
	})
	t.Run("sha256 divergente", func(t *testing.T) {
		dir, dump := fakeTools(t, 0, 0)
		if err := os.WriteFile(dump+".sha256", []byte(strings.Repeat("0", 64)+"  store.dump\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		ev, _, err := runVerify(t, dir, isolated, "verify", dump, "postgresql://restore-target/auditor")
		if err == nil || ev.Succeeded {
			t.Fatalf("checksum mismatch accepted: %+v", ev)
		}
	})
	t.Run("contagens divergentes", func(t *testing.T) {
		dir, dump := fakeTools(t, 0, 0)
		expected := filepath.Join(dir, "source-counts.json")
		if err := os.WriteFile(expected, []byte(`{"audit_run" : 4}`), 0o600); err != nil {
			t.Fatal(err)
		}
		ev, stderr, err := runVerify(t, dir, isolated, "verify", dump, "postgresql://restore-target/auditor", expected)
		if err == nil || ev.Succeeded || !strings.Contains(stderr, "diferem") {
			t.Fatalf("count mismatch accepted: %+v %s", ev, stderr)
		}
	})
	t.Run("destino não confirmado como isolado", func(t *testing.T) {
		dir, dump := fakeTools(t, 0, 0)
		if _, stderr, err := runVerify(t, dir, []string{"ENV_LABEL=staging"}, "verify", dump, "postgresql://x/auditor"); err == nil || !strings.Contains(stderr, "isolated") {
			t.Fatalf("unconfirmed target accepted: %s", stderr)
		}
	})
	t.Run("destino igual à origem", func(t *testing.T) {
		dir, dump := fakeTools(t, 0, 0)
		env := append([]string{"SOURCE_DSN=postgresql://prod/auditor"}, isolated...)
		if _, stderr, err := runVerify(t, dir, env, "verify", dump, "postgresql://prod/auditor"); err == nil || !strings.Contains(stderr, "origem") {
			t.Fatalf("restore over the source accepted: %s", stderr)
		}
	})
}

func toolMajor(t *testing.T, tool string) int {
	out, err := exec.Command(tool, "--version").Output()
	if err != nil {
		return 0
	}
	m := regexp.MustCompile(`(\d+)(\.\d+)?`).FindStringSubmatch(string(out))
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// A real round trip on the disposable cluster: a frozen copy of the store is
// dumped, restored into a second database by the drill script, and the
// restored counts (snapshots, jobs, users, roles and grants) must match.
func TestRestoreDrillRoundTripIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("requires disposable database")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var serverNum int
	if err = conn.QueryRow(ctx, `SELECT current_setting('server_version_num')::int`).Scan(&serverNum); err != nil {
		t.Fatal(err)
	}
	if major := toolMajor(t, "pg_dump"); major == 0 || major < serverNum/10000 || toolMajor(t, "pg_restore") < serverNum/10000 {
		t.Skipf("pg_dump/pg_restore %d older than server %d", major, serverNum/10000)
	}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	frozen, target := "dba_drill_src_"+suffix, "dba_drill_dst_"+suffix
	for _, name := range []string{frozen, target} {
		if _, err = conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
			t.Skipf("cannot create drill database: %v", err)
		}
		name := name
		t.Cleanup(func() { _, _ = conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)") })
	}
	withDB := func(name string) string {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		u.Path = "/" + name
		return u.String()
	}
	dir := t.TempDir()
	live := filepath.Join(dir, "live.dump")
	if out, err := exec.Command("pg_dump", "-Fc", "-f", live, dsn).CombinedOutput(); err != nil {
		t.Fatalf("pg_dump: %v %s", err, out)
	}
	if out, err := exec.Command("pg_restore", "--no-owner", "-d", withDB(frozen), live).CombinedOutput(); err != nil {
		t.Fatalf("freeze copy: %v %s", err, out)
	}
	dump := filepath.Join(dir, "frozen.dump")
	if out, err := exec.Command("pg_dump", "-Fc", "-f", dump, withDB(frozen)).CombinedOutput(); err != nil {
		t.Fatalf("pg_dump frozen: %v %s", err, out)
	}
	counts, err := exec.Command("sh", verifyScript, "counts", withDB(frozen)).Output()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(dir, "source-counts.json")
	if err = os.WriteFile(expected, counts, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", verifyScript, "verify", dump, withDB(target), expected)
	cmd.Env = append(os.Environ(), "RESTORE_TARGET_CONFIRM=isolated", "ENV_LABEL=ci-drill", "SOURCE_DSN="+withDB(frozen))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("drill failed: %v %s", err, out)
	}
	var ev drillEvidence
	if err = json.Unmarshal(out, &ev); err != nil || !ev.Succeeded || ev.RTO == nil {
		t.Fatalf("evidence: %s %v", out, err)
	}
	var integrity map[string]any
	if err = json.Unmarshal(ev.Integrity, &integrity); err != nil || integrity["audit_run"] == nil || integrity["auditor_user_roles"] == nil {
		t.Fatalf("integrity counts: %s", ev.Integrity)
	}
}
