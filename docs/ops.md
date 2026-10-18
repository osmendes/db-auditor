# Operação em VPS — DB Auditor

## Schema do snapshot store

O schema canônico está em `backend/migrations/01_baseline.sql`. Os ambientes iniciais vêm de `AUDITOR_TARGET_N_*` no `.env`: a API grava nome e id depois das migrations. A API aplica o baseline só quando o banco está vazio e registra `schema_migration`. Um volume que já tem o schema atual recebe só o que falta (`03_audit_schedule.sql` até `25_role_attributes_regression_workload.sql`) e não reaplica o baseline. O segundo boot não executa SQL de novo. Se o checksum de um arquivo já aplicado mudar, a API recusa subir. As notas de cada etapa (migrações, limites e decisões) estão em [notas-por-etapa.md](notas-por-etapa.md).

Antes de atualizar uma instalação persistente, faça backup do snapshot store e confira a restauração. A migração `12_view_detail.sql` acrescenta campos sem apagar visões existentes. Depois do deploy, execute uma nova coleta de PostgreSQL para preencher colunas e opções de segurança: execuções antigas mostram esses campos como não coletados. Para voltar à versão anterior, pare a API nova e restaure o backup ou use a versão anterior do aplicativo; não remova as novas colunas manualmente.

A migração `13_index_detail.sql` também preserva os snapshots antigos. Uma nova coleta preenche as colunas incluídas e informa se a fonte de estatísticas de uso do índice estava disponível. Zero leituras em um único snapshot não prova que um índice é dispensável: confira o período de coleta, o reinício das estatísticas e o histórico antes de qualquer alteração manual no banco auditado.

A migração `14_function_detail.sql` acrescenta dados de retorno, segurança e métricas agregadas das funções sem remover snapshots. Ela diferencia funções com o mesmo nome pela assinatura. A lista de papéis com execução efetiva é consultada por página; configurações e corpo SQL não aparecem no contrato de detalhe. Para voltar à versão anterior, preserve o backup do snapshot store e use a versão anterior da API.

A migração `15_cagg_source.sql` acrescenta campos opcionais de origem e intervalo de bucket aos agregados contínuos. As coletas antigas continuam legíveis e exibem “não coletado” nesses campos. Após atualizar, faça uma nova coleta Timescale para preenchê-los quando a versão instalada oferecer os metadados. O SQL da definição permanece protegido; nenhuma alteração é feita no banco auditado. Para desfazer a implantação, pare a API nova e restaure o backup do snapshot store ou use a versão anterior da aplicação.

`04_p1.sql` adiciona workflow de finding (responsável e prazo), dedupe de alerta, privilégio do papel auditor, GUCs fechados e colunas de job, lag de CAGG e tamanho antes/depois da compressão. Alertas são opcionais: `AUDITOR_ALERT_WEBHOOK_URL` e/ou `AUDITOR_ALERT_EMAIL_TO` com `AUDITOR_ALERT_SMTP_HOST`. Sem URL e sem SMTP, nada é enviado. Falha de envio não falha o run. O payload não leva DSN, senha nem SQL cru. Coletores de catálogo ficam no `statement_timeout` curto do alvo; estatística, chunks, jobs e workload usam 120s locais para não estourar falso timeout em banco grande.

### Checksum antigo da migração 25

`25_role_attributes_regression_workload.sql` mudou durante a revisão do PR #132. A primeira versão (commit `3245036`) só criava `is_superuser`, `bypass_rls` e `replication` em `account_role_snapshot`. A versão que entrou na `main` (`35ee520`, igual à do merge `a964ac2`) acrescenta `create_role` e `inherited_privileges`. Só é afetado um snapshot store em que a API do branch do PR #132 subiu entre esses dois commits (homologação ou QA). Instalações que só rodaram versões da `main` não precisam fazer nada.

| versão de `25_role_attributes_regression_workload.sql` | sha256 |
| --- | --- |
| antiga (`3245036`, só no PR) | `3d3512136baa8b30ce931e6242c6a570050a479c3eae78d08a612d79dda73545` |
| atual (`35ee520` / `main`) | `023d0d2758fbd17784f32a48de03dd70b4ef7828e71db83b97078b11ce0d2b9f` |

