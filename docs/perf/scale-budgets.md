# Orçamentos de consulta em escala (PF-04, issue #64)

`TestScaleQueryBudgetsIntegration` (`backend/internal/repository/scale_budget_integration_test.go`) mede as consultas do inventário, dos KPIs e da fila de achados contra uma massa sintética. Ele usa o SQL real da `Store`, capturado com um `pgx.QueryTracer`. Roda só com `AUDITOR_TEST_DATABASE_URL`, `AUDITOR_TEST_DISPOSABLE=1` e `AUDITOR_TEST_SCALE=1`. No CI, o passo **Query budget report** publica as linhas `SCALE_BUDGET` no resumo do job e anexa os planos (`scale-query-plans`).

## Massa

| Ambiente | Execuções | Tabelas | Achados |
| --- | --- | --- | --- |
| alvo | 30 (1 em 10 falha) | 10 000 na última; 2×100×5 nas anteriores | 20 000 |
| ruído | 2 000 | igual | 80 000 |

O ambiente de ruído domina o volume. Assim, um plano que ignora `environment_id` paga pelo volume dos outros ambientes. A massa inclui `database_snapshot`, `audit_run_coverage`, `analysis_run` e `finding_event`. Depois do seed roda `ANALYZE`. A massa fica no banco descartável: no CI o teste roda duas vezes (suíte e passo de relatório), e a segunda execução, com o dobro de ruído, também precisa passar.

## Orçamento relativo, não absoluto

Um limite fixo em milissegundos só vale para a máquina em que foi medido. Por isso o teste calibra a CPU do próprio banco antes de medir: calcula o p95 de `SELECT sum(g) FROM generate_series(1,200000) g`. Cada consulta tem 30 amostras após aquecimento e

```
orçamento = clamp(fator × calibração, 25 ms, 2 s)
```

| Grupo | Consultas | Fator |
| --- | --- | --- |
| inventário | primeira página ordenada, última página, filtrado | 6 |
| KPIs | contagem de achados, últimas execuções, capacidades, último snapshot | 8 / 3 / 3 / 6 |
| fila de achados | primeira página aberta, página profunda (offset 5000), alta severidade, categoria segurança | 8 |

## Planos

`checkPlans` roda `EXPLAIN (FORMAT JSON)` em cada consulta capturada. O teste falha quando encontra:

- **Seq Scan** em relação com `reltuples ≥ 5000`. A exceção precisa constar em `scaleJustifiedSeqScans`, que hoje está vazia.
- **Índice esperado ausente.** As consultas que dependem de um índice o declaram em `scaleExpectedIndexes`:

| Consulta | Índice |
| --- | --- |
| fila aberta, primeira página | `finding_queue_order_idx` |
| fila de alta severidade | `finding_queue_order_idx` |
| categoria segurança | `finding_env_last_seen_idx` |
| últimas execuções | `audit_run_environment_started_at_idx` ou `audit_run_latest_idx` |
| capacidades | `audit_run_latest_idx` |

Uma regressão de índice quebra o CI.

## Índices novos

- `22_finding_queue_order_idx.sql`: `(environment_id, rank de severidade, confidence DESC, last_seen_at DESC, id::text DESC)`. É a ordem exata da fila.
- `23_finding_env_last_seen_idx.sql`: `(environment_id, last_seen_at DESC, id::text DESC)`. Atende as listas filtradas por categoria ou tipo.

Os dois usam `CREATE INDEX CONCURRENTLY IF NOT EXISTS`, um comando por arquivo. O migrador roda esses arquivos fora de transação. `internal/migrate/concurrent_index_integration_test.go` aplica cada arquivo duas vezes e confere `pg_index.indisvalid`.

## Antes × depois (PostgreSQL 17 local; calibração ≈ 21 ms)

