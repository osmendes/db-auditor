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
| Repositório GitHub | `osmendes/db-auditor` |
| Banco interno (default) | `POSTGRES_DB=db_auditor` |
| `application_name` | `db-auditor` |
| Module path Go | `github.com/osmendes/db-auditor` (após rename #82) |

## Frontend

- Sem acesso direto a bancos
- Listas grandes: paginação API (Inventory Explorer)
- Estados loading/error/empty nas páginas de operação e dashboard
- Navegação por teclado nos botões da shell; labels em filtros

## Integração

- Fluxos críticos cobertos por testes Go/Vitest e smoke `make smoke`
- Relatórios JSON em `/api/v1/reports/*` para exportação posterior CSV se necessário

## Sessão

O cookie permanece `SameSite=Lax` e `HttpOnly`. Lax preserva a entrada por um link compartilhado; `Strict` exigiria um novo login nessa navegação. Mutações continuam exigindo o token CSRF, então um site externo não as dispara só com o cookie.

A sessão ociosa dura 45 minutos. Um GET autenticado renova `last_seen_at` sem ultrapassar o teto absoluto de 8 horas gravado na criação. Logout apaga a linha.

## Hosts inseguros do alvo

`AUDITOR_TARGET_INSECURE_HOSTS` começa vazio. Sem um nome nessa lista, o auditor recusa TLS sem verificação no banco analisado. O padrão é verificar a cadeia completa.