**Sintoma.** No boot, `migrate.Apply` (`backend/internal/migrate`) compara o sha256 do arquivo com `schema_migration.checksum` e para na primeira diferença. A API sai com `could not apply snapshot store migrations` e `error="checksum of 25_role_attributes_regression_workload.sql changed after it was applied"`. As migrações depois da 25 também não rodam.

**Como detectar.** Antes de atualizar, ou depois da falha (`$POSTGRES_USER`/`$POSTGRES_DB` como no `.env.prod`):

```bash
sha256sum backend/migrations/25_role_attributes_regression_workload.sql
podman compose -f deploy/compose.prod.yaml --env-file .env.prod exec -T postgres \
  psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At \
  -c "SELECT checksum FROM schema_migration WHERE version = '25_role_attributes_regression_workload.sql'" \
  -c "SELECT string_agg(column_name, ',' ORDER BY column_name) FROM information_schema.columns
      WHERE table_name = 'account_role_snapshot'
        AND column_name IN ('is_superuser','bypass_rls','replication','create_role','inherited_privileges')" \
  -c "SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = 'regression_alert_category_check'"
```

O checksum `3d3512…545` com `create_role` e `inherited_privileges` ausentes confirma a versão antiga. Outro checksum que não seja nenhum dos dois da tabela vem de um arquivo alterado (por exemplo, fim de linha CRLF no checkout): não aplique o procedimento abaixo, investigue o arquivo.

