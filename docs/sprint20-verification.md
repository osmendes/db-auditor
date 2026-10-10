# Verificação do Sprint 20

O Sprint 20 não é reimplementado aqui. A evidência está nos testes e nas rotas que já existem na linha principal.

- Paginação de achados, inventário e execuções: `ListFindingsPage`, `ListFindingsCategoryPage` e as páginas do frontend com 20/50/100/todos.
- Totais do dashboard: KPIs usam a agregação do snapshot, não a soma da página visível.
- Escala: `sprint20_scale_integration_test.go` só roda com `AUDITOR_TEST_SCALE=1` contra Postgres e Mongo descartáveis. Não faz parte do CI padrão.
- O que ainda era lacuna do sprint entrou nas ondas anteriores: rotas no fragmento, uma lista de achados, cobertura parcial visível e retenção de relatório.

Nenhum coletor passou a escrever no banco auditado.
