package audit_test

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mayconmendes-qc/db-auditor/internal/audit"
	"github.com/mayconmendes-qc/db-auditor/internal/repository"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestMongoRegistryPersistsInventoryWithoutPostgresRules(t *testing.T) {
	uri, control := os.Getenv("AUDITOR_MONGO_TEST_URI"), os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if uri == "" || control == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("requires disposable MongoDB and snapshot store")
	}
	u, err := url.Parse(uri)
	if err != nil || !strings.HasSuffix(u.Path, "_ci") {
		t.Fatal("isolated _ci MongoDB database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mongoClient, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()
	collection := mongoClient.Database(strings.TrimPrefix(u.Path, "/")).Collection("audit_runner_ci")
	if _, err := collection.InsertOne(ctx, bson.M{"secret": "never-persist-this"}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = collection.Drop(context.Background()) }()
	pool, err := pgxpool.New(ctx, control)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := repository.NewStore(pool)
	var env string
	if err := pool.QueryRow(ctx, `INSERT INTO audit_environment(name,type,discovery_mode,engine) VALUES(gen_random_uuid()::text,'self_hosted','single_database','mongodb') RETURNING id::text`).Scan(&env); err != nil {
		t.Fatal(err)
	}
	runner := audit.NewRunner(audit.NewRegistry(), &repository.AuditRunStore{Store: store}, audit.RunnerOptions{
		EngineRegistries: map[string]*audit.Registry{"mongodb": audit.NewMongoRegistry(map[string]string{env: uri}, store)},
	})
	result, err := runner.Run(ctx, env, audit.ProfileManual)
	if err != nil || result.Status != "success" || result.Analysis != nil {
		t.Fatalf("MongoDB run: %+v %v", result, err)
	}
	var collections, leaked int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM table_snapshot WHERE audit_run_id=$1::uuid AND relation_class='collection' AND table_name='audit_runner_ci'`, result.AuditRunID).Scan(&collections); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM finding_event WHERE audit_run_id=$1::uuid`, result.AuditRunID).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if collections != 1 || leaked != 0 {
		t.Fatalf("MongoDB facts/rules: collections=%d postgres_findings=%d", collections, leaked)
	}
	if rules, err := store.EffectiveRules(ctx, env, ""); err != nil || len(rules) != 0 {
		t.Fatalf("PostgreSQL rules leaked to MongoDB: %d %v", len(rules), err)
	}
	score, err := store.GetScopeScore(ctx, env, result.AuditRunID, "", "", "")
	if err != nil || score.Status != "not_applicable" || score.Score != nil {
		t.Fatalf("MongoDB score: %+v %v", score, err)
	}
}
