package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mayconmendes-qc/db-auditor/internal/collectors/postgres"
)

func (s *Store) SaveAuditorPrivilege(ctx context.Context, environmentID, auditRunID pgtype.UUID, role postgres.AuditorRole) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO auditor_privilege_snapshot
(audit_run_id, environment_id, database_name, role_name, is_superuser, can_create_db, can_create_role, replication, can_write, check_failed)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (audit_run_id, database_name) DO UPDATE SET
  role_name=EXCLUDED.role_name, is_superuser=EXCLUDED.is_superuser, can_create_db=EXCLUDED.can_create_db,
  can_create_role=EXCLUDED.can_create_role, replication=EXCLUDED.replication, can_write=EXCLUDED.can_write,
  check_failed=EXCLUDED.check_failed, collected_at=now()`,
		auditRunID, environmentID, role.Database, role.RoleName, role.Superuser, role.CreateDB, role.CreateRole, role.Replication, role.CanWrite, role.CheckFailed)
	return err
}
