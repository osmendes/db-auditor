# Guia do usuário — DB Auditor

O DB Auditor ajuda a conhecer um banco, encontrar sinais de risco e acompanhar melhorias. Ele **não corrige, remove ou altera dados no banco analisado**. As recomendações são hipóteses para a equipe responsável confirmar antes de executar qualquer mudança fora da aplicação.

Os coletores atendem PostgreSQL, TimescaleDB e o catálogo de coleções e índices do MongoDB. No MongoDB, o auditor não lê documentos nem aplica regras específicas de PostgreSQL; as demais capacidades aparecem como não aplicáveis. A ausência de um alerta só é tranquilizadora quando a coleta e a análise terminaram com cobertura suficiente para aquela verificação.

## 1. Entrar e escolher um ambiente

Abra o endereço fornecido pelo administrador e entre com sua conta. A sessão dura até oito horas. Ao recarregar a página, o aplicativo verifica se ela ainda é válida. Use **Sair** em um computador compartilhado.

Escolha um ambiente no seletor. Cada ambiente representa um banco ou conjunto de bancos configurado pelo administrador. Se ele não aparecer, peça ao operador que confira o acesso da sua conta. Um **viewer** consulta dados; um **auditor** registra decisões e solicita diagnósticos; um **operator** também administra contas e configurações autorizadas.

Os endereços e credenciais dos bancos analisados ficam na configuração do servidor. Eles não são digitados na interface nem aparecem nos relatórios.

A navegação fica no fragmento da URL (`#/dashboard`, `#/findings/<id>?env=<ambiente>`), não num caminho do servidor. Recarregar a página, voltar no navegador ou enviar o link abre a mesma seção, o mesmo ambiente e o mesmo achado. Uma sessão expirada pede login de novo e, depois dele, o fragmento continua na barra de endereço.

## 2. Fazer uma coleta e entender a cobertura

Em **Execuções**, selecione o ambiente e inicie uma auditoria quando seu papel permitir. Escolha o perfil de coleta adequado: perfis mais amplos observam mais aspectos e podem levar mais tempo. A tela mostra a execução, os coletores, avisos e falhas.

- **Sucesso:** a execução terminou; confira ainda a cobertura dos coletores usados na conclusão.
- **Sucesso parcial:** parte dos dados não pôde ser coletada. Uma lista vazia pode significar falta de dados, não ausência de problemas.
- **Falha:** verifique a mensagem e peça ao operador que revise conectividade e permissões da conta de leitura.
- **Sem coleta:** inicie uma execução antes de interpretar inventário, score ou tendências.

O identificador da execução permite relacionar inventário, achados, gráficos e relatórios à mesma observação.

## 3. Navegar pelo inventário

Em **Inventário**, escolha banco, esquema e tipo de objeto. Para MongoDB, abra o banco e veja as coleções e índices; a tela mostra contagem estimada de documentos, armazenamento e índices. A busca e os filtros reduzem a lista. Use os controles de página para percorrer objetos; o total indica quantos atendem ao filtro. Clique no cabeçalho de uma coluna para ordenar **todo o resultado filtrado**, inclusive as outras páginas. Abrir uma linha mostra detalhes e histórico do objeto; fechar o painel devolve a URL à lista.

O painel de cada tipo de objeto oferece as mesmas seções: visão geral, estrutura, relacionamentos, performance, segurança e recomendações. Cada seção mostra dados do objeto selecionado ou explica por que a informação está **não coletada**, **não aplicável**, **vazia** ou **sem permissão**; ausência de dados não comprova que o objeto esteja seguro ou livre de problemas. O painel avisa quando a coleta foi parcial ou vazia. Use o link direto no início do painel para compartilhar a observação exata com outra pessoa que tenha acesso ao ambiente. Ao fechar o painel, o endereço volta à lista; links antigos de tabelas continuam abrindo.

Tabelas, índices, visualizações, funções, hypertables e agregados contínuos dependem das capacidades da coleta. Se um tipo não se aplica ao mecanismo ou não foi coletado, a aplicação indica a limitação. Um número de linhas é estimado pelo PostgreSQL e pode diferir da contagem real.

