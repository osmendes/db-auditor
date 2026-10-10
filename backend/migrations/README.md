# Snapshot store schema

O diretório contém o baseline, o seed local e migrações incrementais. O Postgres do Compose executa os `.sql` **somente na primeira inicialização** de um volume vazio, em ordem alfabética; a API registra e aplica as migrações incrementais pendentes em volumes existentes.

- `01_baseline.sql`: schema completo. As migrations das sprints 13–20 foram incorporadas aqui.
- `02_seed_demo.sql`: duas linhas locais de demonstração. A API não aplica este arquivo.
- `03_audit_schedule.sql`: agenda persistente. A API aplica no boot se ainda não estiver registrada.
- `04_p1.sql`: primeira ampliação incremental.
- `05_p2.sql`: melhorias da primeira etapa.
- `06_second_stage.sql`: planos de ação, eventos, anotações e regressões da segunda etapa.
- `07_third_stage.sql`: mecanismo do ambiente, diagnósticos agregados, decisões de saneamento e medições antes/depois. A constraint `audit_environment_engine_format` é criada de forma idempotente (reaplicável após apply parcial).
- `08_action_workload.sql`: registra a confirmação explícita de carga semelhante nas medições antes/depois; medições antigas permanecem como não confirmadas.
- `09_query_action_metrics.sql`: permite registrar latência média e leituras por mil chamadas de uma consulta observada, preservando as medições anteriores.
- `10_account_activity.sql`: guarda apenas validade e presença de sessão no instante da coleta para sugerir revisão de contas possivelmente inativas. Não guarda senhas nem comprova ausência de uso entre amostras.
- `11_quality_validation.sql`: vincula a decisão de validação de qualidade ao diagnóstico posterior comparável, preservando o vínculo também no evento.
- `12_view_detail.sql`: adiciona colunas, opções de segurança e estado de população às visões. Snapshots antigos ficam com valor ausente; uma nova coleta preenche os campos. A migração não remove dados.
- `13_index_detail.sql`: registra colunas incluídas e se as estatísticas de uso do índice estavam disponíveis. Em snapshots anteriores, esses campos ficam ausentes até uma nova coleta; os índices históricos permanecem preservados.
- `14_function_detail.sql`: registra retorno, configuração segura do caminho de busca, papéis com EXECUTE observado e estatísticas agregadas das funções. Execuções antigas mantêm esses campos como não coletados; o corpo SQL permanece protegido na API.
- `15_cagg_source.sql`: acrescenta origem e intervalo de bucket do agregado contínuo quando o catálogo Timescale os fornece. Snapshots antigos mantêm esses campos ausentes; uma nova coleta pode preenchê-los.

Não reintroduza arquivos `.down.sql`: o entrypoint executaria todos os `.sql`.

Depois de alterar o baseline em desenvolvimento:

```bash
make reset-volume   # destrutivo: apaga o volume snapshot-store
make up
```

Um volume que já existe **não** reaplica o baseline. A API cria as tabelas de login (`auditor_user`, sessão e log) se elas ainda não existirem, adiciona `blocked_until` ao contador de tentativas de login quando necessário e executa migrações incrementais pendentes. Faça backup antes de atualizar um volume persistente; confira o registro `schema_migration` e não altere arquivos já aplicados. Veja [docs/third-stage-operations.md](../../docs/third-stage-operations.md) para o procedimento de implantação e restauração.

Se o checksum de um arquivo **já aplicado** mudar (por correção de idempotência), a API recusa subir. Em desenvolvimento, atualize o registro após validar o schema:

```bash
sha256sum backend/migrations/07_third_stage.sql
# no snapshot store:
# UPDATE schema_migration SET checksum = '<hex>' WHERE version = '07_third_stage.sql';
```

Volumes em que a migração **falhou a meio** (objeto já existe, versão ausente em `schema_migration`) podem subir de novo após o pull: o SQL idempotente completa o que faltava e grava o checksum.

## Ambientes

`02_seed_demo.sql` não descreve os Timescale da empresa. Ele só nomeia, em volume vazio:

| id | nome |
| --- | --- |
| `00000000-0000-0000-0000-000000000001` | Demo Tiger Cloud |
| `00000000-0000-0000-0000-000000000002` | Demo Datacenter |

Host, porta, database, usuário e senha ficam em `AUDITOR_TARGET_N_*` no `.env`. O `ENVIRONMENT_ID` do slot tem de ser um `id` que já exista em `audit_environment`. Não crie outra migration de seed para os servidores reais: ela não roda de novo num volume existente e não deve carregar senha.