| Consulta | Antes | Depois |
| --- | --- | --- |
| fila aberta, 1ª página (lista) | top-N heapsort sobre 8 000 linhas, 9,2 ms | Index Scan `finding_queue_order_idx`, 0,10 ms |
| categoria segurança (lista) | Index Scan no `finding_last_seen_idx` global + incremental sort, 0,49 ms | Index Scan `finding_env_last_seen_idx`, 0,12 ms |
| fila aberta, p95 da chamada | 13,9 ms | 3,8 ms (razão 0,18) |
| fila, página profunda (offset 5000) | external merge 2,3 MB, ≈ 29 ms | igual, p95 36,5 ms (razão 1,74) |
| inventário, última página | — | bitmap `table_snapshot_run_history_idx` + quicksort de 10 000 linhas, p95 22 ms (razão 1,05) |

A maior razão medida foi 1,74, na página profunda; o fator é 8. As demais ficaram abaixo de 0,6.

Os planos completos (`EXPLAIN (ANALYZE, BUFFERS)`) estão em [`plans/before`](plans/before) (sem as migrações 22/23) e [`plans/after`](plans/after). Para regerar: `AUDITOR_SCALE_PLAN_DIR=<dir> go test -p 1 -count=1 -v -run '^TestScaleQueryBudgetsIntegration$' ./internal/repository/`.

## Planos aceitos com justificativa

- **Página profunda da fila.** Com offset 5000, o planner prefere ordenar as cerca de 8 000 linhas abertas a percorrer 5 000 entradas do índice. É o plano certo para esse offset. A UI pagina a partir da primeira página, que usa o índice. O orçamento continua cobrindo o caso.
- **Inventário.** O inventário de uma execução lê todas as tabelas dela pelo `table_snapshot_run_history_idx` e ordena em memória (quicksort, 2,9 MB). A estimativa é de 650 linhas contra 10 000 reais, por causa da correlação entre execução e ambiente. Mesmo assim, 10 000 tabelas ficam em cerca de 1× a calibração. Paginação por keyset no inventário é outra mudança e não entrou aqui.

Os números do runner (PostgreSQL 18 no CI) aparecem no resumo do job **Backend (Go)** de cada PR.

## Memória do relatório (PF-07, issue #65)

O PDF é escrito página a página direto no `COPY` de `report_artifact`. Os dados são lidos por seção por `Store.OpenReportSections`: visão geral, achados, bancos e tabelas, todos de um único snapshot `REPEATABLE READ READ ONLY`. O renderizador (`report.WritePDFSections`) percorre as seções três vezes: contagens do resumo, contagem de páginas e escrita. Cada seção é carregada, diagramada e liberada antes da próxima. A saída é byte a byte idêntica ao renderizador de referência: o código anterior ao #65 fica copiado sem mudanças em `pdf_legacy_test.go`, com testes por nível de redação, com achados empatados e com filtros.

| Medição | Documento inteiro + renderizador legado | Por seção + streaming |
| --- | --- | --- |
| Pico de heap, ~190 páginas (5 600 tabelas) | ≈ 5,0 MB | ≈ 1,9 MB |
| Só o renderizador, mesmo documento já carregado | ≈ 4,5 MB | ≈ 1,1 MB |

**Limite real de contêiner.** O passo de CI *Report under a container memory limit* compila o teste do worker estaticamente e o roda em `alpine` com `docker run --memory 48m --memory-swap 48m`:

- Um contêiner de controle com os mesmos flags precisa ser morto por OOM. Isso prova que o limite é aplicado.
- `TestLargeReportUnderContainerMemoryLimit` se recusa a rodar se `memory.max` do cgroup não for o limite pedido ou se houver swap.
- O teste gera três relatórios:
  - o maior relatório que cabe nos limites: 501 bancos e 5 001 tabelas (ambos truncados) e 200 achados, cerca de 162 páginas (2,2 MB com fonte embutida e marcação, #107);
  - a maior seção de achados (2 001, truncada em 2 000): só os 100 primeiros da prioridade são detalhados (AN-12), então o relatório sai com cerca de 114 páginas e grava `limit_note`;
  - todas as seções no teto ao mesmo tempo: esse precisa falhar com "relatório excede 200 páginas".
- O passo falha se houver OOM, e o teste falha se `memory.peak` atingir o limite.

Localmente, num cgroup v2 de 48 MiB sem swap, o pico do cgroup foi ≈ 21 MiB (com a fonte embutida e a árvore de estrutura do #107). O resultado do runner aparece no resumo do job.
