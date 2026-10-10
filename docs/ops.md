# Operação em VPS — DB Auditor

## Schema do snapshot store

O schema canônico está em `backend/migrations/01_baseline.sql`. O seed local é `02_seed_demo.sql`. A API aplica o baseline só quando o banco está vazio e registra `schema_migration`. Um volume que já tem o schema atual recebe só o que falta (`03_audit_schedule.sql` até `17_report_retention.sql`) e não reaplica o baseline. O segundo boot não executa SQL de novo. Se o checksum de um arquivo já aplicado mudar, a API recusa subir. A migração da segunda etapa e seu procedimento operacional estão em [second-stage-operations.md](second-stage-operations.md).

Antes de atualizar uma instalação persistente, faça backup do snapshot store e confira a restauração. A migração `12_view_detail.sql` acrescenta campos sem apagar visões existentes. Depois do deploy, execute uma nova coleta de PostgreSQL para preencher colunas e opções de segurança: execuções antigas mostram esses campos como não coletados. Para voltar à versão anterior, pare a API nova e restaure o backup ou use a versão anterior do aplicativo; não remova as novas colunas manualmente.

A migração `13_index_detail.sql` também preserva os snapshots antigos. Uma nova coleta preenche as colunas incluídas e informa se a fonte de estatísticas de uso do índice estava disponível. Zero leituras em um único snapshot não prova que um índice é dispensável: confira o período de coleta, o reinício das estatísticas e o histórico antes de qualquer alteração manual no banco auditado.

A migração `14_function_detail.sql` acrescenta dados de retorno, segurança e métricas agregadas das funções sem remover snapshots. Ela diferencia funções com o mesmo nome pela assinatura. A lista de papéis com execução efetiva é consultada por página; configurações e corpo SQL não aparecem no contrato de detalhe. Para voltar à versão anterior, preserve o backup do snapshot store e use a versão anterior da API.

A migração `15_cagg_source.sql` acrescenta campos opcionais de origem e intervalo de bucket aos agregados contínuos. As coletas antigas continuam legíveis e exibem “não coletado” nesses campos. Após atualizar, faça uma nova coleta Timescale para preenchê-los quando a versão instalada oferecer os metadados. O SQL da definição permanece protegido; nenhuma alteração é feita no banco auditado. Para desfazer a implantação, pare a API nova e restaure o backup do snapshot store ou use a versão anterior da aplicação.

`04_p1.sql` adiciona workflow de finding (responsável e prazo), dedupe de alerta, privilégio do papel auditor, GUCs fechados e colunas de job, lag de CAGG e tamanho antes/depois da compressão. Alertas são opcionais: `AUDITOR_ALERT_WEBHOOK_URL` e/ou `AUDITOR_ALERT_EMAIL_TO` com `AUDITOR_ALERT_SMTP_HOST`. Sem URL e sem SMTP, nada é enviado. Falha de envio não falha o run. O payload não leva DSN, senha nem SQL cru. Coletores de catálogo ficam no `statement_timeout` curto do alvo; estatística, chunks, jobs e workload usam 120s locais para não estourar falso timeout em banco grande.

## Produção

Copie [deploy/env.prod.example](../deploy/env.prod.example) para `.env.prod` na raiz do repositório. Preencha a senha do snapshot store, a senha inicial do operador e os dois slots `AUDITOR_TARGET_1_*` e `AUDITOR_TARGET_2_*`.

```bash
podman compose -f deploy/compose.prod.yaml --env-file .env.prod up -d
```

