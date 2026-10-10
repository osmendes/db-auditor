package repository

import "testing"

func TestRegressionRequiresTwoCompleteCollections(t *testing.T) {
	cases := []struct {
		current, previous string
		want              bool
	}{
		{"complete", "complete", true},
		{"partial", "complete", false},
		{"complete", "partial", false},
		{"partial", "partial", false},
		{"incompatible", "complete", false},
		{"", "complete", false},
		{"complete", "", false},
	}
	for _, tc := range cases {
		if got := AllowRegressionAlert(tc.current, tc.previous); got != tc.want {
			t.Errorf("AllowRegressionAlert(%q,%q)=%v want %v", tc.current, tc.previous, got, tc.want)
		}
	}
}

func TestFindingAlertSkipsPartialNeighbor(t *testing.T) {
	if !AllowFindingAlert(true, false, false) {
		t.Fatal("first complete collection may alert")
	}
	if AllowFindingAlert(false, false, false) {
		t.Fatal("partial collection must not alert findings")
	}
	if AllowFindingAlert(true, true, false) {
		t.Fatal("partial previous collection must silence the alert")
	}
	if !AllowFindingAlert(true, true, true) {
		t.Fatal("two complete collections may alert")
	}
}
