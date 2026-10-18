# Regras e heurísticas — Sprint 16

O catálogo `rule_catalog` é semeado na inicialização da API. Cada combinação
`rule_id`/`rule_version` é imutável: alterar critérios, texto ou parâmetros
exige nova versão. A API recusa iniciar se detectar uma versão existente com
conteúdo diferente. Findings persistem a versão, confiança, parâmetros,
recomendação, validação e referências usados na análise. Mudança de versão ou
de política gera outra identidade de deduplicação, sem reescrever o histórico.

## Configuração por ambiente e schema

`rule_policy` guarda overrides. `schema_name=''` vale para todo o ambiente;
uma linha para um schema específico tem precedência. A API de leitura é
`GET /api/v1/environments/{id}/rules?schema=public`, que retorna catálogo,
estado habilitado e parâmetros efetivos. Não há endpoint de escrita enquanto
o produto não tiver autenticação/RBAC (planejados para a Sprint 20). Um
operador do banco interno pode alterar a política com SQL controlado:

```sql
INSERT INTO rule_policy(environment_id,schema_name,rule_id,enabled,parameters)
VALUES ('<environment-id>', 'public', 'model.wide_table', true,
        '{"min_columns":70}'::jsonb)
ON CONFLICT(environment_id,schema_name,rule_id)
DO UPDATE SET enabled=EXCLUDED.enabled,
              parameters=EXCLUDED.parameters,updated_at=now();
```

Somente chaves do catálogo são aplicadas. Os parâmetros de contagem, tamanho,
idade e razão usam números positivos. Para `model.type_review` existem
`check_money`, `check_timestamp_without_timezone` e `check_status_text`.
`model.naming_inconsistent` aceita `snake_case`, `lowercase` ou `none`.

## Interpretação e limites

- Integridade: ausência de PK, FK sem índice de suporte, constraint não
  validada, tipo de FK divergente, sequence sem ownership e default apontando
  para sequence de outra coluna são sinais para
  investigação. PK em partições é ignorada; índice parcial ou inválido não
  conta como suporte universal de FK. Nunca se cria FK/PK automaticamente.
- Índices: duplicidade estrutural, prefixo, indisponibilidade, excesso sob
  escrita e carga de leitura sem índice geram hipóteses. Predicados,
  unicidade, `INCLUDE`, expressão e plano real podem invalidar a hipótese.
  Nenhuma regra recomenda `DROP` automático.
- Modelagem: largura, colunas numeradas opcionais, entidades semelhantes
  entre schemas, relacionamento sugerido por nome, tipos, JSONB e convenção
  de nomes têm confiança inferior às regras determinísticas. Nomes e
  estatísticas não revelam intenção de negócio.
- Manutenção: dead tuples e bloat são **estimativas**; `last_analyze` pode
  estar ausente ou desatualizado. Confirmar com métricas de autovacuum e
  diagnóstico autorizado antes de mudanças.
- Workload: a correlação com tabela exige uma referência qualificada única.
  Fingerprints não armazenam texto SQL nem literais. Contadores cobrem a
  janela desde `stats_reset`; reset, baixa amostra e consultas complexas
  reduzem confiança.

Para validar um finding, conferir o run e cobertura, revisar evidências,
comparar planos e métricas durante uma janela suficiente e testar qualquer
mudança em ambiente controlado. O auditor não executa DDL nem exclusões.

## Texto por regra e consulta de confirmação (AN-01, AN-05)

Toda regra do catálogo tem constatação, evidência, impacto, próximo passo e
uma consulta de confirmação próprios (`backend/internal/actions/catalog.go`).
A consulta é um único `SELECT` sobre `pg_catalog`, `information_schema`,
`pg_stat_statements` ou `timescaledb_information`, com `LIMIT`, parâmetros
`$n` documentados, mecanismo, capacidade exigida, permissão mínima, limite de
linhas (100), timeout (5000 ms) e aviso de impacto. O auditor **não** executa
a consulta; ela é para revisão humana em transação `READ ONLY`. O teste
`TestEveryCatalogRuleHasFiveFieldsAndASafeQuery` recusa DML/DDL, funções com efeito colateral,
subconsultas em `FROM` e tabelas fora do catálogo.

## Impacto conservador (AN-03)

O impacto só é estimado quando há valor, unidade, denominador e período
comparáveis (`impact_estimate` no achado e no plano de ação). Exemplos: bytes
do índice ÷ bytes de todos os índices da tabela na janela de `idx_scan`;
tuplas mortas ÷ (mortas + vivas) desde o último (auto)vacuum — **estimativa,
não bloat medido**; leituras de disco ÷ leituras totais na janela de
observação; bytes da tabela ÷ bytes do banco na coleta. Sem esses dados o
impacto é "não estimado" e o motivo é exibido. Os campos voláteis ficam em
`evidence.impact_basis` e não entram na assinatura de descarte.

## Priorização (AN-02)