O compose falha se faltarem `POSTGRES_PASSWORD`, `AUDITOR_BOOTSTRAP_USER`, `AUDITOR_BOOTSTRAP_PASSWORD`, `AUDITOR_CORS_ORIGINS` ou `AUDITOR_TARGET_ALLOWED_HOSTS`. Os DSNs não entram no YAML. O Postgres interno não publica porta. `GET /metrics` não passa pelo Caddy; o scrape fica em `api:9090`, dentro da rede do compose.
Mantenha `VITE_API_BASE_URL=/` e encaminhe `/api/*` pelo proxy HTTPS da mesma origem da interface. Os arquivos Caddy do projeto definem CSP, bloqueio de MIME sniffing e política de referência; ao usar outro proxy, configure cabeçalhos equivalentes. O Compose fixa o Caddy em `172.30.72.10/32` e somente esse endereço pode informar `X-Forwarded-For` e `X-Forwarded-Proto` à API. Se usar outro proxy, ajuste `AUDITOR_TRUSTED_PROXY_CIDRS` para os endereços exatos dele e faça o proxy substituir cabeçalhos de origem enviados pelo cliente. Sem proxy confiável, a API usa o IP da conexão e marca o cookie como `Secure` apenas com TLS direto. Escolha outra sub-rede no Compose se `172.30.72.0/24` já estiver em uso na VPS. A sessão da UI usa cookie HttpOnly e cabeçalho CSRF para alterações; o proxy deve preservar `Set-Cookie`, `Cookie` e `X-CSRF-Token`. A API cria e atualiza a tabela compartilhada de contagem de login em volumes existentes no boot, sem reaplicar o baseline. Depois de quatro tentativas de uma conta em cinco minutos, aplica espera progressiva de 1 a 60 segundos; também limita por conta e IP. As respostas 429 incluem `Retry-After`, usam texto genérico e aparecem em `auditor_http_requests_total`. Os contadores guardam hashes, não nomes nem senhas. Confira o impacto em usuários atrás de NAT antes de ajustar limites. O limite de login fica nessa tabela compartilhada (até 600 tentativas por IP por minuto e 10 por conta a cada cinco minutos) e sobrevive a reinício e a réplica. Além dele, cada processo limita 600 chamadas autenticadas por usuário por minuto e 20 criações de execução ou relatório por IP por minuto; esses dois tetos são locais ao processo e não substituem o limite de login.
O Postgres do Compose aplica scripts deste diretório **somente na primeira inicialização** do volume.

Após alterar o baseline em desenvolvimento:

```bash
make reset-volume   # destrutivo
make up
```

**Nome do banco interno:** default `db_auditor` (`.env` / `POSTGRES_DB`). Se o volume foi criado com o nome legado `timescale_auditor`, alinhe o `.env` ou recrie o volume.

Detalhes: `backend/migrations/README.md`.

## Adicionar um ambiente MongoDB

Crie um ambiente no snapshot store após aplicar as migrações e guarde o ID retornado:

```sql
INSERT INTO audit_environment (name, type, discovery_mode, engine)
VALUES ('MongoDB de produção', 'self_hosted', 'single_database', 'mongodb')
RETURNING id;
```

Configure o slot `AUDITOR_TARGET_3_*` do [exemplo de produção](../deploy/env.prod.example) com esse ID, banco, host, porta e usuário. Acrescente o host exato a `AUDITOR_TARGET_ALLOWED_HOSTS` e reinicie a API. O conector aceita um host direto (`mongodb://`), TLS verificado e um banco explícito; ele não segue descoberta de réplica ou URI `mongodb+srv`. Crie uma conta com apenas as permissões necessárias para `listCollections`, `listIndexes` e `collStats`. A coleta lê somente metadados, limita a 1.000 coleções e 20.000 índices por execução e não lê documentos. Se o banco ultrapassar esses limites, a execução falha com motivo e exige um escopo menor. As regras PostgreSQL, a nota de saúde e o diagnóstico de qualidade de dados aparecem como não aplicáveis ao MongoDB.

Não cole a URI com senha em logs ou tickets. Faça backup do snapshot store antes de mudar a configuração ou migrar ambientes existentes. Desativar o ambiente ou remover o slot interrompe novas coletas; os snapshots anteriores permanecem no store.

## Observabilidade (Sprint 10)

| Endpoint | Uso |
|----------|-----|
| `GET /health` | Liveness |
| `GET /ready` | Readiness (snapshot store) |
| `GET /metrics` | Prometheus, só na porta interna 9090 |
| `GET /api/v1/status` | Visão operacional (UI Status) |

