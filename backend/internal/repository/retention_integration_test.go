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
	defer pool.Close()
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
	if _, err = store.UpdateRetentionPolicy(ctx, RetentionPolicyInput{SnapshotRetentionDays: 365, ReportJobRetentionDays: 90, OwnerName: "Maycon", EncryptionAtRest: "LUKS no volume + backup cifrado"}, "operator"); err != nil {
		t.Fatal(err)
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
	if err != nil || removed != plan.ReportJobsEligible {
		t.Fatalf("apply: %d %v", removed, err)
	}
	var oldLeft, recentLeft, runs int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM report_job WHERE id=$1::uuid),(SELECT count(*) FROM report_job WHERE id=$2::uuid),(SELECT count(*) FROM audit_run WHERE id=$3::uuid)`, oldJob, recentJob, run).Scan(&oldLeft, &recentLeft, &runs); err != nil {
		t.Fatal(err)
	}
	if oldLeft != 0 || recentLeft != 1 || runs != 1 {
		t.Fatalf("only the expired trail beyond the period may go; snapshots stay: old=%d recent=%d runs=%d", oldLeft, recentLeft, runs)
	}
	// Editing the policy drops the approval again.
	if _, err = store.UpdateRetentionPolicy(ctx, RetentionPolicyInput{SnapshotRetentionDays: 0, ReportJobRetentionDays: 120, OwnerName: "Maycon", EncryptionAtRest: "LUKS"}, "operator"); err != nil {
		t.Fatal(err)
	}
	if plan, err = store.PlanRetention(ctx); err != nil || plan.CleanupAllowed || plan.SnapshotRunsEligible != 0 {
		t.Fatalf("edited policy must be unapproved and 0 days keeps every snapshot: %+v %v", plan, err)
	}
}
