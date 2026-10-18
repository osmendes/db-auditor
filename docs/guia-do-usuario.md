# Guia do usuário — DB Auditor

O DB Auditor ajuda a conhecer um banco, encontrar sinais de risco e acompanhar melhorias. Ele **não corrige, remove ou altera dados no banco analisado**. As recomendações são hipóteses para a equipe responsável confirmar antes de executar qualquer mudança fora da aplicação.

Os coletores atendem PostgreSQL, TimescaleDB e o catálogo de coleções e índices do MongoDB. No MongoDB, o auditor não lê documentos nem aplica regras específicas de PostgreSQL; as demais capacidades aparecem como não aplicáveis. A ausência de um alerta só é tranquilizadora quando a coleta e a análise terminaram com cobertura suficiente para aquela verificação.

## 1. Entrar e escolher um ambiente

Abra o endereço fornecido pelo administrador e entre com sua conta. A sessão dura até oito horas. Ao recarregar a página, o aplicativo verifica se ela ainda é válida. Use **Sair** em um computador compartilhado.

Escolha um ambiente no seletor. Cada ambiente representa um banco ou conjunto de bancos configurado pelo administrador. Se ele não aparecer, peça ao operador que confira o acesso da sua conta. Um **viewer** consulta dados; um **auditor** registra decisões e solicita diagnósticos; um **operator** também administra contas e configurações autorizadas.

Os endereços e credenciais dos bancos analisados ficam na configuração do servidor. Eles não são digitados na interface nem aparecem nos relatórios.

A navegação fica no fragmento da URL (`#/dashboard`, `#/findings/<id>?env=<ambiente>`), não num caminho do servidor. Recarregar a página, voltar ou avançar no navegador ou enviar o link abre a mesma seção, o mesmo ambiente, o mesmo achado e os mesmos filtros da lista. Uma sessão expirada pede login de novo e, depois dele, o fragmento continua na barra de endereço.

## 2. Fazer uma coleta e entender a cobertura

Em **Execuções**, selecione o ambiente e inicie uma auditoria quando seu papel permitir. Escolha o perfil de coleta adequado: perfis mais amplos observam mais aspectos e podem levar mais tempo. A tela mostra a execução, os coletores, avisos e falhas.

- **Sucesso:** a execução terminou; confira ainda a cobertura dos coletores usados na conclusão.
- **Sucesso parcial:** parte dos dados não pôde ser coletada. Uma lista vazia pode significar falta de dados, não ausência de problemas.
- **Falha:** verifique a mensagem e peça ao operador que revise conectividade e permissões da conta de leitura.
- **Sem coleta:** inicie uma execução antes de interpretar inventário, score ou tendências.

O identificador da execução permite relacionar inventário, achados, gráficos e relatórios à mesma observação.

**Dashboard**, **Achados** e **Relatórios** mostram o mesmo aviso de cobertura para a mesma execução. Ele aparece quando a coleta foi parcial, quando a conta de leitura não teve permissão para parte do catálogo, quando uma tentativa mais recente falhou (lacuna: os dados vêm da coleta anterior) ou quando parte das verificações não se aplica ao mecanismo. Com coleta parcial, a nota aparece marcada como **parcial** e com a confiança ao lado; não a compare com a nota de uma coleta completa.

Uma lista vazia sempre diz o motivo, e cada motivo tem um próximo passo:

- **Nenhuma coleta concluída:** ainda não houve execução que terminou. Inicie uma execução.
- **Nenhum item na última coleta:** a coleta terminou e não encontrou nada. Confira a cobertura antes de concluir que não há risco.
- **Nenhum item neste filtro:** há dados, mas os filtros escondem todos. Limpe os filtros.
- **Ambiente sem conexão configurada:** falta o DSN. Peça ao operador para concluir a conexão em Ambientes.

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

Para instalar, atualizar e fazer backup, use o [guia de operação](ops.md). Limites e decisões técnicas de cada etapa ficam nas [notas por etapa](notas-por-etapa.md), referenciadas pelo guia de operação.

## 11. Ajuda por tela

