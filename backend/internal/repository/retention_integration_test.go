package repository

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const testSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestCleanupBlockersRequireApprovalBackupAndDrill(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	approved := now.Add(-time.Hour)
	policy := RetentionPolicy{ApprovedBy: "owner", ApprovedAt: &approved, UpdatedAt: approved}
	fresh := &RetentionEvidence{PerformedAt: now.Add(-24 * time.Hour)}
	if got := CleanupBlockers(policy, fresh, fresh, now); len(got) != 0 {
		t.Fatalf("complete evidence blocked: %v", got)
	}
	if got := CleanupBlockers(RetentionPolicy{}, nil, nil, now); len(got) != 3 {
		t.Fatalf("missing approval, backup and drill must all block: %v", got)
	}
	stale := &RetentionEvidence{PerformedAt: now.Add(-40 * 24 * time.Hour)}
	if got := CleanupBlockers(policy, stale, stale, now); len(got) != 2 {
		t.Fatalf("stale evidence must block: %v", got)
	}
	edited := policy
	edited.UpdatedAt = approved.Add(time.Hour)
	if got := CleanupBlockers(edited, fresh, fresh, now); len(got) != 1 || !strings.Contains(got[0], "alterada") {
		t.Fatalf("edited policy must need approval again: %v", got)
	}
	for _, bad := range []RetentionPolicyInput{
		{SnapshotRetentionDays: -1, ReportJobRetentionDays: 90, OwnerName: "o", EncryptionAtRest: "luks"},
		{ReportJobRetentionDays: 7, OwnerName: "o", EncryptionAtRest: "luks"},
		{ReportJobRetentionDays: 90, OwnerName: " ", EncryptionAtRest: "luks"},
		{ReportJobRetentionDays: 90, OwnerName: "o"},
	} {
		if ValidateRetentionInput(bad) == nil {
			t.Fatalf("invalid policy accepted: %+v", bad)
		}
	}
}

