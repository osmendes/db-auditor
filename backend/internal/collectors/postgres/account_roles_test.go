package postgres

import "testing"

// #134: the pg_has_role modes switch exactly at PostgreSQL 16
// (server_version_num 160000). 'SET', 'USAGE' and 'MEMBER WITH ADMIN OPTION'
// do not exist before 16, and 'MEMBER' on 16+ would count INHERIT FALSE,
// SET FALSE memberships. The integration matrix (PG 14-18) checks the same
// boundary against real servers.
func TestMembershipModesSwitchAtPostgreSQL16(t *testing.T) {
	for _, tc := range []struct {
		version             int
		set, inherit, admin string
	}{
		{140024, "MEMBER", "MEMBER", "MEMBER"},
		{150014, "MEMBER", "MEMBER", "MEMBER"},
		{159999, "MEMBER", "MEMBER", "MEMBER"},
		{160000, "SET", "USAGE", "MEMBER WITH ADMIN OPTION"},
		{160010, "SET", "USAGE", "MEMBER WITH ADMIN OPTION"},
		{170006, "SET", "USAGE", "MEMBER WITH ADMIN OPTION"},
		{180000, "SET", "USAGE", "MEMBER WITH ADMIN OPTION"},
	} {
		set, inherit, admin := membershipModes(tc.version)
		if set != tc.set || inherit != tc.inherit || admin != tc.admin {
			t.Fatalf("membershipModes(%d) = %q %q %q, want %q %q %q", tc.version, set, inherit, admin, tc.set, tc.inherit, tc.admin)
		}
	}
}