A página **Documentação** do aplicativo lê os tópicos abaixo deste arquivo, sem cópia no código da interface. Cada tópico tem um parágrafo de abertura, os passos em **Como usar** e, quando houver, **Vale saber** e **Termos desta tela**. O comentário logo abaixo de cada título identifica o tópico e a tela que o botão "Abrir" leva; mantenha esse formato ao editar.

<!-- ajuda-por-tela:inicio -->

### Por onde começar

<!-- topic: comecar -->

O DB Auditor mostra um retrato do banco: o que existe, o que merece atenção e o que mudou desde a última visita. Ele só observa. Não corrige, não apaga dados e não muda permissões no banco analisado. Um aviso é um convite para a equipe confirmar, não uma ordem para agir.

**Como usar**

1. Entre com o usuário e a senha que a pessoa administradora passou para você.
2. No seletor de ambiente, escolha o banco que quer ver. Se ele não aparecer, peça acesso. Quem consulta um ambiente não vê os outros.
3. Se ainda não houver coleta, abra Execuções e inicie uma auditoria, se o seu papel permitir.
4. Antes de concluir que está tudo bem, leia a cobertura. Coleta parcial ou com falha significa que parte da informação não chegou.
5. Em um computador compartilhado, use Sair. A sessão dura até oito horas. Recarregar a página não pede a senha de novo enquanto ela valer.

**Vale saber**

- Quem só consulta é viewer. Quem registra decisões e pede relatórios é auditor. Quem administra contas é operator.
- O endereço e a senha do banco analisado ficam no servidor. Eles não aparecem na tela nem no relatório.
- O link da página guarda a seção e o ambiente. Enviar esse endereço abre o mesmo lugar para quem tiver acesso.

**Termos desta tela**

- **Ambiente** — Um banco, ou um grupo de bancos, que o auditor acompanha. Cada ambiente tem um nome na tela.
- **Coleta** — Uma visita somente leitura. O auditor anota a estrutura e as estatísticas que o banco deixou ver naquele momento.
- **Cobertura** — O quanto da visita deu certo. Sem cobertura suficiente, uma lista vazia não significa ausência de problema.

### Dashboard

<!-- topic: dashboard | abrir: Dashboard -->

O painel é o resumo do ambiente escolhido: espaço, achados, nota e as cinco prioridades do momento. Use-o para decidir por onde começar, não para fechar um diagnóstico.

**Como usar**

1. Escolha o ambiente na barra lateral. A opção de ver todos soma os ambientes que você pode acessar.
2. Leia o aviso do topo. Ele diz se a conexão está boa, se a coleta foi parcial ou se falta permissão.
3. Os cartões levam à tela correspondente: achados, armazenamento ou execuções.
4. No gráfico, cada barra é uma execução identificável. Um intervalo sem coleta não é tratado como zero.
5. A nota só aparece quando a coleta e a análise necessárias terminaram. Compare notas apenas entre visitas compatíveis.

**Vale saber**

- A nota é uma média ponderada. Achados reduzem a nota. Chaves primárias verificadas ajudam a parte de estrutura. Os pesos aparecem no próprio painel.
- Uma mudança de categoria é uma pista, não a prova da causa.
- As cinco prioridades são os sinais mais urgentes da última análise concluída. Abra cada um para ler a evidência.

**Termos desta tela**

- **Nota** — Um número de apoio, com versão e pesos visíveis. Ela fica indisponível quando falta coletor ou a análise não terminou.
- **Prioridade** — Um achado destacado para a leitura executiva. Severidade alta não garante ganho se a equipe aplicar a sugestão.

### Ambientes

<!-- topic: ambientes | abrir: Ambientes -->

Aqui ficam os bancos que o auditor conhece. A tela mostra nome, tipo e se a conexão está disponível. Credenciais não são editadas neste lugar.

**Como usar**

1. Confira se o ambiente que você espera está na lista.
2. Veja o tipo: PostgreSQL, TimescaleDB ou MongoDB. O tipo muda o que a coleta consegue observar.
3. Se o status estiver indisponível, a visita não vai conseguir ler esse banco. Peça a quem opera o servidor para revisar o acesso de leitura.
4. Depois de uma mudança de conexão no servidor, a API precisa ser reiniciada. Em seguida, confira Status e o painel.

