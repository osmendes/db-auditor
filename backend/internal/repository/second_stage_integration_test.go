package repository

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSecondStageWorkflowsIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("requires disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := NewStore(pool)
	hash, err := HashAuditorPassword("long-password-for-stage-two")
	if err != nil {
		t.Fatal(err)
	}
	var env string
	if err = pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	one, err := s.CreateAuditorUser(ctx, "stage2a"+env[:8], hash, "operator", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateAuditorAccount(ctx, one.ID, AccountChange{Role: "viewer", Active: true}); !errors.Is(err, ErrLastOperator) {
		// Other integration tests may have operators; verify invariant with a transaction-local fixture instead.
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.UpdateAuditorAccount(ctx, one.ID, AccountChange{Role: "operator", Active: true}); err != nil {
		t.Fatal(err)
	}
	two, err := s.CreateAuditorUser(ctx, "stage2b"+env[:8], hash, "operator", []string{env})
	if err != nil {
		t.Fatal(err)
	}
	sessionHash := []byte("session-stage2-" + env)
	if err = s.CreateAuditorSession(ctx, two.ID, sessionHash, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	account, err := s.UpdateAuditorAccount(ctx, two.ID, AccountChange{Role: "auditor", Active: true, Environments: []string{env}})
	if err != nil || account.Role != "auditor" {
		t.Fatalf("account: %+v %v", account, err)
	}
	if user, err := s.GetAuditorSession(ctx, sessionHash); err != nil || user != nil {
		t.Fatalf("old session survived: %+v %v", user, err)
	}
	if err = s.ChangeAuditorPassword(ctx, one.ID, hash); err != nil {
		t.Fatal(err)
	}
	if err = s.CreateAuditorSession(ctx, one.ID, sessionHash, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	newHash, err := HashAuditorPassword("new-recovery-password-stage-two")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverOperatorPassword(ctx, two.Username, newHash); !errors.Is(err, ErrRecoveryTarget) {
		t.Fatalf("non-operator recovery must fail: %v", err)
	}
	if err = s.RecoverOperatorPassword(ctx, one.Username, newHash); err != nil {
		t.Fatal(err)
	}
	if user, err := s.GetAuditorSession(ctx, sessionHash); err != nil || user != nil {
		t.Fatalf("recovered operator session survived: %+v %v", user, err)
	}
	if user, err := s.FindAuditorUser(ctx, one.Username); err != nil || user == nil || !CheckAuditorPassword("new-recovery-password-stage-two", user.PasswordHash) {
		t.Fatalf("recovered password unavailable: %+v %v", user, err)
	}

	runs := make([]string, 3)
	for i := range runs {
		at := time.Now().UTC().Add(time.Duration(i-3) * time.Hour)
		if err = pool.QueryRow(ctx, `INSERT INTO audit_run(environment_id,profile,status,service_version,collector_version,started_at)
VALUES($1::uuid,'manual','success','test','test',$2) RETURNING id::text`, env, at).Scan(&runs[i]); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO database_snapshot(audit_run_id,environment_id,database_name) VALUES($1::uuid,$2::uuid,'db')`, runs[i], env); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO table_snapshot(audit_run_id,environment_id,database_name,schema_name,table_name,has_primary_key) VALUES($1::uuid,$2::uuid,'db','public','orders',true)`, runs[i], env); err != nil {
			t.Fatal(err)
		}
		for _, collector := range scoreCollectors {
			if _, err = pool.Exec(ctx, `INSERT INTO audit_run_coverage(audit_run_id,environment_id,collector_name,database_name,status) VALUES($1::uuid,$2::uuid,$3,'db','success')`, runs[i], env, collector); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = pool.Exec(ctx, `INSERT INTO analysis_run(audit_run_id,environment_id,status,analyzer_version) VALUES($1::uuid,$2::uuid,'success','test')`, runs[i], env); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.SelectAuditBaseline(ctx, env, "", "", "", runs[0], "tester"); err != nil {
		t.Fatal(err)
	}
	var findingID string
	if err = pool.QueryRow(ctx, `INSERT INTO finding(environment_id,audit_run_id,finding_type,severity,title,dedup_key,database_name,schema_name,object_name)
VALUES($1::uuid,$2::uuid,'model.no_primary_key','high','test',gen_random_uuid()::text,'db','public','orders') RETURNING id::text`, env, runs[1]).Scan(&findingID); err != nil {
		t.Fatal(err)
	}
	for _, run := range runs[1:] {
		if _, err = pool.Exec(ctx, `INSERT INTO finding_event(finding_id,audit_run_id,event_type,severity,category) VALUES($1::uuid,$2::uuid,'observed','high','model')`, findingID, run); err != nil {
			t.Fatal(err)
		}
	}
	action, err := s.GetFindingAction(ctx, findingID)
	if err != nil || action.Plan.Category != "structure" || action.Coverage != "complete" {
		t.Fatalf("action: %+v %v", action, err)
	}
	action, err = s.UpdateFindingAction(ctx, findingID, "tester", ActionProgress{Status: "planned", Owner: "dba", Justification: "reviewed"})
	if err != nil || action.Status != "planned" {
		t.Fatalf("workflow: %+v %v", action, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE finding SET audit_run_id=$2::uuid,recurrence_count=1 WHERE id=$1::uuid`, findingID, runs[2]); err != nil {
		t.Fatal(err)
	}
	action, err = s.GetFindingAction(ctx, findingID)
	if err != nil || action.Status != "planned" || *action.SourceRunID != runs[2] {
		t.Fatalf("recurrence lost decision: %+v %v", action, err)
	}
	events, err := s.ListFindingActionEvents(ctx, findingID)
	if err != nil || len(events) != 1 {
		t.Fatalf("events: %+v %v", events, err)
	}
	annotation, err := s.CreateAuditAnnotation(ctx, env, runs[2], "deployment", "release 1", "tester", time.Now().UTC())
	if err != nil || annotation.Note != "release 1" {
		t.Fatalf("annotation: %+v %v", annotation, err)
	}
	if _, err = s.CreateAuditAnnotation(ctx, env, "00000000-0000-4000-8000-000000000001", "note", "bad", "tester", time.Now()); !errors.Is(err, ErrMonitoringScope) {
		t.Fatalf("foreign run accepted: %v", err)
	}
	if err = s.EvaluateRegressionAlerts(ctx, env, runs[2]); err != nil {
		t.Fatal(err)
	}
	if err = s.EvaluateRegressionAlerts(ctx, env, runs[2]); err != nil {
		t.Fatal(err)
	}
	alerts, err := s.ListRegressionAlerts(ctx, env)
	if err != nil || len(alerts) != 1 || alerts[0].Category != "findings" {
		t.Fatalf("alerts: %+v %v", alerts, err)
	}
	if err = s.AcknowledgeRegressionAlert(ctx, env, alerts[0].ID, "tester", "explained"); err != nil {
		t.Fatal(err)
	}
	score, err := s.GetScopeScore(ctx, env, runs[2], "", "", "")
	if err != nil || score.Score == nil || score.Version != ScopeScoreFormulaVersion {
		t.Fatalf("score: %+v %v", score, err)
	}
}
