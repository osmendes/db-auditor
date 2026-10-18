#!/bin/sh
# Daily pg_dump of the snapshot store. Target DSNs are not in this database.
# The dump holds password hashes, TOTP secrets and the whole history: the
# directory is 0700 and every dump and checksum 0600 (BA-16, #129).
set -eu
umask 077
retention="${BACKUP_RETENTION_DAYS:-14}"
dir="${BACKUP_DIR:-/backup}"
mkdir -p "$dir"
chmod 0700 "$dir"
# Dumps written before the umask (0644) are tightened as well.
find "$dir" -maxdepth 1 -name 'db-auditor-*.dump*' -type f -exec chmod 0600 {} +
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
out="${dir}/db-auditor-${stamp}.dump"
pg_dump -Fc --no-password -f "$out" || {
  rm -f "$out"
  psql -v ON_ERROR_STOP=1 -c "UPDATE snapshot_backup SET last_attempt_at=now(), last_status='failed' WHERE id=1;" || true
  exit 1
}
sha256sum "$out" > "$out.sha256"
chmod 0600 "$out" "$out.sha256"
find "$dir" -name 'db-auditor-*.dump*' -type f -mtime +"$retention" -delete
psql -v ON_ERROR_STOP=1 -c "UPDATE snapshot_backup SET last_success_at=now(), last_attempt_at=now(), last_status='success', artifact_path='${out}' WHERE id=1;"
echo "backup wrote ${out}"
