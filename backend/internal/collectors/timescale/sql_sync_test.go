package timescale

import (
	"os"
	"strings"
	"testing"
)

// The checked-in SQL is the reviewable contract for the SQL run by Go.
func TestReviewedSQLMatchesExecutedSQL(t *testing.T) {
	for name, executed := range map[string]string{"jobs": jobsSQL, "policies": policiesSQL} {
		body, err := os.ReadFile("../../../sql/collectors/timescale/" + name + ".sql")
		if err != nil {
			t.Fatal(err)
		}
		reviewed := string(body)
		if i := strings.IndexByte(reviewed, '\n'); i >= 0 && strings.HasPrefix(reviewed, "--") {
			reviewed = reviewed[i+1:]
		}
		if strings.TrimSpace(reviewed) != strings.TrimSpace(executed) {
			t.Fatalf("%s.sql diverges from runtime collector SQL", name)
		}
	}
}