**Vale saber**

- Criar ou apontar um ambiente novo é trabalho de quem administra o servidor, não um formulário desta tela.
- Dois ambientes de demonstração podem existir numa instalação nova. Eles só ganham dados quando a conexão é preenchida e uma coleta termina.

**Termos desta tela**

- **Somente leitura** — A conta usada na coleta pode consultar metadados. Ela não recebe ordem de criar, apagar ou alterar objetos.
- **Descoberta** — A forma de achar um banco ou vários bancos no servidor já configurado.

### Execuções

<!-- topic: execucoes | abrir: Execuções -->

Uma execução é uma visita completa: o auditor percorre os coletores, grava o que viu e marca o resultado. Inventário, achados e relatórios apontam para uma execução.

**Como usar**

1. Escolha o ambiente e inicie a auditoria, se o seu papel permitir. O pedido pede confirmação.
2. Escolha o perfil de coleta. Um perfil mais amplo olha mais coisas e pode demorar mais.
3. Acompanhe o progresso de cada coletor. Enquanto houver visita em andamento, a lista se atualiza sozinha.
4. Abra a execução para ver erros, avisos e o que foi de fato coletado.
5. Uma visita em andamento pode ser cancelada por um operator. O cancelamento é cooperativo: o que já foi gravado permanece.

**Vale saber**

- Sucesso: a visita terminou. Ainda assim, confira a cobertura dos coletores.
- Sucesso parcial: uma parte não pôde ser lida. Lista vazia, nesse caso, pode ser falta de dados.
- Falha: leia a mensagem. Em geral é conexão ou permissão da conta de leitura.
- Sem coleta ainda, não interprete inventário, nota ou tendência.

**Termos desta tela**

- **Coletor** — Uma parte da visita. Um lê tabelas, outro lê índices, outro lê permissões, e assim por diante.
- **Perfil** — O conjunto de coletores desta visita. Comparar visitas de perfis diferentes pede cuidado.
- **Baseline** — Uma execução aprovada como referência. Aprovar outra exige confirmação e cobertura suficiente.

### Inventário

<!-- topic: inventario | abrir: Inventário -->

O inventário é a lista do que existia na execução escolhida: tabelas, índices, visões, funções e, no TimescaleDB, tabelas temporais e agregados. No MongoDB, a lista mostra coleções e índices. O auditor não lê os documentos.

**Como usar**

1. Escolha o ambiente. Sem ambiente, a lista não tem de onde vir.
2. Filtre por banco, esquema e tipo de objeto. A busca reduz a lista pelo nome.
3. Use as páginas para percorrer o resultado. Ordenar uma coluna vale para o resultado inteiro, não só para a página que está na tela.
4. Abra uma linha para ver o detalhe daquele objeto. Feche pelo botão, pelo fundo ou com Esc. O endereço volta para a lista.
5. Copie o link do painel quando quiser mostrar exatamente aquele objeto para outra pessoa com acesso.

**Vale saber**

- Cada painel separa visão geral, estrutura, relacionamentos, performance, segurança e recomendações. Se um dado não veio, o texto diz se foi não coletado, não aplicável, vazio ou sem permissão.
- O número de linhas é uma estimativa do PostgreSQL. Ele pode diferir da contagem real.
- A definição SQL de visões, índices e funções fica protegida, porque pode conter valores sensíveis. Uma impressão digital ajuda a perceber que algo mudou sem mostrar o texto.
- Índice com zero varreduras não deve ser removido só por isso. Confira há quanto tempo as estatísticas foram observadas e se a carga habitual passou por ali.
- No MongoDB, verificações feitas para PostgreSQL aparecem como não aplicáveis.

**Termos desta tela**

- **Tabela temporal** — Uma tabela do Timescale organizada no tempo, ou em outra dimensão, e dividida em pedaços.
- **Agregado contínuo** — Um resumo que o Timescale mantém atualizado a partir de uma origem. Sem política visível, ele ainda pode ser atualizado à mão.
- **Não coletado** — O campo não veio nesta visita. Isso não prova que o recurso está desligado nem que o objeto está seguro.

