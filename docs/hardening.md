# Hardening notes — DB Auditor (Sprint 12)

## Backend

- Collectors e analyzers: **read-only**; evidências com `safety_note` onde aplicável
- Partial failures: AuditRunner modela SUCCESS / PARTIAL_SUCCESS / FAILED; multi-DB usa `PartialWarning` por database
- Timeouts: `AUDITOR_STATEMENT_TIMEOUT`, `AUDITOR_LOCK_TIMEOUT`, context cancel em collectors (padrão 10m por coletor)
- Migrations SQL versionadas em `backend/migrations`
- Compatibilidade Timescale: registry em collectors/timescale
- **DSN sanitization**: `config.SanitizeDSN` / `SanitizeError` em erros de connect e `FormatPartialErrors` — senhas nunca em logs/UI

## Identidade do produto vs módulo Go

| Conceito | Valor |
|----------|--------|
| Produto / UI | **DB Auditor** |
| Repositório canônico | `mayconmendes-qc/db-auditor` |
| Fork de desenvolvimento | `osmendes/db-auditor` |
| Banco interno (default) | `POSTGRES_DB=db_auditor` |
| `application_name` | `db-auditor` |
| Module path Go | `github.com/mayconmendes-qc/db-auditor` |

## Frontend

- Sem acesso direto a bancos
- Listas grandes: paginação API (Inventory Explorer)
- Estados loading/error/empty nas páginas de operação e dashboard
- Navegação por teclado nos botões da shell; labels em filtros

## Integração

- Fluxos críticos cobertos por testes Go/Vitest e smoke `make smoke`
- Relatórios JSON em `/api/v1/reports/*` para exportação posterior CSV se necessário
