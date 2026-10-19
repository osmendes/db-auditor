# Evidência da #135 — série de evolução do relatório de tabela

Dados sintéticos do teste `TestTableReportGrowthIsThatTableIntegration`
(`backend/internal/reportworker/table_growth_integration_test.go`), no mesmo
formato do achado do aceite visual da #107: duas coletas completas; em
`erp.public` o schema passa de 4 para 5 tabelas, e `log_evento` também existe
em `erp.auditoria` e no banco `outro`.

| Tabela | Coleta 1 | Coleta 2 |
| --- | --- | --- |
| `erp.public.log_evento` | 6 MiB | 9 MiB |
| `erp.public.pedido` / `cliente` / `produto` | 12 / 10 / 8 MiB | igual |
| `erp.public.nota_fiscal` | — | 2 MiB |
| `erp.auditoria.log_evento` | 3 MiB | 4 MiB |
| `outro.public.log_evento` | 20 MiB | 30 MiB |

## Era bug

A consulta da série (`reportGrowthSQL`, `backend/internal/repository/report_document.go`)
filtrava só `database_name`. Os filtros de schema e tabela do relatório não
entravam, então o relatório de `erp / public / log_evento` somava todas as
tabelas do banco `erp`.

- `antes-relatorio-tabela-log_evento-p1.png`: com a consulta antiga, 40894464 B
  em 5 tabelas → 47185920 B em 6 tabelas (todo o banco `erp`, os dois schemas).
- `relatorio-tabela-log_evento-p1.png` e `relatorio-tabela-log_evento.pdf`: com
  a correção, 6291456 B → 9437184 B, "em 1 tabela" (+50,0%), só
  `erp.public.log_evento`.

A legenda passou de "tamanho total das tabelas" para "tamanho total da tabela
do escopo" no relatório de tabela (e "das tabelas do schema/banco do escopo"
nos outros recortes). A legenda não cita nomes, então vale para todos os níveis
de redação.

## `note` em português

`relatorio-tecnico-erp-public-p2.png`: relatório técnico de `erp / public`, com
a evidência técnica do achado `index.unused` mostrando
`"note": "O auditor nunca recomenda DROP automaticamente; a remoção exige revisão humana."`
(antes: `"Never recommend DROP automatically"`).

## Como regenerar

```bash
cd backend
mkdir -p /tmp/evidencia-135
AUDITOR_REPORT_EVIDENCE_DIR=/tmp/evidencia-135 AUDITOR_TEST_DATABASE_URL=... AUDITOR_TEST_DISPOSABLE=1 \
  go test -count=1 -v -run TableReportGrowthIsThatTable ./internal/reportworker/
pdftoppm -r 110 -f 1 -l 1 -png -singlefile /tmp/evidencia-135/relatorio-tabela-log_evento.pdf relatorio-tabela-log_evento-p1
pdftoppm -r 110 -f 2 -l 2 -png -singlefile /tmp/evidencia-135/relatorio-tecnico-erp-public.pdf relatorio-tecnico-erp-public-p2
```

Os IDs de ambiente e execução mudam a cada geração.
