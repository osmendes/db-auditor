# Virtualização de tabelas

As listas de inventário e achados paginam no servidor (20, 50, 100 ou todas as linhas filtradas). Não há virtualização no cliente.

Uma biblioteca como `react-window` só entra se um perfil de heap do navegador, com cerca de 10 mil linhas já carregadas no DOM, mostrar pressão de memória. Até lá, a paginação no snapshot store é o limite. Teclado, leitor de tela e a página atual permanecem como estão.