**Correção segura.** **Não** basta trocar só o checksum: o schema antigo não tem as duas colunas novas, e a coleta de roles e a regra `security.powerful_role` falham sem elas. O arquivo atual é idempotente (`ADD COLUMN IF NOT EXISTS`, `DROP CONSTRAINT IF EXISTS` antes do `ADD CONSTRAINT`), só acrescenta colunas com default e não altera nem apaga linhas. Com a API parada e um backup recente (veja [Backup do snapshot store](#backup-do-snapshot-store)), use **uma** das opções:

1. Deixar a API reaplicar: apague só o registro antigo e suba a API. O migrador vê a 25 como pendente, executa o arquivo atual numa transação e grava o checksum novo.

   ```sql
   DELETE FROM schema_migration
    WHERE version = '25_role_attributes_regression_workload.sql'
      AND checksum = '3d3512136baa8b30ce931e6242c6a570050a479c3eae78d08a612d79dda73545';
   ```

2. Aplicar à mão e então atualizar o registro (o `UPDATE` só depois de o `psql -1` terminar sem erro):

   ```bash
   podman compose -f deploy/compose.prod.yaml --env-file .env.prod exec -T postgres \
     psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -1 \
     < backend/migrations/25_role_attributes_regression_workload.sql
   ```

   ```sql
   UPDATE schema_migration SET checksum = '023d0d2758fbd17784f32a48de03dd70b4ef7828e71db83b97078b11ce0d2b9f'
    WHERE version = '25_role_attributes_regression_workload.sql'
      AND checksum = '3d3512136baa8b30ce931e6242c6a570050a479c3eae78d08a612d79dda73545';
   ```

Depois, rode de novo a consulta de detecção: o checksum deve ser `023d0d…b9f`, as cinco colunas devem existir e `regression_alert_category_check` deve aceitar `workload`. Suba a API e confira `GET /ready`. Snapshots antigos ficam com `create_role = false` e `inherited_privileges = '{}'` até a próxima coleta de PostgreSQL. Não existe comando da API para reconciliar checksums. A correção é feita só por SQL no snapshot store, como descrito em `backend/migrations/README.md`.

## Produção

Copie [deploy/env.prod.example](../deploy/env.prod.example) para `.env.prod` na raiz do repositório. Preencha a senha do snapshot store, a senha inicial do operador e os dois slots `AUDITOR_TARGET_1_*` e `AUDITOR_TARGET_2_*`.

```bash
podman compose -f deploy/compose.prod.yaml --env-file .env.prod up -d
```

O compose falha se faltarem `POSTGRES_PASSWORD`, `AUDITOR_BOOTSTRAP_USER`, `AUDITOR_BOOTSTRAP_PASSWORD`, `AUDITOR_CORS_ORIGINS` ou `AUDITOR_TARGET_ALLOWED_HOSTS`. Os DSNs não entram no YAML. O Postgres interno não publica porta. `GET /metrics` não passa pelo Caddy; o scrape fica em `api:9090`, dentro da rede do compose.
Mantenha `VITE_API_BASE_URL=/` e encaminhe `/api/*` pelo proxy HTTPS da mesma origem da interface. Os arquivos Caddy do projeto definem CSP, bloqueio de MIME sniffing e política de referência; ao usar outro proxy, configure cabeçalhos equivalentes. O Compose fixa o Caddy em `172.30.72.10/32` e somente esse endereço pode informar `X-Forwarded-For` e `X-Forwarded-Proto` à API. Se usar outro proxy, ajuste `AUDITOR_TRUSTED_PROXY_CIDRS` para os endereços exatos dele e faça o proxy substituir cabeçalhos de origem enviados pelo cliente. Sem proxy confiável, a API usa o IP da conexão e marca o cookie como `Secure` apenas com TLS direto. Escolha outra sub-rede no Compose se `172.30.72.0/24` já estiver em uso na VPS. A sessão da UI usa cookie HttpOnly e cabeçalho CSRF para alterações; o proxy deve preservar `Set-Cookie`, `Cookie` e `X-CSRF-Token`. A tabela compartilhada de contagem de login vem das migrações versionadas (`16_identity.sql` e `21_identity_login_block.sql`), aplicadas no boot em volumes existentes sem reaplicar o baseline. Depois de quatro tentativas de uma conta em cinco minutos, aplica espera progressiva de 1 a 60 segundos; também limita por conta e IP. As respostas 429 incluem `Retry-After`, usam texto genérico e aparecem em `auditor_http_requests_total`. Os contadores guardam hashes, não nomes nem senhas. Confira o impacto em usuários atrás de NAT antes de ajustar limites. O limite de login fica nessa tabela compartilhada (até 600 tentativas por IP por minuto e 10 por conta a cada cinco minutos) e sobrevive a reinício e a réplica. Além dele, cada processo limita 600 chamadas autenticadas por usuário por minuto e 20 criações de execução ou relatório por IP por minuto; esses dois tetos são locais ao processo e não substituem o limite de login.
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

O worker grava o PDF em streaming: renderiza página a página direto num `COPY` para `report_artifact`, sem montar o arquivo inteiro na memória. Os dados também são lidos por seção (visão geral, achados, bancos, tabelas) de um único snapshot `REPEATABLE READ` somente leitura, e cada seção é liberada antes da próxima. O CI gera, dentro de um contêiner com 48 MiB e swap desligado, o maior relatório que cabe nos limites (cerca de 162 páginas) e a maior seção de achados (2 000, com detalhamento limitado). Cada job registra `peak_heap_bytes`, `pages` e `bytes` no log `report pdf rendered`. Se o heap do job passar de `AUDITOR_REPORT_MAX_HEAP_MB` (padrão 256), o job falha com o motivo "relatório excedeu o orçamento de memória do worker". Limites de 200 páginas, 16 MiB e 8 MiB de texto também falham com motivo legível. Dimensione o limite de memória do contêiner da API acima desse teto.

O PDF é autocontido e acessível (#107). Ele embute um subconjunto WinAnsi da Liberation Sans (OFL 1.1, em `backend/internal/report/fonts/`), que tem as métricas da Helvetica, e por isso a paginação não mudou. Também grava título no Info e no XMP, `Lang pt-BR` e uma árvore de estrutura: Document com H1/H2/H3/P na ordem de leitura. Cabeçalho, rodapé, fundo e barras ficam marcados como artefato. Para conferir: `pdffonts relatorio.pdf` (espera `emb yes sub yes`), `pdfinfo relatorio.pdf` (espera `Title:` e `Tagged: yes`) e `pdfinfo -struct relatorio.pdf`. A fonte e a estrutura acrescentam cerca de 40 KB por PDF, mais uns 100 bytes por linha do relatório; o caso volumoso de 162 páginas tem 2,2 MB, bem abaixo do teto de 16 MiB.

O prazo do PDF fica em `auditor_setting.report_retention_days` (padrão 30). Um artefato vencido não é mais baixado e é removido pela limpeza horária do worker; a linha do job permanece como trilha auditável. Uma atualização não apaga achados, execuções nem jobs.

### Política de retenção (BA-03)

A política fica em `retention_policy` e só um **operador** autenticado a lê ou altera:

| Campo | Padrão | Significado |
|---|---|---|
| `snapshot_retention_days` | 0 | 0 = manter todos os snapshots. Um valor > 0 só gera **contagem** no plano; a API nunca apaga snapshots. |
| `report_job_retention_days` | 90 (30–3650) | Prazo da trilha do job depois que o PDF venceu. |
| `owner_name` | `operator` | Responsável pela política. |
| `encryption_at_rest` | `required-external` | Como volumes, backups e artefatos são cifrados (texto da política aprovada). |
| `archive_note` | vazio | Onde ficam os backups arquivados e por quanto tempo. |
| `approved_by` / `approved_at` | vazio | Preenchidos pelo próprio `PUT`, que só grava com `"approve": true` (exatamente esse campo, minúsculo, com o literal `true`). Sem aprovação, com campo desconhecido ou com variação de caixa (`Approve`, `APPROVE`), a resposta é 400 e nada é gravado. Uma alteração feita fora da API depois da aprovação bloqueia a limpeza até nova aprovação. |

```bash
# ler / alterar e aprovar (sessão de operador; cookie + X-CSRF-Token da UI)
curl -sS -b cookies.txt https://auditor.exemplo/api/v1/retention-policy
curl -sS -b cookies.txt -H "X-CSRF-Token: $CSRF" -X PUT https://auditor.exemplo/api/v1/retention-policy \
  -d '{"snapshot_retention_days":0,"report_job_retention_days":180,"owner_name":"Maycon Mendes","encryption_at_rest":"LUKS2 no volume snapshot-store e snapshot-backup; dumps cifrados com age antes de sair da VPS","archive_note":"dumps diários por 14 dias; mensais por 12 meses no bucket X","approve":true}'
# plano (dry-run: só contagens e bloqueios)
curl -sS -b cookies.txt https://auditor.exemplo/api/v1/retention-policy/plan
```

**Nenhuma limpeza destrutiva** roda sem: política aprovada e não alterada depois da aprovação, backup verificado há no máximo 7 dias e restauração de teste bem-sucedida em ambiente isolado há no máximo 30 dias. `POST /api/v1/retention-policy/apply` exige `confirm=true` e a contagem exata `expected_report_jobs` do plano atual; remove apenas linhas de `report_job` encerradas (`success`, `failed`, `cancelled`), além do prazo, com PDF vencido e sem artefato guardado. Plano, verificação dos bloqueios e `DELETE` rodam na mesma transação, com a linha da política travada (`FOR UPDATE`); se a contagem removida divergir do esperado, tudo é desfeito. O log da aplicação e o `auditor_operation_log` registram quem aprovou, os prazos aprovados e quantas linhas foram removidas (sem o texto de criptografia nem notas). Snapshots (execuções, tabelas, achados) só saem por procedimento manual, depois de arquivados em backup verificado e com aprovação registrada.

### Backup verificado e restauração de teste (BA-10)

`deploy/snapshot-backup.sh` grava o dump e o arquivo `.sha256` ao lado. `deploy/snapshot-restore-verify.sh` faz o ensaio:

1. confere o sha256 do dump com o `.sha256` do backup e valida o arquivo com `pg_restore --list`;
2. restaura com `pg_restore --clean --if-exists --exit-on-error` em um Postgres **isolado** (recusa sem `RESTORE_TARGET_CONFIRM=isolated` e se o destino for igual a `SOURCE_DSN`);
3. mede o tempo de recuperação (RTO, em segundos);
4. conta `audit_run`, `table_snapshot`, `finding`, `finding_event`, `report_job`, `report_artifact`, `auditor_user` (por papel), `auditor_user_environment` (permissões por ambiente) e `schema_migration`, e compara com as contagens da origem;
5. escreve a evidência JSON (`succeeded=false` em qualquer falha) para registro.

Rollback: o ensaio nunca toca a origem. Se a restauração real for necessária e falhar, mantenha a API parada, restaure o dump anterior verificado (`*.dump` + `.sha256` do dia anterior) e só promova depois de o script terminar com sucesso.

### Passos do responsável (homologação, evidência a anexar na issue #106)

Execute com volume representativo (cópia do store de produção ou massa equivalente) em homologação. Os comandos rodam num host com `postgresql-client` da mesma versão major do servidor ou superior (18 no Compose); sem cliente local, use o próprio contêiner: `podman run --rm --network host -v ./deploy:/deploy:ro -v snapshot-backup:/backup -v "$PWD":/out postgres:18-alpine sh /deploy/snapshot-restore-verify.sh ...`.

```bash
# 0) janela calma: pare escritas na origem (worker/agendador) para que as contagens batam
podman compose -f deploy/compose.prod.yaml --env-file .env.prod stop api
# 1) backup + checksum
podman compose -f deploy/compose.prod.yaml --env-file .env.prod exec -T snapshot-backup sh /snapshot-backup.sh
# 2) contagens da origem no mesmo instante do dump
sh deploy/snapshot-restore-verify.sh counts "$SOURCE_DSN" > source-counts.json
podman compose -f deploy/compose.prod.yaml --env-file .env.prod start api
# 3) Postgres isolado (outro host/volume, mesma versão major ou superior)
podman run -d --name dba-restore -e POSTGRES_PASSWORD=x -p 55432:5432 postgres:18-alpine
TARGET_DSN=postgresql://postgres:x@127.0.0.1:55432/postgres
# 4) papéis globais (pg_dump não leva roles do cluster)
pg_dumpall --globals-only -d "$SOURCE_DSN" | psql "$TARGET_DSN"
# 5) ensaio
SOURCE_DSN="$SOURCE_DSN" RESTORE_TARGET_CONFIRM=isolated ENV_LABEL=staging \
  sh deploy/snapshot-restore-verify.sh verify /backup/db-auditor-AAAAMMDDTHHMMSSZ.dump "$TARGET_DSN" source-counts.json > restore-evidence.json
# 6) registrar a evidência (e um backup_verified com o mesmo sha256)
curl -sS -b cookies.txt -H "X-CSRF-Token: $CSRF" -X POST https://auditor.exemplo/api/v1/retention-policy/evidence -d @restore-evidence.json
jq '.kind="backup_verified"' restore-evidence.json | curl -sS -b cookies.txt -H "X-CSRF-Token: $CSRF" -X POST https://auditor.exemplo/api/v1/retention-policy/evidence -d @-
```

Anexe na issue: `restore-evidence.json`, `source-counts.json`, saída do passo 5 (RTO), saída do `GET /api/v1/retention-policy/plan` depois do registro, e o login de um usuário restaurado na UI apontada para o destino (prova de que permissões e ambientes vieram juntos).

### Criptografia, chaves, acesso e descarte (BA-16)

O auditor não cifra disco nem guarda chave. Para a infraestrutura usada, registre na política (`encryption_at_rest`) e anexe na issue:

- **Volumes** `snapshot-store` e `snapshot-backup`: `lsblk -o NAME,TYPE,FSTYPE,MOUNTPOINT` e `cryptsetup status <mapeamento>` (LUKS) ou a tela/CLI do provedor mostrando o volume cifrado (`aws ec2 describe-volumes --query 'Volumes[].Encrypted'`, equivalente no provedor).
- **Backups fora da VPS**: cifre o dump antes de enviar (`age -r <chave-publica> -o dump.age dump`) ou use bucket com SSE-KMS; anexe a configuração do bucket (`aws s3api get-bucket-encryption`) ou o comando `age` usado.
- **Chaves**: quem guarda a chave LUKS/age/KMS, onde (cofre), rotação e quem pode ler. A chave nunca fica no Compose, no `.env.prod` nem no repositório.
- **Acesso**: diretório `/backup` com dono do serviço e modo `0700`; dumps e `.sha256` com modo `0600`. O `deploy/snapshot-backup.sh` aplica `umask 077`, corrige o modo do diretório e de dumps antigos a cada execução. Anexe `ls -ld /backup`, `ls -l /backup` e a lista de usuários do host com acesso.
- **Descarte**: dumps locais saem após `BACKUP_RETENTION_DAYS` (padrão 14) pelo `find -delete` do script; PDFs vencidos saem pela limpeza horária; para mídia/volume descartado, `cryptsetup luksErase` ou destruição de chave KMS. Registre o prazo aprovado em `archive_note`.

### Limites de consultas históricas (BA-03, #106)

Limites aplicados pela API:

| Endpoint | Janela | Paginação |
|---|---|---|
| `analytics/storage`, `analytics/findings-trends` | padrão 90 dias; máximo 90 dias, ou 7 dias com `granularity=hour`; acima disso, 400 | não paginam: no máximo um ponto por período da janela |
| `environments/{id}/history`, `environments/{id}/tables/history` | padrão 30 dias; mesmo teto de 90 dias (7 por hora) | `limit` padrão 50, de 1 a 500; `offset` ≥ 0; fora disso, 400. A página conta períodos, e a resposta traz `page` (`limit`, `offset`, `total`, `has_more`) |
| `environments/{id}/tables/workload`, `tables/column-stats` | mesmo teto de 90 dias | `workload` devolve no máximo 200 linhas |
| `audit-runs` | — | `limit` padrão 50, de 1 a 500; acima disso, 400 |

O teto aceita até 1 h a mais, para o cliente que calcula `from` = agora − 90 dias com o próprio relógio. Séries de 365 dias não ficam expostas: precisam de um agregado por coleta gravado na ingestão, que ainda não existe.

Consultas "últimas N coletas" aplicam o `LIMIT` em `audit_run` antes do join com `table_snapshot`. O crescimento do PDF e as séries paginadas seguem essa regra, e `TestHistoryQueriesLimitBeforeJoinIntegration` confere isso no `EXPLAIN`. Na homologação do #106 (1,3 milhão de linhas em `table_snapshot`), o join antes do `LIMIT` fez seq scan e levou 308 ms; com o `LIMIT` antes, foram 3,8 ms.

Meça e anexe, com o store de homologação restaurado:

```bash
psql "$TARGET_DSN" -c "SELECT relname, n_live_tup FROM pg_stat_user_tables WHERE relname IN ('audit_run','table_snapshot','finding','finding_event','report_job') ORDER BY 2 DESC;"
psql "$TARGET_DSN" -c "EXPLAIN (ANALYZE, BUFFERS) WITH recent AS (SELECT r.id, r.started_at FROM audit_run r WHERE r.environment_id='<env>' AND r.status IN ('success','partial_success') ORDER BY r.started_at DESC, r.id DESC LIMIT 6) SELECT recent.id, recent.started_at, s.bytes FROM recent CROSS JOIN LATERAL (SELECT sum(t.total_size_bytes) bytes FROM table_snapshot t WHERE t.environment_id='<env>' AND t.audit_run_id=recent.id) s ORDER BY recent.started_at DESC;"
/usr/bin/time -v curl -sS -o /dev/null -b cookies.txt "https://auditor.exemplo/api/v1/analytics/storage?environment_id=<env>&granularity=day"
curl -sS -o /dev/null -w '%{http_code}\n' -b cookies.txt "https://auditor.exemplo/api/v1/environments/<env>/tables/history?database=<db>&schema=<schema>&table=<tabela>&limit=501"   # espera 400
```

Registre cardinalidade, tempo e pico de memória (`Maximum resident set size`) e o limite operacional decidido. O plano da consulta de "últimas 6" não pode ter seq scan em `table_snapshot`.

### Calibração de regressões (BA-15)

Com pelo menos 10 coletas completas reais de um ambiente: liste os motivos de silêncio no log (`regression silenced`, nível debug) e os alertas criados em `regression_alert`; confira manualmente cada alerta (verdadeiro/falso positivo). Se a taxa de falso positivo passar de 1 em 10, ajuste `AUDITOR_REGRESSION_MAX_GAP` ao intervalo real do agendamento e anexe a tabela de calibração na issue #104.

## Recuperar o acesso de operador

Se todos os operadores perderem acesso, não apague `auditor_user`. Confirme um backup recente e restaurável do snapshot store e execute em terminal interativo na VPS:

```bash
podman compose -f deploy/compose.prod.yaml --env-file .env.prod exec -it api /auditor recover-operator --username NOME_DO_OPERADOR --backup-confirmed
```

O comando pede a nova senha sem eco, aceita somente um operador ativo já existente, revoga suas sessões e registra o evento `operator.password_recovery` sem guardar a senha. Teste o login e mantenha o registro do incidente. Nunca informe a senha em argumento, variável de ambiente, ticket ou log. Se não houver operador ativo, restaure o backup em ambiente isolado e investigue antes de modificar dados manualmente.