Em **Visões**, abra uma linha para consultar colunas, dependências registradas, acessos efetivos observados e achados daquela execução. A definição SQL fica protegida porque pode conter valores sensíveis; a impressão digital permite perceber uma mudança sem revelar o texto. Uma visão comum não armazena dados próprios, então seu tamanho não representa o custo da consulta. Uma visão materializada ocupa espaço, mas a coleta atual não informa o instante do último `REFRESH` nem o custo das consultas. As opções `security_invoker` e `security_barrier` aparecem quando o PostgreSQL permite observá-las; “não coletado” não significa que estejam desativadas. Dependências e acessos ausentes na lista também não comprovam ausência de uso ou acesso.

Em **Índices**, abra uma linha para ver as colunas da chave, colunas incluídas, validade, uso, tamanho, histórico e recomendações daquela execução. O link para a tabela relacionada abre a análise da tabela no mesmo ambiente e execução. A definição SQL e o predicado ficam protegidos; a impressão digital ajuda a reconhecer mudanças sem mostrar valores que possam ser sensíveis. Se o contador de varreduras for zero, confira por quanto tempo as estatísticas foram observadas, se foram reiniciadas e se a carga habitual ocorreu nesse período. Zero isolado não justifica remover um índice. Um índice inválido pede investigação e planejamento com o administrador; o auditor não executa alterações. As permissões exibidas são as da tabela, pois esta coleta não registra permissões próprias para índices. Snapshots antigos podem mostrar “não coletado” para colunas incluídas e disponibilidade das estatísticas; faça uma nova coleta para preencher esses campos.

Em **Funções**, o nome e a assinatura identificam cada sobrecarga. Abra a linha correta para ver retorno, linguagem, volatilidade, paralelismo, dependências registradas, estatísticas agregadas, papéis com `EXECUTE` observado e achados daquela assinatura. O corpo e as configurações brutas não são exibidos. `SECURITY DEFINER` indica execução com privilégios do proprietário e pede revisão humana do corpo, dos acessos e do caminho de busca; a opção sozinha não comprova uma falha. Estatísticas de chamadas só aparecem se o PostgreSQL as tiver fornecido. Uma coleta antiga pode mostrar “não coletado” para os novos campos; faça outra coleta. Dependências por SQL dinâmico não são inferidas.

Em **Tabelas temporais**, abra uma linha para examinar dimensões, chunks, índices, políticas, jobs, armazenamento, histórico, acessos observados na tabela base e achados daquela execução. Um job com falhas merece investigação, mas o número acumulado de falhas não prova que a última execução falhou. Os tamanhos não medem tempo de consulta. Compare coletas com cobertura semelhante e use a indicação de coleta parcial antes de concluir que houve crescimento ou melhora. Quando a tabela base não aparece na coleta, o auditor não deduz seus acessos. Compressão desabilitada significa apenas que ela não estava habilitada no momento observado. Use os links da sheet para abrir a tabela base e seus índices no mesmo ambiente e execução; o auditor não executa mudanças no banco analisado.

Em **Agregados contínuos**, abra uma linha para analisar origem, materialização, políticas de atualização, atraso observado, dependências, acessos da visão e achados do agregado na execução selecionada. O SQL da definição é protegido; sua impressão digital permite reconhecer alterações sem expor valores. A origem e o intervalo do bucket aparecem quando o catálogo do Timescale os fornece; a coluna temporal usada no SQL não é inferida. Confirme esses detalhes no banco antes de mudar uma política. Quando o tamanho da materialização ou o atraso não estiver disponível, a sheet mostra “não coletado”. Uma política ausente não significa que o agregado não seja atualizado manualmente. O número de falhas de job é acumulado; consulte o último resultado antes de concluir que o refresh atual falhou. Os acessos exibidos pertencem à visão do agregado, não à origem. Uma execução parcial limita comparações de tamanho e atraso.

## 4. Investigar achados