Fórmula `priority-v1`:
`pontuação = peso(severidade) × fator(impacto) × max(confiança, 0,3) ÷ fator(esforço)`.
Pesos: crítica 5, alta 4, média 3, baixa 2, informativa 1. Impacto medido alto
1,5, médio 1,2, baixo 1,0; impacto não estimado 0,8 (sem métrica o impacto
nunca eleva a prioridade). Esforço baixo 1, médio 1,5, alto 2. Máximo 7,5.
Confiança ≥ 0,75 é alta, ≥ 0,5 média, abaixo disso baixa. Empates seguem
severidade e ID do achado. `GET /api/v1/findings/priorities` devolve os cinco
primeiros entre até 500 achados abertos; Dashboard, Findings e PDF usam a
mesma fórmula e mostram o ID do achado.

## Baseline e regressões (AN-11)

Uma regressão só é registrada quando:

1. a execução atual e a anterior estão completas (status `success`, cobertura
   sem falhas e análise concluída) — qualquer lado parcial silencia;
2. o intervalo entre elas é de no máximo `AUDITOR_REGRESSION_MAX_GAP`
   (padrão 168h, mínimo 1h) e não há execução parcial, falha ou cancelada no
   meio (lacuna);
3. para achados e consultas, o reset de contadores não mudou entre as duas
   coletas: `stats_reset` das tabelas (`pg_stat_database`) e o de
   `pg_stat_statements`. A passagem de `NULL` (cluster nunca resetado) para um
   valor, e o inverso, também conta como reset;
4. a referência (baseline) é completa, anterior e com o mesmo perfil, versão
   de coletor e manifesto de regras;
5. o limiar é **persistente**: a anterior **e** a atual ficam acima da
   referência. Um pico isolado não alerta. Por categoria:
   - `findings`: achados altos/críticos observados acima do total da
     referência;
   - `structure`: a mesma tabela removida ou alterada (tipo, chave primária,
     número de colunas), ou o mesmo índice removido, nas duas coletas;
   - `workload`: a mesma consulta lenta ou pesada (`performance.slow_query`,
     `performance.workload_*`) nova em relação à referência nas duas coletas.
     Esses achados são de severidade baixa e não entram em `findings`.

Deduplicação por identidade: cada alerta guarda as chaves do que alertou
(`dedup_key` dos achados, tabelas, índices; até 1000, com o total em
`keys_total`). Para a mesma referência e categoria, enquanto o episódio dura,
um novo alerta só é criado se aparecer algo que o último não tinha (um achado,
tabela, índice ou consulta diferente), mesmo com contagem menor ou igual.
Reconhecer um alerta silencia a repetição do mesmo conteúdo. O episódio acaba
quando uma coleta completa e analisada entre o último alerta e a atual mostra a
regressão resolvida (achados de volta ao total da referência; nenhuma das
chaves alertadas ainda regredida). Depois disso, a mesma regressão volta a
alertar, com `previous_alert_recovered` na evidência. Alertas antigos, sem
chaves, seguem a regra anterior (só alerta se a contagem piorar). A notificação
de novos achados segue a mesma regra de lacuna. O motivo do silêncio
(`partial_collection`, `gap_window`, `intermediate_incomplete_run`,
`stats_reset`, `below_threshold`, `deduplicated`…) é registrado em log de
depuração.

Calibração (BA-15): o limiar padrão vem de coletas sintéticas nos testes
`TestRegressionNoiseControlIntegration`. Registre a calibração com coletas
reais seguindo o procedimento em [ops.md](ops.md#calibração-de-regressões-ba-15).

## Tendência e manutenção (BA-21)

`maintenance.growth_trend` usa só duas coletas completas e comparáveis com no
máximo 14 dias de intervalo. Coleta parcial, lacuna, reset de contador ou
mecanismo sem estatística viram achado informativo ou nenhum achado. Opções
de particionamento, compressão (TimescaleDB), retenção e revisão de
autovacuum/ANALYZE expõem pré-condições, custo de leitura, escrita e
armazenamento, janela, risco e validação posterior. Retenção exige aprovação
de negócio/compliance e backup restaurável. Nenhuma recomendação propõe
exclusão, `DROP` ou alteração automática. A interface e o PDF separam
**hipótese**, **ganho previsto** e **ganho medido**; o ganho medido só aparece
com uma medição comparável registrada após nova coleta completa.

## Relatórios PDF (BA-11)

Os PDFs executivo, técnico e de tabela mostram cobertura, período, unidade,
matriz impacto × esforço × confiança × risco com ID do achado, hipótese/ganho
e custos. Coletas parciais aparecem como lacuna no gráfico de armazenamento e
nunca entram na variação. Limites: 500 bancos, 5000 tabelas, 2000 achados,
detalhamento dos 100 achados de maior prioridade, 200 páginas, 16 MiB, 8 MiB
de texto e `AUDITOR_REPORT_MAX_SECONDS` (padrão 120, 10–900) de geração. Ao
exceder, o job falha com mensagem explícita; truncamento, limite de detalhe e
cobertura parcial aparecem no PDF e em `limit_note` do job. Os golden files
ficam em `backend/internal/report/testdata` (`go test ./internal/report -run
Golden -update` para revisar mudanças).
