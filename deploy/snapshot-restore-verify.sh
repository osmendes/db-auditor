#!/bin/sh
# Restore drill for the snapshot store (BA-10). Restores a dump into an
# ISOLATED Postgres, checks the archive, measures recovery time and counts
# snapshots, jobs and permissions, then writes evidence JSON that an operator
# records with POST /api/v1/retention-policy/evidence.
#
#   counts mode:  snapshot-restore-verify.sh counts  "$SOURCE_DSN" > source-counts.json
#   verify mode:  RESTORE_TARGET_CONFIRM=isolated ENV_LABEL=staging \
#                 snapshot-restore-verify.sh verify dump.file "$TARGET_DSN" [source-counts.json] > evidence.json
#
# The target is cleaned (--clean). Never point it at production: the script
# refuses unless RESTORE_TARGET_CONFIRM=isolated and the target differs from
# SOURCE_DSN when that variable is set.
set -eu

counts_sql="SELECT json_build_object(
 'audit_run',(SELECT count(*) FROM audit_run),
 'table_snapshot',(SELECT count(*) FROM table_snapshot),
 'finding',(SELECT count(*) FROM finding),
 'finding_event',(SELECT count(*) FROM finding_event),
 'report_job',(SELECT count(*) FROM report_job),
 'report_artifact',(SELECT count(*) FROM report_artifact),
 'auditor_user',(SELECT count(*) FROM auditor_user),
 'auditor_user_roles',(SELECT COALESCE(json_object_agg(role,n),'{}'::json) FROM (SELECT role,count(*) n FROM auditor_user GROUP BY role ORDER BY role) r),
 'environment_grants',(SELECT count(*) FROM auditor_user_environment),
 'schema_migration',CASE WHEN to_regclass('schema_migration') IS NULL THEN NULL ELSE (xpath('/row/n/text()',query_to_xml('SELECT count(*) AS n FROM schema_migration',false,true,'')))[1]::text::bigint END,
 'last_migration',CASE WHEN to_regclass('schema_migration') IS NULL THEN NULL ELSE (xpath('/row/v/text()',query_to_xml('SELECT max(version) AS v FROM schema_migration',false,true,'')))[1]::text END)"

counts() {
  psql "$1" -X -At -v ON_ERROR_STOP=1 -c "$counts_sql"
}

evidence() {
  # $1 succeeded, $2 rto seconds or null, $3 integrity json, $4 note
  printf '{"kind":"restore_drill","environment_label":"%s","artifact_sha256":"%s","succeeded":%s,"rto_seconds":%s,"integrity":%s,"note":"%s","performed_at":"%s"}\n' \
    "$ENV_LABEL" "$sha" "$1" "$2" "$3" "$4" "$started_at"
}

mode="${1:?modo: counts ou verify}"
case "$mode" in
counts)
  counts "${2:?informe o DSN de origem}"
  exit 0
  ;;
verify) ;;
*)
  echo "modo desconhecido: $mode" >&2
  exit 2
  ;;
esac

dump="${2:?informe o caminho do dump}"
target="${3:?informe o DSN do Postgres isolado}"
expected="${4:-}"
ENV_LABEL="${ENV_LABEL:?informe ENV_LABEL (ex.: staging)}"
if [ "${RESTORE_TARGET_CONFIRM:-}" != "isolated" ]; then
  echo "defina RESTORE_TARGET_CONFIRM=isolated para confirmar que o destino é um ambiente isolado" >&2
  exit 2
fi
if [ -n "${SOURCE_DSN:-}" ] && [ "$SOURCE_DSN" = "$target" ]; then
  echo "o destino é igual a SOURCE_DSN; a restauração nunca roda sobre a origem" >&2
  exit 2
fi
if [ ! -f "$dump" ]; then
  echo "dump inexistente: $dump" >&2
  exit 1
fi
sha="$(sha256sum "$dump" | cut -d ' ' -f 1)"
started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
if [ -f "$dump.sha256" ] && [ "$(cut -d ' ' -f 1 "$dump.sha256")" != "$sha" ]; then
  evidence false null '{}' "sha256 do dump difere de $dump.sha256"
  echo "sha256 do dump não confere com o registrado no backup" >&2
  exit 1
fi
if ! pg_restore --list "$dump" >/dev/null; then
  evidence false null '{}' "pg_restore --list rejeitou o arquivo"
  echo "arquivo de backup ilegível; nada foi restaurado" >&2
  exit 1
fi
begin="$(date +%s)"
if ! pg_restore --clean --if-exists --no-password --exit-on-error -d "$target" "$dump"; then
  evidence false null '{}' "pg_restore falhou"
  echo "restauração falhou; o ambiente isolado não foi promovido" >&2
  exit 1
fi
rto=$(( $(date +%s) - begin ))
if ! restored="$(counts "$target")"; then
  evidence false "$rto" '{}' "contagens de integridade falharam"
  echo "contagens de integridade falharam" >&2
  exit 1
fi
if [ -n "$expected" ]; then
  want="$(tr -d ' \n' < "$expected")"
  got="$(printf '%s' "$restored" | tr -d ' \n')"
  if [ "$want" != "$got" ]; then
    evidence false "$rto" "$restored" "contagens restauradas diferem da origem"
    echo "contagens restauradas diferem da origem: esperado $want, obtido $got" >&2
    exit 1
  fi
fi
evidence true "$rto" "$restored" "restauração isolada conferida"
echo "restauração conferida em ${rto}s" >&2
