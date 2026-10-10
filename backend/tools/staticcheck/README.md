# Staticcheck usado pela CI

O projeto usa Go 1.27. Staticcheck v0.8.1 depende de uma versão de `golang.org/x/tools` que não lê os dados de exportação do Go 1.27. Este módulo separado fixa Staticcheck v0.8.1 e `x/tools` v0.51.0, combinação validada com `staticcheck ./...` no projeto. A aplicação mantém seu `go.mod` próprio.

A CI compila o executável neste diretório e o executa a partir de `backend/`. Atualize as versões juntas quando Staticcheck publicar suporte nativo a Go 1.27; não remova a análise estática para contornar o erro da ferramenta.