### Mapeamentos

<!-- topic: mapeamentos | abrir: Mapeamentos -->

Mapeamento é dizer que um objeto de um ambiente corresponde a um objeto de outro. Sem esse par, a comparação de estrutura fica sem referência.

**Como usar**

1. Tenha pelo menos dois ambientes com inventário já coletado.
2. Peça a sugestão automática. Ela propõe pares parecidos. Nada é validado sozinho.
3. Revise cada sugestão. Confirme o par certo e recuse o que não for o mesmo objeto.
4. Se a sugestão não achar um par, associe à mão quando você souber que os dois lados são o mesmo objeto.

**Vale saber**

- Nome igual não garante que seja o mesmo objeto. Olhe o tipo e a estrutura antes de validar.
- Um par recusado continua no histórico. Ele não é apagado em silêncio.

**Termos desta tela**

- **Impressão digital** — Uma assinatura do objeto, feita com nome, tipo e estrutura, usada para sugerir pares.
- **Par validado** — Uma correspondência que uma pessoa confirmou. A comparação de desvio usa esses pares.

### Desvio de schema

<!-- topic: desvio | abrir: Desvio de schema -->

Desvio de schema é a diferença de estrutura entre dois ambientes: o que está igual, o que mudou e o que existe só de um lado. A tela mostra a diferença. Ela não aplica correção.

**Como usar**

1. Escolha a origem e o destino. Os dois precisam ter inventário comparável.
2. Leia as categorias: igual, diferente, só na origem e só no destino.
3. Abra um item para ver o que divergiu, como coluna, tipo ou índice.
4. Use o resultado como evidência para a equipe do banco. A mudança, se houver, acontece fora do auditor.

**Vale saber**

- Versão, perfil de coleta e cobertura mudam o que dá para comparar. Uma ausência pode ser objeto que não entrou na visita.
- Ambientes de mecanismos diferentes, como PostgreSQL e MongoDB, não têm a mesma estrutura para comparar.

**Termos desta tela**

- **Igual** — Os dois lados parecem o mesmo objeto, sem diferença relevante de estrutura nesta comparação.
- **Diferente** — O par existe nos dois lados, mas alguma parte da estrutura não coincide.
- **Só de um lado** — O objeto apareceu na origem ou no destino, e não no outro.

### Comparar servidores

<!-- topic: comparar | abrir: Comparar -->

Esta tela coloca lado a lado o último retrato de dois ambientes. Serve para ver diferenças de configuração e de tamanho sem abrir uma sessão nova no banco.

**Como usar**

1. Escolha o ambiente da esquerda e o da direita. Eles precisam ser diferentes.
2. Leia o relatório gerado a partir dos snapshots já gravados.
3. Trate diferença de parâmetro, como memória compartilhada, como informação. Ela não é, sozinha, um defeito.
4. Se o retrato de um lado for antigo ou parcial, repita a coleta antes de concluir que os servidores divergiram.

**Vale saber**

- A comparação não liga de novo no banco auditado. Ela usa o que a última visita guardou.
- Para diferença objeto a objeto, use Desvio de schema. Esta tela é o panorama dos dois servidores.

**Termos desta tela**

- **Snapshot** — A cópia do que foi observado numa execução. Comparar servidores usa o snapshot mais recente de cada lado.

### Achados

<!-- topic: achados | abrir: Findings -->

Um achado é um sinal de uma regra: algo que vale investigar. A tela explica o que foi visto, em que objeto, com que confiança e o que a equipe pode fazer em seguida. O auditor não executa a correção.

**Como usar**

1. Filtre por gravidade, estado e tema. Desempenho e Segurança são filtros desta mesma lista, não páginas separadas.
2. Abra o achado. Leia a constatação, a evidência, a cobertura e a orientação.
3. Se o seu papel permitir, reconheça o sinal, planeje uma ação ou justifique uma supressão.
4. Para comparar duas visitas, escolha as duas execuções no painel de diferenças. A lista mostra o que surgiu, sumiu ou mudou.
5. Exporte CSV ou JSON quando precisar levar a lista para fora. Não inclua senha nem dado de linha nesse arquivo.

