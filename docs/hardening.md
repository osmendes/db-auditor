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
| Module path Go | `github.com/mayconmendes-qc/db-auditor` |

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

`AUDITOR_TARGET_INSECURE_HOSTS` começa vazio. Sem um nome nessa lista, o auditor recusa TLS sem verificação no banco analisado. O padrão é verificar a cadeia completa (`sslmode=verify-full`). Um host só entra com TLS fraco (`disable`, `require` ou `verify-ca`) se também estiver em `AUDITOR_TARGET_ALLOWED_HOSTS`. O boot registra os nomes da lista, nunca o DSN. Se a política não exigir a exceção, deixe a variável vazia e use stunnel ou uma CA interna.

## TOTP

O operador pode ativar TOTP na própria conta em Contas. O segredo fica cifrado com `AUDITOR_TOTP_KEY` (mínimo de 16 caracteres). Oito códigos de recuperação são mostrados uma vez e guardados só como hash SHA-256. Não há SMS. Viewer e auditor podem ativar o mesmo fluxo; só o operador marca a própria conta como obrigatória, e só depois de confirmar um código. No login, a senha correta de uma conta com TOTP devolve `mfa_required` e um desafio de 5 minutos, sem cookie de sessão. O segundo passo aceita o código do autenticador ou um código de recuperação ainda não usado.

## Rate limit de login

O bloqueio de senha usa a conta junto com o IP visto pelo proxy confiável (`login-pair`). Uma conta bloqueada não impede outra conta no mesmo NAT. Sem proxy confiável, o IP é o da conexão. A resposta 429 não revela se o usuário existe.

## Métricas

`GET /metrics` escuta em `:9090` dentro do container. O Compose não publica essa porta no host. O scrape fica em `api:9090`, na rede do Compose. Não mapeie 9090 para a interface da VPS.

## Retenção, backup e criptografia

A política em `retention_policy` guarda o dono, o prazo dos jobs de PDF (padrão 90 dias) e o prazo de snapshots (0 = não apagar até aprovação). A expiração do PDF remove só o artefato; a trilha do job permanece. O script `deploy/snapshot-backup.sh` gera `pg_dump` custom e registra sucesso ou falha. Restaure com `pg_restore --clean --if-exists` em um Postgres vazio, fora de produção, e confira contagens de `audit_run`, `finding` e `auditor_user` antes de promover. Volumes e backups precisam de criptografia do disco ou do destino (LUKS, volume cifrado do provedor ou `age`/`gpg` no arquivo). O Compose não guarda a chave. Sem backup verificado e aprovação, nenhuma limpeza apaga snapshots existentes.

## CSP

A API envia `Content-Security-Policy` em todas as respostas. O Vite de desenvolvimento envia a mesma política com `script-src 'unsafe-inline'` e `connect-src` para o websocket local, porque o servidor de desenvolvimento injeta o cliente HMR. Produção, no Caddy, não libera `unsafe-inline` em script.

