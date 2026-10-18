package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const MaxAccountRoles = 10000

// AccountRoleFacts is a bounded sample of public role metadata. A missing
// active session is not proof that the account has not logged in recently.
type AccountRoleFacts struct {
	DatabaseName  string
	RoleName      string
	CanLogin      bool
	ValidUntil    *time.Time
	SampledActive bool
	// Attributes behind security.powerful_role (#126).
	Superuser   bool
	BypassRLS   bool
	Replication bool
	CreateRole  bool
	// Inherited is the power the role gets through membership, as
	// "privilege:source role" (for example "superuser:g_admin" or
	// "pg_execute_server_program:pg_execute_server_program"), sorted.
	Inherited []string
}

// MaxInheritedPrivileges bounds the membership sources kept per role.
const MaxInheritedPrivileges = 50

// PowerfulPredefinedRoles are the predefined roles that reach the server's
// file system or programs, or read or write every table.
var PowerfulPredefinedRoles = []string{"pg_execute_server_program", "pg_read_server_files", "pg_write_server_files", "pg_read_all_data", "pg_write_all_data"}

// accountRolesSQL samples login roles with their own attributes and the
// power they reach through membership. Role attributes (SUPERUSER,
// BYPASSRLS, CREATEROLE, REPLICATION) are never inherited: a member gets them
// only by SET ROLE, so $2 is the pg_has_role mode for that ('SET' on
// PostgreSQL 16+, 'MEMBER' before). Predefined roles confer privileges both
// by inheritance and by SET ROLE, so $3 ('USAGE' on 16+, 'MEMBER' before) is
// checked as well. On 16+ a membership WITH ADMIN OPTION alone lets the member
// grant the role to itself WITH SET/INHERIT TRUE and take its power in one
// command, so $6 ('MEMBER WITH ADMIN OPTION' on 16+, 'MEMBER' before, where
// MEMBER already covers it) counts for BYPASSRLS, REPLICATION, CREATEROLE and
// the predefined roles. ADMIN over a SUPERUSER role is not a path: only a
// superuser may grant it. A membership WITH INHERIT FALSE, SET FALSE and no
// ADMIN gives nothing and is not reported.
const accountRolesSQL = `SELECT current_database(),r.rolname,r.rolcanlogin,r.rolvaliduntil,
EXISTS(SELECT 1 FROM pg_catalog.pg_stat_activity a WHERE a.usesysid=r.oid) AS sampled_active,
r.rolsuper,r.rolbypassrls,r.rolreplication,r.rolcreaterole,
-- A superuser passes every pg_has_role check and is reported as such.
CASE WHEN r.rolsuper THEN '{}'::text[] ELSE ARRAY(SELECT x FROM (
  SELECT 'superuser:'||g.rolname AS x FROM pg_catalog.pg_roles g WHERE g.oid<>r.oid AND g.rolsuper AND pg_catalog.pg_has_role(r.oid,g.oid,$2)
  UNION SELECT 'bypass_rls:'||g.rolname FROM pg_catalog.pg_roles g WHERE g.oid<>r.oid AND g.rolbypassrls AND (pg_catalog.pg_has_role(r.oid,g.oid,$2) OR pg_catalog.pg_has_role(r.oid,g.oid,$6))
  UNION SELECT 'replication:'||g.rolname FROM pg_catalog.pg_roles g WHERE g.oid<>r.oid AND g.rolreplication AND (pg_catalog.pg_has_role(r.oid,g.oid,$2) OR pg_catalog.pg_has_role(r.oid,g.oid,$6))
  UNION SELECT 'createrole:'||g.rolname FROM pg_catalog.pg_roles g WHERE g.oid<>r.oid AND g.rolcreaterole AND (pg_catalog.pg_has_role(r.oid,g.oid,$2) OR pg_catalog.pg_has_role(r.oid,g.oid,$6))
  UNION SELECT g.rolname||':'||g.rolname FROM pg_catalog.pg_roles g WHERE g.rolname=ANY($4::text[])
    AND (pg_catalog.pg_has_role(r.oid,g.oid,$2) OR pg_catalog.pg_has_role(r.oid,g.oid,$3) OR pg_catalog.pg_has_role(r.oid,g.oid,$6))
) inherited ORDER BY x LIMIT $5) END AS inherited
FROM pg_catalog.pg_roles r WHERE r.rolcanlogin AND r.rolname !~ '^pg_' ORDER BY r.rolname LIMIT $1`

// membershipModes returns the pg_has_role modes for attributes reached by
// SET ROLE, privileges reached by inheritance and roles the member can grant
// to itself (ADMIN OPTION).
func membershipModes(serverVersionNum int) (setMode, inheritMode, adminMode string) {
	if serverVersionNum >= 160000 {
		return "SET", "USAGE", "MEMBER WITH ADMIN OPTION"
	}
	return "MEMBER", "MEMBER", "MEMBER"
}

func CollectAccountRoles(ctx context.Context, conn *pgx.Conn) ([]AccountRoleFacts, error) {
	var version int
	if err := conn.QueryRow(ctx, `SELECT current_setting('server_version_num')::int`).Scan(&version); err != nil {
		return nil, fmt.Errorf("versão do servidor: %w", err)
	}
	setMode, inheritMode, adminMode := membershipModes(version)
	rows, err := conn.Query(ctx, accountRolesSQL, MaxAccountRoles+1, setMode, inheritMode, PowerfulPredefinedRoles, MaxInheritedPrivileges, adminMode)
	if err != nil {
		return nil, fmt.Errorf("consultar contas do banco: %w", err)
	}
	defer rows.Close()
	out := make([]AccountRoleFacts, 0)
	for rows.Next() {
		var item AccountRoleFacts
		if err := rows.Scan(&item.DatabaseName, &item.RoleName, &item.CanLogin, &item.ValidUntil, &item.SampledActive, &item.Superuser, &item.BypassRLS, &item.Replication, &item.CreateRole, &item.Inherited); err != nil {
			return nil, err
		}
		out = append(out, item)
		if len(out) > MaxAccountRoles {
			return nil, fmt.Errorf("mais de %d contas; reduza o escopo da coleta", MaxAccountRoles)
		}
	}
	return out, rows.Err()
}