**Vale saber**

- Gravidade é uma prioridade inicial, não um ganho garantido.
- Confiança baixa ou coleta parcial pede uma confirmação a mais.
- A consulta de confirmação é um modelo. Troque os parâmetros, revise o texto e rode só num ambiente autorizado.
- Suprimir não apaga o histórico. O sinal pode ficar quieto por um período. Se voltar depois disso, ele reaparece.
- Uma visita incompleta não serve para dizer que um achado foi resolvido.

**Termos desta tela**

- **Regra** — Uma verificação automática. Ela produz o achado. Uma pessoa decide o que fazer com ele.
- **Gravidade** — Crítica, alta, média, baixa ou informativa. Ajuda a ordenar a fila, não a provar impacto.
- **Supressão** — Um registro de que o sinal não deve chamar atenção agora, com motivo e prazo.

### Performance

<!-- topic: performance | abrir: Findings -->

Performance reúne os sinais de espaço, manutenção, índices e consultas. Na página Análises, use o filtro Desempenho. Abra o item: o painel explica a melhoria e o passo a passo.

**Como usar**

1. Escolha o ambiente e a execução que quer ler.
2. Abra cada sinal e confira se a evidência cobre uma janela representativa da carga.
3. Cruze com o Inventário: tamanho de tabela, tamanho de índice e histórico do objeto.
4. Planeje a manutenção com a equipe do banco. Nada desta tela roda sozinho no banco analisado.

**Vale saber**

- Um índice sem uso na amostra pode ser necessário num relatório mensal que não passou pela coleta.
- Estatísticas reiniciadas fazem contadores voltarem a zero. O aviso de reinício importa antes de comparar.
- Tendência de crescimento só é estimada quando as duas coletas comparadas terminaram por completo.

**Termos desta tela**

- **Manutenção** — Rotinas como limpeza e atualização de estatísticas. O auditor aponta atraso ou custo. A execução fica com o administrador do banco.
- **Consulta agrupada** — Consultas parecidas, reunidas sem os valores literais, para ver o padrão e não cada chamada isolada.

### Segurança

<!-- topic: seguranca | abrir: Findings -->

Segurança reúne os sinais de permissão, papéis e funções privilegiadas. Na página Análises, use o filtro Segurança. A ausência de um alerta só tranquiliza quando a coleta daquela verificação terminou.

**Como usar**

1. Comece pela gravidade crítica e alta.
2. Abra o achado e veja o objeto, a permissão e o que foi de fato observado.
3. Contas sem sessão na amostra, ou com validade vencida, pedem confirmação com a equipe. A amostra não prova que a conta ficou sem uso entre as visitas.
4. Não revogue acesso a partir da tela. Combine a mudança e um plano de retorno fora do auditor.

**Vale saber**

- Função marcada como SECURITY DEFINER executa com os privilégios de quem é dono dela. Isso pede revisão humana. A marca sozinha não é uma falha comprovada.
- Permissões de índice, nesta coleta, são as da tabela dona.
- O corpo das funções não é exibido.

**Termos desta tela**

- **Permissão efetiva** — O que a coleta conseguiu ver sobre quem pode usar aquele objeto, não a lista completa de tudo que a pessoa pode fazer no servidor.
- **Evidência** — O fato observado: objeto, permissão, configuração ou contagem. Sem evidência, o texto diz o que faltou.

### Regras

<!-- topic: regras | abrir: Findings -->

Regras são as verificações que geram as análises. Em Análises, o botão Regras do ambiente abre o catálogo: a versão e se a regra está ligada no esquema escolhido. Uma regra que não se aplica ao tipo de banco não é um resultado saudável. Ela simplesmente não roda ali.

**Como usar**

1. Escolha o ambiente e o esquema.
2. Leia o nome da regra e a explicação em português: o que ela procura e o que não afirma.
3. Veja se a regra está ativa para aquele esquema. Quem opera pode ajustar a exceção permitida. A mudança fica registrada.
4. Para ver os sinais já produzidos, volte para Achados e filtre pela regra.

**Vale saber**

