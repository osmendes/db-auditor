# Exemplos de PDF (dados sintéticos)

Gerados a partir dos golden files de `backend/internal/report/golden_test.go`, sem dados reais:

- `empty.pdf` — executivo sem achados;
- `partial.pdf` — técnico de execução parcial, com lacuna no gráfico, matriz de prioridade, impacto com unidade/período, hipótese/ganho previsto/ganho medido e custos de manutenção;
- `long.pdf` — técnico com 40 achados e evidência longa;
- `table.pdf` — por tabela, com referência e regressão.

O caso volumoso (5000 tabelas, 2000 achados, 162 páginas) é verificado pelo resumo em `testdata/volumous.golden.txt` e não é versionado aqui.

Os PDFs embutem um subconjunto da fonte Liberation Sans (`pdffonts` mostra `emb yes sub yes`). Também têm título nos metadados (Info e XMP) e são marcados para leitor de tela: `pdfinfo` mostra `Tagged: yes`, e `pdfinfo -struct` lista a árvore Document › H1/H2/H3/P (#107).

Para regenerar:

```bash
cd backend
AUDITOR_REPORT_VARIANTS_DIR=/tmp/pdfs go test ./internal/report -run Golden -count=1
```