Em **Achados**, filtre por severidade, estado e tipo. A **constatação** resume o que foi observado em português. Abra o achado para ver evidência, confiança, cobertura, objeto, orientação e histórico. Os termos e textos originais da regra ficam na seção técnica.

Um achado é um sinal para investigar. Severidade indica prioridade inicial, não ganho garantido. Confiança baixa ou coleta parcial pede confirmação adicional. O auditor pode reconhecer um achado, planejar uma ação, justificar uma supressão e registrar o resultado. Se o sinal reaparecer, o histórico anterior permanece visível.

Antes de usar uma consulta de confirmação, substitua os parâmetros pelo objeto correto, revise o SQL e execute-o somente em um ambiente autorizado. O auditor não executa a correção sugerida.

## 5. Ler o dashboard e acompanhar mudanças

O **Dashboard** mostra indicadores atuais e a evolução por execução: armazenamento, achados, score e cobertura. Escolha período, granularidade e ambiente. Cada barra representa uma execução identificável; intervalos sem coleta não são tratados como zero. Avisos indicam coletas parciais, versões ou perfis incompatíveis e reinício observado de contadores.

O **score** é uma nota ponderada, com versão e pesos mostrados no produto. Achados reduzem a nota e chaves primárias verificadas acrescentam pontos à categoria de estrutura. Ele fica indisponível quando faltam coletores necessários ou a análise não foi concluída. Compare notas somente entre execuções compatíveis. A comparação mostra quais categorias mudaram; isso é uma pista para investigar, não prova de causa.

Em **Acompanhamento**, um auditor pode aprovar um baseline, registrar manutenção ou implantação e reconhecer alertas de regressão. Um alerta persistente mostra sua referência e a evidência. Não interprete um intervalo sem coleta como melhora.

## 6. Medir uma ação antes e depois

Em **Ações assistidas**, escolha uma ação e duas execuções: a primeira antes da alteração externa e a segunda depois. Informe a hipótese, a janela observada e o que sabe sobre a carga. Marque a confirmação de carga semelhante somente se tiver evidência para isso. A aplicação verifica ainda perfil, versões, análise, objeto e cobertura.

Algumas ações mostram um indicador numérico preliminar: tamanho de um índice sem uso observado ou proporção estimada de espaço associado a tuplas mortas. Esse número não é uma promessa de espaço recuperável; a própria ação explica as limitações. O resultado mostra valores anteriores e posteriores, comparabilidade e ressalvas. Uma diferença observada **não comprova que a ação a causou**. Para marcar uma ação como validada, registre uma medição comparável em uma coleta completa posterior à mudança. Se um achado persistir após uma medição comparável, a ação volta para análise. A lista e as medições têm páginas. Use **Baixar histórico JSONL** ou **Baixar histórico CSV** para receber todas as ações e medições do ambiente: o servidor envia o arquivo em partes, sem carregar o histórico inteiro na página. JSONL contém uma linha JSON por ação ou medição; o campo `record_type` indica o tipo. O arquivo termina com um registro `complete` (JSONL) ou `# export_complete` (CSV), com as contagens exportadas. Se esse registro faltar, a transferência foi interrompida e deve ser repetida. O arquivo pode conter nomes de objetos e notas, então guarde-o em local autorizado.

## 7. Diagnóstico opcional de qualidade de dados

Um operador precisa habilitar o recurso e configurar uma conta de leitura sem privilégios de escrita. Em **Ações assistidas**, informe banco, esquema, tabela, limite de até 1.000 linhas e as colunas que deseja examinar. Você pode verificar nulos inesperados, chaves candidatas repetidas, datas fora de um intervalo, distribuição concentrada e referências órfãs em relações simples.

O resultado guarda **contagens**, não valores de linhas. Em tabelas grandes, o PostgreSQL seleciona páginas para a amostra; em tabelas menores, a leitura limitada pode seguir a ordem física. A margem de erro amostral é **desconhecida** e aparece no diagnóstico; as contagens descrevem somente as linhas lidas. Confirme qualquer hipótese na população de dados antes de uma mudança.