- O texto da regra descreve constatação, evidência, impacto e próximo passo. O impacto só fala em número quando há métrica e denominador.
- Atualizar o catálogo muda a versão usada na análise. Relatórios guardam a versão com a qual foram gerados.

**Termos desta tela**

- **Catálogo** — A lista versionada de verificações. A mesma versão deve ser usada para comparar duas análises.
- **Não aplicável** — A regra não faz sentido naquele mecanismo, por exemplo uma verificação de PostgreSQL num catálogo MongoDB.

### Relatórios

<!-- topic: relatorios | abrir: Relatórios -->

O relatório em PDF é uma leitura para compartilhar: resumo executivo, detalhe técnico ou uma tabela. Ele é pedido em segundo plano e só nasce de uma execução concluída, com a análise das regras também concluída.

**Como usar**

1. Escolha o ambiente, a execução concluída e o tipo: executivo, técnico ou tabela.
2. No tipo tabela, informe banco, esquema e tabela.
3. Acompanhe a fila. Você pode cancelar um pedido ainda em andamento ou tentar de novo quando a tela permitir.
4. Baixe o PDF enquanto ele existir. Confira a cobertura no documento antes de usar os números.
5. Se o arquivo estiver grande, ele pode vir truncado. O próprio PDF diz o que foi cortado.

**Vale saber**

- O PDF fica guardado por 30 dias. Depois disso o arquivo some, mas o registro de que ele foi pedido permanece.
- Há um código do arquivo para conferir se o download é o mesmo que foi gerado.
- O servidor pode ocultar alguns identificadores no cabeçalho, se essa opção estiver ligada.
- Uma execução parcial não é usada para afirmar que um achado foi resolvido.

**Termos desta tela**

- **Fila** — A lista de pedidos. O PDF não sai no instante do clique. A tela avisa quando ele fica pronto.
- **Redação** — Uma opção de esconder identificadores no PDF, para compartilhar o conteúdo com menos contexto interno.

### Acompanhamento

<!-- topic: acompanhamento | abrir: Acompanhamento -->

Acompanhamento é o caderno da equipe: baseline aprovado, anotações de implantação ou manutenção, e alertas de regressão. Um alerta só nasce de duas coletas completas, para não gritar por causa de uma visita pela metade.

**Como usar**

1. Escolha o ambiente.
2. Veja qual execução está aprovada como referência.
3. Registre uma implantação ou uma manutenção, com uma nota curta do que aconteceu fora do auditor.
4. Leia os alertas de regressão. Cada um mostra a referência e a evidência.
5. Reconheça um alerta quando a equipe já souber do que se trata. O reconhecimento pede um motivo.

**Vale saber**

- Aprovar baseline é uma decisão. Ela pede cobertura suficiente.
- Avisos de execução, como falha de coleta, continuam aparecendo mesmo quando a visita foi parcial. O que fica mais quieto é o alerta de regressão de achado.
- Anote o que mudou no mundo real. Sem isso, a próxima comparação não tem contexto.

**Termos desta tela**

- **Regressão** — Um sinal de que algo piorou em relação à referência, nas duas visitas completas comparadas.
- **Anotação** — Um registro humano de implantação ou manutenção, preso ao ambiente e à data.

### Ações assistidas

<!-- topic: acoes | abrir: Ações assistidas -->

Uma ação assistida é um plano para a equipe: o que conferir, o risco, como validar e como voltar atrás. O auditor escreve o plano. A mudança, se acontecer, é feita fora dele, no banco, por quem tem essa responsabilidade.

**Como usar**

1. Escolha um achado e registre o responsável.
2. Leia os passos de confirmação antes de qualquer mudança.
3. Depois que a mudança for feita lá fora, marque a ação como executada externamente.
4. Rode uma nova coleta comparável e veja a medição. Uma diferença observada não prova, sozinha, que a mudança foi a causa.
5. Exporte o histórico em JSONL ou CSV quando precisar da trilha completa do ambiente. Confira a marca de fim do arquivo.

**Vale saber**

- O diagnóstico opcional de qualidade lê uma amostra limitada, com conta de leitura, e guarda só contagens. A margem de erro dessa amostra não é conhecida.
- Compare visitas com cobertura, perfil e carga parecidos. Visitas diferentes demais não sustentam a conclusão.

