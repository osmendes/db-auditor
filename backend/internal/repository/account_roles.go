package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/osmendes/db-auditor/internal/analyzer"
	"github.com/osmendes/db-auditor/internal/collectors/postgres"
)

func (s *Store) SaveAccountRoles(ctx context.Context, environmentID, auditRunID pgtype.UUID, roles []postgres.AccountRoleFacts) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, role := range roles {
		_, err = tx.Exec(ctx, `INSERT INTO account_role_snapshot(audit_run_id,environment_id,database_name,role_name,can_login,valid_until,sampled_active,is_superuser,bypass_rls,replication,create_role,inherited_privileges)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT(audit_run_id,database_name,role_name) DO NOTHING`, auditRunID, environmentID,
			role.DatabaseName, role.RoleName, role.CanLogin, role.ValidUntil, role.SampledActive, role.Superuser, role.BypassRLS, role.Replication, role.CreateRole, inheritedOrEmpty(role.Inherited))
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) loadAccountRoles(ctx context.Context, environmentID, auditRunID string, facts *analyzer.SnapshotFacts) error {
	rows, err := s.pool.Query(ctx, `SELECT r.database_name,r.role_name,r.can_login,r.valid_until,r.sampled_active,r.collected_at,r.is_superuser,r.bypass_rls,r.replication,r.create_role,r.inherited_privileges,
COALESCE((SELECT count(*)>=2 AND min(h.collected_at)<=r.collected_at-interval '30 days' AND NOT bool_or(h.sampled_active)
FROM account_role_snapshot h WHERE h.environment_id=r.environment_id AND h.database_name=r.database_name AND h.role_name=r.role_name
AND h.collected_at BETWEEN r.collected_at-interval '90 days' AND r.collected_at),false)
FROM account_role_snapshot r WHERE r.environment_id=$1::uuid AND r.audit_run_id=$2::uuid`, environmentID, auditRunID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var role analyzer.RoleFact
		if err := rows.Scan(&role.Database, &role.RoleName, &role.Login, &role.ValidUntil, &role.SampledActive, &role.CollectedAt, &role.Superuser, &role.BypassRLS, &role.Replication, &role.CreateRole, &role.Inherited, &role.PossiblyInactive); err != nil {
			return err
		}
		facts.Roles = append(facts.Roles, role)
	}
	return rows.Err()
}

// inheritedOrEmpty stores an empty array instead of NULL.
func inheritedOrEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