O plano sugerido descreve confirmação, dependências, backup, janela, validação e recuperação. Para dados fora de uma política de retenção, obtenha aprovação da área de negócio, teste a cópia para arquivo e a restauração antes de considerar exclusão externa. Para marcar a ação de qualidade como **validada**, repita a mesma verificação após a mudança, com método, limite e número de linhas comparáveis. O auditor vincula o ID do novo diagnóstico à decisão. Um texto livre sozinho não valida a ação; compare as contagens e explique o resultado. Mesmo duas amostras comparáveis podem ter lido páginas diferentes.

## 8. Gerar e compartilhar um PDF

Em **Relatórios**, escolha ambiente, execução concluída, tipo de relatório e filtros. A geração ocorre em segundo plano; acompanhe o estado e baixe o arquivo quando estiver pronto. O relatório executivo resume riscos e próximos passos. O técnico acrescenta evidências e inventário. O relatório por tabela foca um objeto.

O PDF informa escopo, execução, cobertura, versão das regras, score, gráficos e análise final. Quando existe baseline, destaca novos achados altos ou críticos. Se o conteúdo ultrapassar os limites, o próprio documento indica o recorte ou a geração falha com uma explicação. Os limites atuais são 500 bancos, 5.000 tabelas, 2.000 achados, 200 páginas e 16 MiB.

Antes de compartilhar, escolha a redação apropriada:

- **Sem redação:** inclui identificadores e evidências disponíveis; compartilhe apenas com quem pode acessá-los.
- **Identificadores:** oculta nomes, evidências e textos originais sensíveis, preservando gráficos agregados.
- **Estrita:** também retira listas de objetos, evolução de armazenamento e detalhes do baseline; preserva contagens e a proveniência do pedido.

O artefato expira após 30 dias. A expiração remove o PDF, mas preserva o registro do pedido. Um novo pedido pode gerar outro arquivo. O hash exibido no histórico permite conferir a integridade do artefato enquanto ele existir.

## 9. Outras áreas

- **Performance:** consulte sinais de consultas, índices, manutenção, espaço e carga. Confirme planos e estatísticas em uma janela representativa antes de agir.
- **Segurança:** revise papéis, permissões efetivas por tabela, funções privilegiadas e exposição observável. O auditor sinaliza contas com validade expirada ou sem sessão ativa vista em coletas espaçadas por pelo menos 30 dias. A ausência de sessão na amostra não comprova que a conta não foi usada entre coletas. Confirme com a equipe responsável antes de desativar ou revogar acesso, com plano de retorno.
- **Mapeamentos, Desvio de schema e Comparar:** relacione objetos entre ambientes e examine diferenças. Versão, perfil e cobertura podem limitar a comparação.
- **Regras:** veja o catálogo, sua versão e a explicação das verificações. Uma regra não aplicável ao mecanismo não deve ser interpretada como resultado saudável.
- **Status:** veja se API, armazenamento interno, conexões e tarefas estão funcionando.
- **Contas:** operadores gerenciam usuários e acessos. Não compartilhe senha; peça revogação se perder um dispositivo. TOTP é opcional: em Contas, gere o segredo, confirme o código do autenticador e guarde os códigos de recuperação. Só um operador pode exigir o código na própria conta. Não há SMS.
- **Documentação:** consulte ajuda contextual e o glossário dentro do aplicativo.

## 10. Se algo parecer errado

1. Confira o ambiente, a execução e os filtros selecionados.
2. Verifique se a coleta e a análise terminaram e se a cobertura é completa.
3. Leia o aviso exibido; ele pode indicar falta de permissão, limite, versão incompatível ou erro temporário.
4. Repita a ação apenas se ela for segura. Para falhas persistentes, envie ao operador o horário, ambiente, identificador da execução e `X-Request-ID` da resposta. Não envie credenciais nem dados de linha.

Para instalar, atualizar e fazer backup, use o [guia de operação](ops.md). Para conhecer limites e decisões técnicas da terceira etapa, use [operações da terceira etapa](third-stage-operations.md).
