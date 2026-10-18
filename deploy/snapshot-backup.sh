#!/bin/sh
# Daily pg_dump of the snapshot store. Target DSNs are not in this database.
set -eu
retention="${BACKUP_RETENTION_DAYS:-14}"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
out="/backup/db-auditor-${stamp}.dump"
pg_dump -Fc --no-password -f "$out" || {
  psql -v ON_ERROR_STOP=1 -c "UPDATE snapshot_backup SET last_attempt_at=now(), last_status='failed' WHERE id=1;" || true
  exit 1
}
sha256sum "$out" > "$out.sha256"
find /backup -name 'db-auditor-*.dump*' -type f -mtime +"$retention" -delete
psql -v ON_ERROR_STOP=1 -c "UPDATE snapshot_backup SET last_success_at=now(), last_attempt_at=now(), last_status='success', artifact_path='${out}' WHERE id=1;"
echo "backup wrote ${out}"