Logs da API são **JSON estruturados** por padrão (`AUDITOR_LOG_FORMAT=json`).  
Use `AUDITOR_LOG_FORMAT=text` e `AUDITOR_LOG_LEVEL=debug` em desenvolvimento.

Cada resposta HTTP inclui `X-Request-ID` (ou propaga o valor enviado pelo cliente) para correlacionar logs e traces manuais.

Erros de conexão a targets **não** incluem senha (ver `config.SanitizeDSN`). Warnings parciais multi-database aparecem no `collector_run.warning`.

### Métricas principais

- `auditor_up`
- `auditor_http_requests_total{method,path,code}`
- `auditor_http_request_duration_seconds_{sum,count}`
- `auditor_audit_runs_total` / `auditor_audit_runs_failed_total`

Paths com UUIDs são normalizados para `:id` para manter cardinalidade baixa.

OpenTelemetry e Sentry são opcionais (P1): configure DSN/endpoints via env quando forem adotados — o núcleo atual não exige dependências externas de APM.

## Deploy com Podman Compose

1. Crie `.env.prod` (fora do git) com `POSTGRES_PASSWORD`, `AUDITOR_DOMAIN`, etc.
2. Build e subida:

```bash
podman compose -f deploy/compose.prod.yaml --env-file .env.prod up -d --build
```

3. Caddy termina TLS (quando o domínio aponta para a VPS) e encaminha:
   - `/api/*`, `/health`, `/ready` → API
   - `/metrics` não é publicado; scrape em `api:9090`
   - resto → frontend estático

### Volumes e secrets

- Volume `snapshot-store`: dados do PostgreSQL interno (backup/restore = dump deste volume ou `pg_dump`).
- Senhas apenas em env files locais ou secret managers; nunca no repositório.
- Rede `auditor` é privada; Postgres **não** publica porta no compose de produção.
- Bancos **auditados** (Tiger Cloud / self-hosted): connection strings só no env da API, credenciais **read-only**.

### Health e restart

- `restart: unless-stopped` em todos os serviços long-running.
- Healthchecks de Postgres e API controlam dependências de startup.

## Backup do snapshot store

O serviço `snapshot-backup` do compose de produção roda `pg_dump -Fc` uma vez por dia para o volume `snapshot-backup`, separado de `snapshot-store`. A retenção padrão é 14 dias (`BACKUP_RETENTION_DAYS`). Os DSNs dos Timescale auditados não entram no dump: ficam só no ambiente da API.

Cada sucesso grava `snapshot_backup.last_success_at`. A métrica `auditor_snapshot_backup_age_seconds` é a idade dessa marca. Sem nenhum sucesso a idade parte do epoch e continua crescendo, então um job parado não aparece como saudável.

Restore curto, com a API parada:

```bash
podman compose -f deploy/compose.prod.yaml --env-file .env.prod stop api
podman compose -f deploy/compose.prod.yaml --env-file .env.prod exec -T postgres \
  pg_restore -d "$POSTGRES_DB" --clean --if-exists /dev/stdin < /caminho/do.dump
podman compose -f deploy/compose.prod.yaml --env-file .env.prod start api
```

O dump não contém senha de alvo. Confira o `.env.prod` à parte antes de subir de novo.

1. `GET /health` → `ok`
2. `GET /ready` → `ready`
3. De dentro da rede do Compose, `GET http://api:9090/metrics` contém `auditor_up 1`. A porta 9090 não é publicada no host. O mesmo caminho em 8080 responde 404.
4. UI **Status** lista API, store e runs
5. Logs JSON incluem `request_id` e `duration_ms`
6. UI **Documentação** descreve o fluxo de configuração via `.env`

## Retenção e restauração

O prazo do PDF fica em `auditor_setting.report_retention_days` (padrão 30). Um artefato vencido não é mais baixado; a linha do job permanece como trilha. Uma atualização não apaga achados, execuções nem jobs.

Backup e criptografia em repouso são da infraestrutura: use `pg_dump` do snapshot store e o volume criptografado do operador. O auditor não executa `DROP` nem `DELETE` de histórico durante o upgrade.

