#!/bin/sh
# Restore a snapshot-store dump into an isolated Postgres. Never point this at production.
set -eu
dump="${1:?informe o caminho do dump}"
db="${POSTGRES_DB:-auditor}"
if ! pg_restore --clean --if-exists --no-password -d "$db" "$dump"; then
  echo "restauração falhou; o ambiente isolado não foi promovido" >&2
  exit 1
fi
echo "restauração concluída em ${db}"