**Termos desta tela**

- **Medição** — A comparação cautelosa de um número antes e depois, entre duas visitas parecidas.
- **Amostra** — Um pedaço limitado de linhas. Ela pode não representar a tabela inteira.

### Contas e acesso

<!-- topic: contas | abrir: Contas -->

Cada pessoa entra com a própria conta. A senha não deve ser compartilhada. Quem administra as contas é o operator. Os outros papéis só veem e agem dentro dos ambientes que receberam.

**Como usar**

1. Entre com a sua conta. Se o computador for compartilhado, saia ao terminar.
2. Viewer consulta. Auditor também registra decisão e pede relatório. Operator administra contas, execuções e baselines.
3. Para criar uma conta, um operator informa o nome, uma senha longa, o papel e os ambientes permitidos.
4. O código do aplicativo autenticador é opcional. Em Contas, gere o segredo, confirme o código de seis dígitos e guarde os códigos de recuperação. Só um operator pode passar a exigir esse código na própria conta.
5. Se perder o acesso de todos os operators, não apague as contas. Peça o procedimento de recuperação a quem opera o servidor.

**Vale saber**

- Trocar a senha no arquivo de configuração, depois que a conta já existe, não altera a senha gravada.
- Não há envio de código por SMS.
- Senha de banco e connection string não aparecem nesta tela.

**Termos desta tela**

- **Sessão** — O período em que você permanece dentro da aplicação, até oito horas, ou até usar Sair.
- **Escopo** — Os ambientes que uma conta pode abrir. Operator vê a administração. As outras contas ficam nos ambientes marcados.
- **Código de recuperação** — Uma reserva de uso único, mostrada uma vez, para entrar se o aplicativo autenticador não estiver à mão.

### Status

<!-- topic: status | abrir: Status -->

Status diz se a aplicação está de pé: a API, o banco interno onde os retratos ficam guardados, as conexões com os ambientes e as tarefas recentes.

**Como usar**

1. Abra Status quando uma tela parecer parada ou vazia sem explicação.
2. Confira se a API e o armazenamento interno estão saudáveis.
3. Veja a conexão de cada ambiente. Indisponível ou sem configuração explica uma coleta que não começa.
4. Se você acabou de mudar a configuração do servidor, reinicie a API e volte aqui antes de iniciar outra visita.

**Vale saber**

- Métricas internas não ficam publicadas no endereço do site. Elas existem só na rede de operação.
- Um ambiente saudável aqui ainda pode ter uma coleta parcial. Os dois avisos contam histórias diferentes.

**Termos desta tela**

- **API** — O programa que a tela consulta. Se ela não responde, as páginas não conseguem carregar dados.
- **Banco interno** — Onde o auditor guarda contas, execuções, inventário e achados. Não é o banco analisado.

### Quando algo parecer errado

<!-- topic: duvidas -->

Antes de concluir que o banco piorou, ou que a tela quebrou, confira se você está olhando o lugar certo. A maior parte das surpresas vem de ambiente, execução ou filtro.

**Como usar**

1. Confira o ambiente selecionado, a execução e os filtros.
2. Veja se a coleta e a análise terminaram e se a cobertura está completa.
3. Leia o aviso da tela. Ele pode falar de permissão, limite, versão incompatível ou erro temporário.
4. Repita a ação só se ela for segura. Para uma falha que continua, envie a quem opera o horário, o ambiente, o identificador da execução e o código do pedido, se a mensagem mostrar um.
5. Não envie senha, connection string nem conteúdo de linha.

**Vale saber**

- Lista vazia com coleta parcial não é um banco limpo.
- Um número que caiu a zero pode ser reinício de estatística, não uma melhora.
- Se o guia desta página não abrir um tópico, clique de novo no título. Só um tópico fica aberto por vez. O texto aparece logo abaixo dele.

**Termos desta tela**

- **Filtro** — Uma restrição da lista, como gravidade ou esquema. Limpar o filtro mostra de novo o conjunto inteiro daquela execução.

<!-- ajuda-por-tela:fim -->
