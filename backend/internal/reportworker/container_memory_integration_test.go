package reportworker

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/osmendes/db-auditor/internal/report"
	"github.com/osmendes/db-auditor/internal/repository"
)

// readCgroupValue reads a memory file of the cgroup this process runs in
// (cgroup v2, from /proc/self/cgroup), or its cgroup v1 equivalent. Inside a
// container with a private cgroup namespace that is the container's cgroup.
func readCgroupValue(v2, v1 string) (string, bool) {
	dirs := []string{}
	if raw, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			if path, found := strings.CutPrefix(line, "0::"); found {
				dirs = append(dirs, filepath.Join("/sys/fs/cgroup", path))
			}
		}
	}
	dirs = append(dirs, "/sys/fs/cgroup")
	names := []string{}
	for _, dir := range dirs {
		names = append(names, filepath.Join(dir, v2))
	}
	names = append(names, filepath.Join("/sys/fs/cgroup/memory", v1))
	for _, name := range names {
		if raw, err := os.ReadFile(name); err == nil {
			return strings.TrimSpace(string(raw)), true
		}
	}
	return "", false
}

// TestLargeReportUnderContainerMemoryLimit runs inside a memory-limited
// container (CI: `docker run --memory --memory-swap`, see the "Report under a
// container memory limit" step). It refuses to run without a real cgroup
// limit and without swap disabled, renders the largest report the limits
// allow plus one that hits the page limit with the largest finding section,
// and proves both finish inside the container limit: the job succeeds or fails
// with its readable reason, the process is not OOM-killed, and the cgroup
// memory peak stays below the limit.
func TestLargeReportUnderContainerMemoryLimit(t *testing.T) {
	if os.Getenv("AUDITOR_TEST_CONTAINER_MEMORY") != "1" {
		t.Skip("runs only inside the memory-limited CI container (AUDITOR_TEST_CONTAINER_MEMORY=1)")
	}
	rawLimit, ok := readCgroupValue("memory.max", "memory.limit_in_bytes")
	limit, err := strconv.ParseInt(rawLimit, 10, 64)
	if !ok || err != nil || limit <= 0 || limit > 1<<40 {
		t.Fatalf("no real cgroup memory limit is in effect (memory.max=%q)", rawLimit)
	}
	if expected, err := strconv.ParseInt(os.Getenv("AUDITOR_TEST_MEMORY_LIMIT_BYTES"), 10, 64); err != nil || limit != expected {
		t.Fatalf("cgroup limit %d does not match the requested container limit %q", limit, os.Getenv("AUDITOR_TEST_MEMORY_LIMIT_BYTES"))
	}
	if swap, ok := readCgroupValue("memory.swap.max", "memory.memsw.limit_in_bytes"); ok && swap != "0" && swap != rawLimit {
		t.Fatalf("swap is available to the container (memory.swap.max=%s); the limit would not be real", swap)
	}
	ctx, pool, store := openReportTestStore(t)
	// The largest report the limits allow: 501 databases and 5001 tables (both
	// truncated) plus findings, rendered close to the 200-page ceiling.
	env, run := seedReportRun(ctx, t, pool, 501, 5001, 200)
	large, err := store.CreateReportJob(ctx, repository.ReportRequest{EnvironmentID: env, AuditRunID: run, Type: "technical", IdempotencyKey: "container-large"})
	if err != nil {
		t.Fatal(err)
	}
	if err = (Worker{Store: store}).ProcessOne(ctx); err != nil {
		t.Fatalf("large report failed inside the container: %v", err)
	}
	finished, err := store.GetReportJob(ctx, env, large.ID)
	if err != nil || finished.Status != "success" || finished.SizeBytes == nil {
		t.Fatalf("large report: %#v %v", finished, err)
	}
	artifact, err := store.GetReportArtifact(ctx, env, large.ID)
	if err != nil || artifact == nil || report.ValidatePDF(artifact.Content) != nil {
		t.Fatalf("large artifact invalid: %v", err)
	}
	pages := report.PageCount(artifact.Content)
	if pages < 150 {
		t.Fatalf("fixture is not near the page ceiling: %d pages", pages)
	}
	artifact = nil
	// The largest finding section (2000 findings, truncated) exceeds the page
	// ceiling: the job must fail with the readable reason, not be OOM-killed.
	overEnv, overRun := seedReportRun(ctx, t, pool, 1, 10, 2001)
	over, err := store.CreateReportJob(ctx, repository.ReportRequest{EnvironmentID: overEnv, AuditRunID: overRun, Type: "technical", IdempotencyKey: "container-over"})
	if err != nil {
		t.Fatal(err)
	}
	err = (Worker{Store: store}).ProcessOne(ctx)
	if err == nil || !strings.Contains(err.Error(), "páginas; reduza o escopo") {
		t.Fatalf("expected the page limit reason, got %v", err)
	}
	failed, err := store.GetReportJob(ctx, overEnv, over.ID)
	if err != nil || failed.Status != "failed" || failed.Error == nil || !strings.Contains(*failed.Error, "páginas") {
		t.Fatalf("over-limit job: %#v %v", failed, err)
	}
	peak := int64(-1)
	if raw, ok := readCgroupValue("memory.peak", "memory.max_usage_in_bytes"); ok {
		if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
			peak = value
		}
	}
	if peak >= limit {
		t.Fatalf("container memory peak %d reached the limit %d", peak, limit)
	}
	t.Logf("CONTAINER_MEMORY_RESULT limit_bytes=%d peak_bytes=%d large_report_bytes=%d large_report_pages=%d over_limit_job=failed_readable", limit, peak, *finished.SizeBytes, pages)
}