// BA-03/10/16: nothing is removed without approval, a verified backup and a
// restore drill; the dry run counts, the apply removes only expired job
// trails without artifact, and snapshots are never removed.
func TestRetentionGovernanceIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("requires disposable database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := NewStore(pool)
	if _, err = pool.Exec(ctx, `DELETE FROM retention_evidence WHERE recorded_by='retention-test'`); err != nil {
		t.Fatal(err)
	}
	original, err := store.GetRetentionPolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE retention_policy SET snapshot_retention_days=$1,report_job_retention_days=$2,owner_name=$3,encryption_at_rest=$4,archive_note=$5,approved_by=$6,approved_at=$7 WHERE id`,
			original.SnapshotRetentionDays, original.ReportJobRetentionDays, original.OwnerName, original.EncryptionAtRest, original.ArchiveNote, original.ApprovedBy, original.ApprovedAt)
		_, _ = pool.Exec(context.Background(), `DELETE FROM retention_evidence WHERE recorded_by='retention-test'`)
	})
	var env, run, oldJob, recentJob string
	if err = pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version,started_at) VALUES($1::uuid,'manual','success','test','test',now()-interval '400 days') RETURNING id::text`, env).Scan(&run); err != nil {
		t.Fatal(err)
	}
	insertJob := func(created string, key string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO report_job(environment_id,audit_run_id,report_type,filters,requested_by,rule_version,status,idempotency_key,created_at,expires_at,finished_at)
VALUES($1::uuid,$2::uuid,'executive','{}'::jsonb,'tester','v','success',$3,now()-$4::interval,now()-interval '1 day',now()-$4::interval) RETURNING id::text`, env, run, key, created).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	oldJob = insertJob("200 days", "retention-old-"+env)
	recentJob = insertJob("10 days", "retention-recent-"+env)
	// Fresh install state: values set, no approval on record.
	if _, err = pool.Exec(ctx, `UPDATE retention_policy SET snapshot_retention_days=365,report_job_retention_days=90,owner_name='Maycon',encryption_at_rest='LUKS',approved_by='',approved_at=NULL,updated_at=now() WHERE id`); err != nil {
		t.Fatal(err)
	}
	// R2 #5: a policy change without approval writes nothing.
	before, err := store.GetRetentionPolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpdateRetentionPolicy(ctx, RetentionPolicyInput{SnapshotRetentionDays: 30, ReportJobRetentionDays: 30, OwnerName: "Outro", EncryptionAtRest: "nenhuma"}, "operator"); !errors.Is(err, ErrRetentionInvalid) {
		t.Fatalf("unapproved update must be refused: %v", err)
	}
	if after, _ := store.GetRetentionPolicy(ctx); after.SnapshotRetentionDays != before.SnapshotRetentionDays || after.OwnerName != before.OwnerName || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("unapproved update changed the row: before=%+v after=%+v", before, after)
	}
	plan, err := store.PlanRetention(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if plan.CleanupAllowed || len(plan.Blockers) != 3 || plan.SnapshotRunsEligible < 1 || plan.ReportJobsEligible < 1 {
		t.Fatalf("unapproved plan: %+v", plan)
	}
	if _, err = store.ApplyReportJobRetention(ctx, plan.ReportJobsEligible); !errors.Is(err, ErrRetentionBlocked) {
		t.Fatalf("cleanup without approval/backup/drill must be refused: %v", err)
	}
	if _, err = store.RecordRetentionEvidence(ctx, RetentionEvidence{Kind: EvidenceBackupVerified, EnvironmentLabel: "staging", ArtifactSHA256: "bad", PerformedAt: time.Now(), RecordedBy: "retention-test"}); !errors.Is(err, ErrRetentionInvalid) {
		t.Fatalf("invalid evidence accepted: %v", err)
	}
	// A failed restore is recorded but never counts as a drill.
	rto := 420
	if _, err = store.RecordRetentionEvidence(ctx, RetentionEvidence{Kind: EvidenceRestoreDrill, EnvironmentLabel: "staging", ArtifactSHA256: testSHA, Succeeded: false, RTOSeconds: &rto, PerformedAt: time.Now().Add(-time.Hour), RecordedBy: "retention-test", Note: "pg_restore falhou"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpdateRetentionPolicy(ctx, RetentionPolicyInput{SnapshotRetentionDays: 365, ReportJobRetentionDays: 90, OwnerName: "Maycon", EncryptionAtRest: "LUKS no volume + backup cifrado", Approve: true}, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.RecordRetentionEvidence(ctx, RetentionEvidence{Kind: EvidenceBackupVerified, EnvironmentLabel: "staging", ArtifactSHA256: testSHA, Succeeded: true, PerformedAt: time.Now().Add(-time.Hour), RecordedBy: "retention-test"}); err != nil {
		t.Fatal(err)
	}
	plan, err = store.PlanRetention(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if plan.CleanupAllowed || len(plan.Blockers) != 1 || !strings.Contains(plan.Blockers[0], "restauração") {
		t.Fatalf("failed drill must still block: %+v", plan.Blockers)
	}
	integrity, _ := json.Marshal(map[string]int{"audit_run": 10, "report_job": 4, "auditor_user": 2})
	if _, err = store.RecordRetentionEvidence(ctx, RetentionEvidence{Kind: EvidenceRestoreDrill, EnvironmentLabel: "staging", ArtifactSHA256: testSHA, Succeeded: true, RTOSeconds: &rto, Integrity: integrity, PerformedAt: time.Now().Add(-30 * time.Minute), RecordedBy: "retention-test"}); err != nil {
		t.Fatal(err)
	}
	plan, err = store.PlanRetention(ctx)
	if err != nil || !plan.CleanupAllowed {
		t.Fatalf("approved plan with evidence: %+v %v", plan, err)
	}
	if _, err = store.ApplyReportJobRetention(ctx, plan.ReportJobsEligible+1); !errors.Is(err, ErrRetentionBlocked) {
		t.Fatalf("a count that differs from the plan must be refused: %v", err)
	}
	removed, err := store.ApplyReportJobRetention(ctx, plan.ReportJobsEligible)
	if err != nil || removed.ReportJobsRemoved != plan.ReportJobsEligible {
		t.Fatalf("apply: %+v %v", removed, err)
	}
	var oldLeft, recentLeft, runs int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM report_job WHERE id=$1::uuid),(SELECT count(*) FROM report_job WHERE id=$2::uuid),(SELECT count(*) FROM audit_run WHERE id=$3::uuid)`, oldJob, recentJob, run).Scan(&oldLeft, &recentLeft, &runs); err != nil {
		t.Fatal(err)
	}
	if oldLeft != 0 || recentLeft != 1 || runs != 1 {
		t.Fatalf("only the expired trail beyond the period may go; snapshots stay: old=%d recent=%d runs=%d", oldLeft, recentLeft, runs)
	}
	// An edit outside the API (no new approval) blocks the cleanup again.
	if _, err = pool.Exec(ctx, `UPDATE retention_policy SET snapshot_retention_days=0,updated_at=now()+interval '1 minute' WHERE id`); err != nil {
		t.Fatal(err)
	}
	if plan, err = store.PlanRetention(ctx); err != nil || plan.CleanupAllowed || plan.SnapshotRunsEligible != 0 {
		t.Fatalf("edited policy must be unapproved and 0 days keeps every snapshot: %+v %v", plan, err)
	}
}

