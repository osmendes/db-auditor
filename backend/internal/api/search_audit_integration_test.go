package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/osmendes/db-auditor/internal/repository"
)

// sessionStore uses the real repository for everything except the session
// lookup, so the denial is written to the real auditor_operation_log.
type sessionStore struct {
	*repository.Store
	user *repository.AuditorUser
}

func (s sessionStore) GetAuditorSession(context.Context, []byte) (*repository.AuditorUser, error) {
	return s.user, nil
}

func TestDeniedMalformedEnvironmentWritesAuditRowIntegration(t *testing.T) {
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
	var env, userID, username string
	if err := pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode) VALUES(gen_random_uuid()::text,'self_hosted','single_database') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO auditor_user(username,password_hash,role) VALUES('srch_' || substr(md5(random()::text),1,12),'x','viewer') RETURNING id::text, username`).Scan(&userID, &username); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM auditor_operation_log WHERE user_id=$1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM auditor_user WHERE id=$1`, userID)
	})
	store := sessionStore{Store: repository.NewStore(pool), user: &repository.AuditorUser{ID: userID, Username: username, Role: "viewer", Active: true, Environments: []string{env}}}
	mux := http.NewServeMux()
	registerSearchRoutes(mux, &fakeSearchStore{})
	handler := authMiddleware(mux, store)
	for _, malformed := range []string{"*", "-1", env + "," + env, "%27%20OR%201%3D1"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=orders&environment_id="+malformed, nil)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%q: status %d", malformed, rec.Code)
		}
	}
	var denied int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM auditor_operation_log WHERE user_id=$1 AND action='GET /api/v1/search' AND result='denied' AND environment_id IS NULL`, userID).Scan(&denied); err != nil {
		t.Fatal(err)
	}
	if denied != 4 {
		t.Fatalf("expected 4 denial rows with NULL environment, got %d", denied)
	}
}
