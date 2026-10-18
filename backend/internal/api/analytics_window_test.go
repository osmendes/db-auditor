package api

import (
	"net/http/httptest"
	"testing"
)

func TestAnalyticsWindowValidation(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		valid       bool
	}{
		{"default", "", true},
		{"weekly", "?from=2026-09-01T00:00:00Z&to=2026-10-01T00:00:00Z&granularity=week", true},
		{"reverse", "?from=2026-10-01T00:00:00Z&to=2026-09-01T00:00:00Z", false},
		{"too_long", "?from=2024-01-01T00:00:00Z&to=2026-01-01T00:00:00Z", false},
		{"ninety_days", "?from=2026-07-03T00:00:00Z&to=2026-10-01T00:00:00Z", true},
		{"ninety_one_days", "?from=2026-07-02T00:00:00Z&to=2026-10-01T00:00:00Z", false},
		{"year_weekly", "?from=2025-10-01T00:00:00Z&to=2026-10-01T00:00:00Z&granularity=week", false},
		{"hour_too_long", "?from=2026-09-01T00:00:00Z&to=2026-10-01T00:00:00Z&granularity=hour", false},
		{"invalid_granularity", "?granularity=year", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := analyticsWindow(httptest.NewRequest("GET", "/api/v1/analytics/storage"+tc.query, nil))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}