// retentionFixture approves a 90-day job policy, records fresh evidence and
// returns an environment and run to hang jobs on. Everything is restored at
// cleanup.
func retentionFixture(t *testing.T) (context.Context, *pgxpool.Pool, *Store, string, string) {
	t.Helper()
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("requires disposable database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := NewStore(pool)
	original, err := store.GetRetentionPolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE retention_policy SET snapshot_retention_days=$1,report_job_retention_days=$2,owner_name=$3,encryption_at_rest=$4,archive_note=$5,approved_by=$6,approved_at=$7 WHERE id`,
			original.SnapshotRetentionDays, original.ReportJobRetentionDays, original.OwnerName, original.EncryptionAtRest, original.ArchiveNote, original.ApprovedBy, original.ApprovedAt)
		_, _ = pool.Exec(context.Background(), `DELETE FROM retention_evidence WHERE recorded_by='retention-test'`)
	})
	if _, err = store.UpdateRetentionPolicy(ctx, RetentionPolicyInput{SnapshotRetentionDays: 0, ReportJobRetentionDays: 90, OwnerName: "Maycon", EncryptionAtRest: "LUKS", Approve: true}, "owner"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{EvidenceBackupVerified, EvidenceRestoreDrill} {
		if _, err = store.RecordRetentionEvidence(ctx, RetentionEvidence{Kind: kind, EnvironmentLabel: "staging", ArtifactSHA256: testSHA, Succeeded: true, PerformedAt: time.Now().Add(-time.Hour), RecordedBy: "retention-test"}); err != nil {
			t.Fatal(err)
		}
	}
	var env, run string
	if err = pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version,started_at) VALUES($1::uuid,'manual','success','test','test',now()-interval '400 days') RETURNING id::text`, env).Scan(&run); err != nil {
		t.Fatal(err)
	}
	return ctx, pool, store, env, run
}

