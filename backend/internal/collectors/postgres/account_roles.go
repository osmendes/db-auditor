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
}

func CollectAccountRoles(ctx context.Context, conn *pgx.Conn) ([]AccountRoleFacts, error) {
	rows, err := conn.Query(ctx, `SELECT current_database(),r.rolname,r.rolcanlogin,r.rolvaliduntil,
EXISTS(SELECT 1 FROM pg_catalog.pg_stat_activity a WHERE a.usesysid=r.oid) AS sampled_active,
r.rolsuper,r.rolbypassrls,r.rolreplication
FROM pg_catalog.pg_roles r WHERE r.rolcanlogin AND r.rolname !~ '^pg_' ORDER BY r.rolname LIMIT $1`, MaxAccountRoles+1)
	if err != nil {
		return nil, fmt.Errorf("consultar contas do banco: %w", err)
	}
	defer rows.Close()
	out := make([]AccountRoleFacts, 0)
	for rows.Next() {
		var item AccountRoleFacts
		if err := rows.Scan(&item.DatabaseName, &item.RoleName, &item.CanLogin, &item.ValidUntil, &item.SampledActive, &item.Superuser, &item.BypassRLS, &item.Replication); err != nil {
			return nil, err
		}
		out = append(out, item)
		if len(out) > MaxAccountRoles {
			return nil, fmt.Errorf("mais de %d contas; reduza o escopo da coleta", MaxAccountRoles)
		}
	}
	return out, rows.Err()
}
