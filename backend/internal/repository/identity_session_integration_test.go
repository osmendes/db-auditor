package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestAuditorSessionExpiryIntegration exercises the SQL of GetAuditorSession
// against Postgres: the idle deadline (expires_at), the absolute deadline
// (absolute_expires_at) and u.active each reject the session on their own, and
// a live session slides its idle deadline without passing the absolute one.
func TestAuditorSessionExpiryIntegration(t *testing.T) {
	dsn := os.Getenv("AUDITOR_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("AUDITOR_TEST_DISPOSABLE") != "1" {
		t.Skip("disposable integration database not explicitly enabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := NewStore(pool)

	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	hash, err := HashAuditorPassword("a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateAuditorUser(ctx, "session-expiry-"+hex.EncodeToString(suffix), hash, "viewer", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM auditor_session WHERE user_id=$1::uuid`, user.ID)
	})

	newSession := func(t *testing.T, name string, idle, absolute time.Duration) []byte {
		t.Helper()
		digest := sha256.Sum256([]byte(name + hex.EncodeToString(suffix)))
		if err := store.CreateAuditorSession(ctx, user.ID, digest[:], time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		// Place both deadlines relative to the database clock.
		if _, err := pool.Exec(ctx, `UPDATE auditor_session SET expires_at=now()+make_interval(secs=>$2), absolute_expires_at=now()+make_interval(secs=>$3) WHERE token_hash=$1`,
			digest[:], idle.Seconds(), absolute.Seconds()); err != nil {
			t.Fatal(err)
		}
		return digest[:]
	}
	resolves := func(t *testing.T, token []byte) bool {
		t.Helper()
		got, err := store.GetAuditorSession(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		if got != nil && got.ID != user.ID {
			t.Fatalf("session resolved to another user: %+v", got)
		}
		return got != nil
	}

	live := newSession(t, "live", 10*time.Minute, 2*time.Hour)
	idleExpired := newSession(t, "idle", -time.Second, 2*time.Hour)
	absoluteExpired := newSession(t, "absolute", 10*time.Minute, -time.Second)
	bothExpired := newSession(t, "both", -time.Minute, -time.Minute)
	nearAbsolute := newSession(t, "near-absolute", 10*time.Minute, 20*time.Minute)

	if !resolves(t, live) {
		t.Fatal("live session must resolve")
	}
	for name, token := range map[string][]byte{"idle deadline passed": idleExpired, "absolute deadline passed": absoluteExpired, "both passed": bothExpired} {
		if resolves(t, token) {
			t.Fatalf("%s: expired session still resolves", name)
		}
	}
	// Rejection must not extend the expired row either.
	var idleStillPast bool
	if err := pool.QueryRow(ctx, `SELECT expires_at<=now() FROM auditor_session WHERE token_hash=$1`, idleExpired).Scan(&idleStillPast); err != nil || !idleStillPast {
		t.Fatalf("rejected session was refreshed: past=%v err=%v", idleStillPast, err)
	}

	// Sliding window: the idle deadline moves to now()+45min but never past
	// the absolute deadline.
	var slid, capped bool
	if err := pool.QueryRow(ctx, `SELECT expires_at BETWEEN now()+interval '44 minutes' AND now()+interval '46 minutes' FROM auditor_session WHERE token_hash=$1`, live).Scan(&slid); err != nil || !slid {
		t.Fatalf("live session idle deadline did not slide to 45 minutes: %v %v", slid, err)
	}
	if !resolves(t, nearAbsolute) {
		t.Fatal("session before its absolute deadline must resolve")
	}
	if err := pool.QueryRow(ctx, `SELECT expires_at=absolute_expires_at FROM auditor_session WHERE token_hash=$1`, nearAbsolute).Scan(&capped); err != nil || !capped {
		t.Fatalf("idle deadline must be capped at the absolute deadline: %v %v", capped, err)
	}

	// An inactive account loses its live session immediately.
	if _, err := pool.Exec(ctx, `UPDATE auditor_user SET active=false WHERE id=$1::uuid`, user.ID); err != nil {
		t.Fatal(err)
	}
	if resolves(t, live) {
		t.Fatal("session of an inactive user still resolves")
	}

	// Cleanup removes idle-expired rows.
	if err := store.DeleteExpiredAuditorSessions(ctx); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM auditor_session WHERE token_hash=$1 OR token_hash=$2`, idleExpired, bothExpired).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("expired sessions not purged: %d %v", remaining, err)
	}
}