// R2 #1: only finished, old, expired job trails without artifact are removed.
// Each survivor differs from an eligible job in exactly one condition, so
// dropping any predicate from the shared filter deletes a survivor.
func TestRetentionCleanupBoundariesIntegration(t *testing.T) {
	ctx, pool, store, env, run := retentionFixture(t)
	insert := func(name, status, created, expires string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO report_job(environment_id,audit_run_id,report_type,filters,requested_by,rule_version,status,idempotency_key,created_at,expires_at)
VALUES($1::uuid,$2::uuid,'executive','{}'::jsonb,'tester','v',$3,$4,now()-$5::interval,now()+$6::interval) RETURNING id::text`, env, run, status, "retention-boundary-"+name+"-"+env, created, expires).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	eligible := map[string]string{
		"success":   insert("success", "success", "200 days", "-1 day"),
		"failed":    insert("failed", "failed", "200 days", "-1 day"),
		"cancelled": insert("cancelled", "cancelled", "200 days", "-1 day"),
	}
	survivors := map[string]string{
		"running":           insert("running", "running", "200 days", "-1 day"),
		"queued":            insert("queued", "queued", "200 days", "-1 day"),
		"not yet expired":   insert("not-expired", "success", "200 days", "1 day"),
		"inside the period": insert("recent", "success", "89 days", "-1 day"),
		"with artifact":     insert("artifact", "success", "200 days", "-1 day"),
	}
	if _, err := pool.Exec(ctx, `INSERT INTO report_artifact(report_job_id,content,sha256,size_bytes,filename,content_type) VALUES($1::uuid,'%PDF','x',4,'r.pdf','application/pdf')`, survivors["with artifact"]); err != nil {
		t.Fatal(err)
	}
	plan, err := store.PlanRetention(ctx)
	if err != nil || !plan.CleanupAllowed {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	// The dry run counts exactly the rows the shared filter selects.
	var mine int64
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM (`+eligibleReportJobsSQL+`) e JOIN report_job j USING (id) WHERE j.environment_id=$2::uuid`, plan.Policy.ReportJobRetentionDays, env).Scan(&mine); err != nil || mine != int64(len(eligible)) {
		t.Fatalf("eligible in this fixture: %d want %d (%v)", mine, len(eligible), err)
	}
	result, err := store.ApplyReportJobRetention(ctx, plan.ReportJobsEligible)
	if err != nil || result.ReportJobsRemoved != plan.ReportJobsEligible || result.Policy.ApprovedBy != "owner" {
		t.Fatalf("apply: %+v %v", result, err)
	}
	exists := func(id string) bool {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM report_job WHERE id=$1::uuid`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	for name, id := range eligible {
		if exists(id) {
			t.Errorf("eligible %s job was kept", name)
		}
	}
	for name, id := range survivors {
		if !exists(id) {
			t.Errorf("%s job was deleted", name)
		}
	}
	var artifacts int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM report_artifact WHERE report_job_id=$1::uuid`, survivors["with artifact"]).Scan(&artifacts); err != nil || artifacts != 1 {
		t.Fatalf("artifact must survive: %d %v", artifacts, err)
	}
}

// R2 #4: plan, checks and DELETE run in one transaction holding the policy
// row. While another transaction holds the row (an edit or revocation in
// flight), the cleanup waits instead of deleting under a stale plan; a
// revocation committed meanwhile is honoured and nothing is removed.
func TestRetentionApplyLocksPolicyIntegration(t *testing.T) {
	ctx, pool, store, env, run := retentionFixture(t)
	var job string
	if err := pool.QueryRow(ctx, `INSERT INTO report_job(environment_id,audit_run_id,report_type,filters,requested_by,rule_version,status,idempotency_key,created_at,expires_at)
VALUES($1::uuid,$2::uuid,'executive','{}'::jsonb,'tester','v','success',$3,now()-interval '200 days',now()-interval '1 day') RETURNING id::text`, env, run, "retention-lock-"+env).Scan(&job); err != nil {
		t.Fatal(err)
	}
	plan, err := store.PlanRetention(ctx)
	if err != nil || !plan.CleanupAllowed {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	editor, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = editor.Rollback(context.Background()) }()
	// Revoke the approval inside an open transaction.
	if _, err = editor.Exec(ctx, `UPDATE retention_policy SET approved_by='',approved_at=NULL,updated_at=now() WHERE id`); err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	_, err = store.ApplyReportJobRetention(waitCtx, plan.ReportJobsEligible)
	cancel()
	if err == nil {
		t.Fatal("apply must wait for the policy row held by another transaction")
	}
	done := make(chan error, 1)
	go func() {
		_, err := store.ApplyReportJobRetention(ctx, plan.ReportJobsEligible)
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)
	if err = editor.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, ErrRetentionBlocked) {
		t.Fatalf("apply after a committed revocation must be refused: %v", err)
	}
	var left int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM report_job WHERE id=$1::uuid`, job).Scan(&left); err != nil || left != 1 {
		t.Fatalf("nothing may be removed after the revocation: %d %v", left, err)
	}
}
