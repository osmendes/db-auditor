# Virtualização de tabelas

As listas de inventário e achados paginam no servidor (20, 50 ou 100 linhas). A opção de carregar todas as linhas filtradas continua disponível, mas a medição abaixo não justifica uma biblioteca de virtualização.

Medição em 2026-10-10, Node v22.23.3, linux, sem navegador gráfico (`frontend/scripts/measure-table-heap.mjs`):

- 100 linhas (página máxima usual): 35.176 bytes de heap.
- 10.000 linhas montadas de uma vez, como objetos `{id, database, schema, name, size, text}`: 2.782.296 bytes de heap.

O limite adotado para introduzir virtualização é 64 MB. O resultado ficou abaixo, então não há dependência nova. Teclado, leitor de tela e a paginação no servidor permanecem como estão.
