# Notas de operação por etapa

Este arquivo junta as notas que antes ficavam em `sprint20-operations.md`, `sprint20-verification.md`, `second-stage-operations.md` e `third-stage-operations.md`. Os dois textos canônicos continuam sendo o [guia do usuário](guia-do-usuario.md) e o [guia de operação](ops.md); aqui ficam decisões e limites registrados em cada etapa.

- [Sprint 20](#sprint-20)
- [Verificação do Sprint 20](#verificação-do-sprint-20)
- [Segunda etapa](#segunda-etapa)
- [Terceira etapa](#terceira-etapa)

## Sprint 20

_segurança, operação e retenção._

### Primeiro acesso na tela de login

Usuário e senha ficam no `.env`:

```bash
AUDITOR_BOOTSTRAP_USER=admin
AUDITOR_BOOTSTRAP_PASSWORD=db-auditor-local-1
```

A senha do exemplo tem 18 caracteres. O mínimo é 16. O Compose local entrega o `.env` ao container da API. Entre em http://localhost:5173 com `admin` / `db-auditor-local-1`, depois troque a senha do `.env` **antes** da primeira subida se o serviço for compartilhado. Mudar o `.env` depois não atualiza o hash.

A conta `operator` nasce uma vez, quando `auditor_user` está vazia. Se o login falhar:

- as duas variáveis não estavam no `.env` na subida que criou a conta;
- a senha tem menos de 16 caracteres;
- já existe um usuário e o bootstrap foi ignorado. Use uma conta de operador existente para criar outra conta pela API administrativa. Se todos os acessos de operador forem perdidos, preserve os dados e planeje uma recuperação controlada a partir de backup. Não apague `auditor_user`.

`AUDITOR_BOOTSTRAP_PASSWORD_FILE` continua válido apenas quando a variável de senha está vazia. Não coloque a senha no frontend nem no Git além do exemplo local. `deploy/compose.prod.yaml` repassa as variáveis de bootstrap configuradas no ambiente do Compose.

### Migração sem perda de dados

Faça backup do snapshot store antes de atualizar uma instalação que já tem dados.
O schema novo de uma instalação vazia é só `backend/migrations/01_baseline.sql`
e `02_seed_demo.sql`. Os arquivos `03`–`10` foram incorporados no baseline e
removidos. O entrypoint só roda em volume vazio. `make reset-volume` **apaga**
snapshots, findings, baselines e relatórios.

Num volume antigo, `auditor_user`, `auditor_session`, `auditor_login_attempt`
e `auditor_operation_log` vêm das migrações versionadas `16_identity.sql`,
`18_totp_discard_retention.sql` e `21_identity_login_block.sql`, registradas em
`schema_migration`. A API não cria tabelas fora delas. Não aplique o baseline
de novo por cima desse volume.

O `operator` pode criar outras contas pela rota administrativa
`POST /api/v1/auth/users` com `username`, `password` (mínimo 16 caracteres),
`role` (`viewer`, `auditor`, `operator`) e `environments` (lista de UUIDs).
Não-operadores precisam de ambientes explicitamente atribuídos. Sessões são
temporárias e revogáveis. A interface usa cookie HttpOnly, SameSite=Lax e
Secure quando a requisição chega por HTTPS; JavaScript não lê o token. O
`GET /api/v1/auth/me` restaura a identidade e um token CSRF mantido somente
na memória da página. Mutações com sessão por cookie exigem `X-CSRF-Token`.
Clientes de API que já usam bearer continuam aceitos; o login sem
`?mode=cookie` mantém esse contrato. Não armazene o bearer no navegador.
Somente HTTPS deve ser exposto publicamente; a API não termina TLS por conta
própria, então use um proxy HTTPS confiável.

#### Login após reload e limite de requisições

O reload consulta `GET /api/v1/auth/me` com o cookie da sessão.
Se a API responder 401, a sessão expirou ou foi revogada e é preciso entrar
novamente. Se responder 429 ou estiver indisponível, a tela oferece uma nova
tentativa sem revogar a sessão; aguarde antes de tentar de novo.

A API limita o login a 120 tentativas por minuto por endereço remoto e 10 por
conta em cinco minutos, usando o banco interno para coordenar réplicas. Os
contadores de conta são limpos após login bem-sucedido. Requisições autenticadas
mantêm limite independente de 600 por minuto por usuário. Para investigar um 429,
veja no painel Network do navegador qual rota respondeu 429 e quantas vezes
ela foi chamada no minuto. O Inventário oferece 20, 50 ou 100 linhas por
página e faz paginação no servidor. Se vários navegadores ou integrações usam a mesma conta, as chamadas
deles também compartilham o limite desse usuário. Não aumente o limite antes
de identificar a origem das chamadas.

Cada destino auditado exige `sslmode=verify-full` e host exato em
`AUDITOR_TARGET_ALLOWED_HOSTS`. Certificados devem encadear para uma CA
confiável no container (ou ser fornecidos por configuração segura do driver).
`AUDITOR_ALLOW_INSECURE_LOCAL_TARGETS=true` é uma exceção **apenas para
localhost/loopback em desenvolvimento** com `sslmode=disable`.

### Credenciais e rotação

Hoje as credenciais de bancos auditados continuam em secrets/variáveis do
backend e nunca são enviadas ao navegador. Para girá-las: crie credencial
somente leitura nova no alvo, atualize o secret fora do Git, reinicie a API,
confirme conexão e coleta, então revogue a antiga. Não registre DSNs completos.

Armazenamento cifrado no snapshot store é um plano, não está habilitado:
usar envelope encryption (chave de dados aleatória AES-256-GCM por segredo,
nonce exclusivo, chave mestra em KMS/secret manager), guardar `key_version`,
ciphertext e metadados não sensíveis; ler versão antiga e nova durante
rotação, recriptografar em lotes auditados, revogar versão antiga só após
verificação e rollback. Nunca guardar a chave mestra junto com o ciphertext.
Uma futura migração exigirá threat model, testes de recuperação/rotação e
aprovação operacional antes de trocar as variáveis atuais.

### Retenção e minimização

| Dado | Política |
| --- | --- |
| Sessões | Expiram após 8 horas; logout revoga imediatamente. A API remove registros expirados a cada hora. |
| Artefatos PDF | Expiram em 30 dias; o worker remove os expirados. |
| Jobs de relatório | Metadados podem ser mantidos para trilha; definir janela de arquivamento conforme compliance local. |
| Log de operação | Meta de 365 dias; exportar para armazenamento imutável antes de purgar. Purga ainda exige job e aprovação de retenção. |
| Snapshots/runs/findings/eventos | Não apagar automaticamente: baselines, comparações e trilhas dependem deles. Definir janela e exportação/arquivamento após avaliação legal/operacional. |

`AUDITOR_REPORT_REDACT_METADATA` aceita `environment`, `run_id`,
`requested_by`, `service_version`, `rule_version` (separados por vírgula),
ocultando esses campos do cabeçalho de PDFs futuros sem alterar dados
persistidos. Valide os relatórios gerados antes de compartilhar externamente;
evidências e nomes de objetos podem continuar sensíveis.

### Limites operacionais

`AUDITOR_MAX_CONCURRENT_RUNS`, `AUDITOR_MAX_COLLECTOR_WORKERS` e
`AUDITOR_MAX_DATABASE_CONNECTIONS` limitam runs, collectors por run e
conexões por database no processo. `AUDITOR_TARGET_STATEMENT_TIMEOUT` limita
cada consulta no servidor auditado; os collectors opcionais têm limite de
linhas, duração (`AUDITOR_OPTIONAL_COLLECTOR_TIMEOUT`) e custo estimado do
plano (`AUDITOR_OPTIONAL_MAX_PLAN_COST`). Um plano acima do limite vira falha
parcial auditável, não execução irrestrita. O inventário usa páginas de até 100 linhas
na interface e até 500 por requisição na API. Inspecione planos `EXPLAIN (ANALYZE, BUFFERS)` em cópia de
dados representativa antes de elevar limites. Os índices de paginação ficam no
baseline.

O Inventário não oferece “Todas”: os totais vêm do servidor e os objetos são
carregados por página, inclusive hypertables e agregados contínuos.

## Verificação do Sprint 20

O Sprint 20 não é reimplementado aqui. A evidência está nos testes e nas rotas que já existem na linha principal.

- Paginação de achados, inventário e execuções: `ListFindingsPage`, `ListFindingsCategoryPage` e as páginas do frontend com 20/50/100/todos.
- Totais do dashboard: KPIs usam a agregação do snapshot, não a soma da página visível.
- Escala: `sprint20_scale_integration_test.go` roda com `AUDITOR_TEST_SCALE=1` contra o Postgres descartável da CI. O job Backend publica no resumo da execução uma linha `SCALE_RESULT` com volume (linhas, bancos, esquemas, tabelas, tamanho de página) e tempos (carga, última página, p95, total).
- O que ainda era lacuna do sprint entrou nas ondas anteriores: rotas no fragmento, uma lista de achados, cobertura parcial visível e retenção de relatório.

Nenhum coletor passou a escrever no banco auditado.

## Segunda etapa

_operação e limites._

### Atualização do banco interno

Faça backup verificável do banco de controle antes de implantar. A API aplica `06_second_stage.sql` uma única vez, após as migrações anteriores, criando apenas tabelas de planos de ação, eventos, anotações e alertas. Não execute essa migração no banco auditado. Confira `schema_migration` e os logs de inicialização antes de liberar a interface. Em caso de falha, interrompa o serviço, restaure o backup do banco de controle em uma instância separada, confira a compatibilidade do binário e só então redirecione o serviço. Evite reverter a migração apagando tabelas, pois isso elimina decisões e trilhas registradas.

### Contas, sessões e permissões

Cada usuário autenticado pode trocar a própria senha em **Contas**. A troca invalida todas as suas sessões, inclusive a atual. Um operador pode listar contas, alterar papel, escopos e estado, definir uma nova senha e revogar sessões. Alterar conta também revoga sessões. O último operador ativo não pode perder o papel ou ser desativado. Senhas novas devem ser comunicadas por um canal seguro fora do auditor; não as inclua em tickets ou relatórios. Revise o log de operações para mudanças administrativas. Se todos os operadores perderem acesso, siga [Recuperar o acesso de operador](ops.md#recuperar-o-acesso-de-operador).

### Recomendações para o banco auditado

Em **Achados**, cada ocorrência tem um plano com benefício esperado sem estimativa inventada, riscos, pré-requisitos, confirmação, validação, possibilidade de falso positivo e, quando disponível, uma consulta somente leitura parametrizada. A consulta exige que o operador forneça o identificador do objeto após confirmar o escopo; não concatene nomes recebidos da interface em SQL. Os grupos cobrem estrutura (chaves, constraints e índices inválidos), consulta e índice, manutenção e crescimento, segurança e investigação. Marque o responsável e a justificativa antes de planejar; registre o resultado antes de validar. A decisão e o histórico persistem quando o achado reaparece. Toda alteração proposta requer revisão do dono da aplicação, backup, janela e rollback externos. Nenhuma recomendação aplica DDL, DML, VACUUM ou configuração ao banco auditado.

### PDFs e privacidade

O formulário oferece `none`, `identifiers` e `strict`. Nos dois últimos níveis, o PDF mascara nomes de ambientes, bancos, esquemas, tabelas e objetos, solicitante, descrições originais, evidências e notas de cobertura. A redação atua na cópia renderizada; snapshots e metadados continuam no banco interno. Compartilhe apenas o artefato gerado com o nível adequado e proteja o acesso ao armazenamento e aos backups. Revise o PDF antes de distribuição externa; IDs de execução, achado e regras continuam visíveis para rastreabilidade. Os relatórios exibem gráficos de severidade e crescimento, cobertura, confiança e prioridades, e terminam com análise e próximos passos. A variação de crescimento requer coletas comparáveis para interpretação.

### Comparações, anotações e alertas

A nota `scope-v3` usa segurança 35%, performance 30%, estrutura 20% e manutenção 15%. Cada achado desconta 25, 15, 8, 3 ou 1 ponto conforme severidade crítica, alta, média, baixa ou informativa, limitado a 100 por categoria. Na categoria estrutura, a base é 80 pontos e chaves primárias verificadas nas tabelas observadas acrescentam até 20 pontos. A nota fica indisponível quando coleta ou análise não permite conclusão. A comparação explica a variação por categoria, quantidade de achados, penalidade e evidência positiva. Compare notas apenas com cobertura completa, mesmo perfil, mesma versão de coletor, mesmas regras e mesma fórmula. A série temporal preserva lacunas, perfis e versões; não trate ausência de coleta como zero. Anote implantações, manutenção e incidentes com data para contextualizar mudanças.

O alerta persistente é criado quando dois runs completos e analisados após a referência aprovada apresentam mais achados altos/críticos que ela, ou quando duas comparações consecutivas registram mudança ou remoção estrutural. Perfil e versões precisam ser iguais entre os três runs. A avaliação é idempotente e fica no banco interno. Um auditor pode reconhecer o alerta com justificativa. Um alerta é sinal de investigação, não prova de causalidade; reavalie após novo run. Alertas não geram notificações externas nesta etapa.

### Retenção e comparabilidade

O painel limita a exibição a 100 anotações e 100 alertas por ambiente; isso não apaga dados. A retenção de runs, snapshots, eventos, decisões e referências depende da política do operador: mantenha ao menos a janela necessária para comparações e obrigações de auditoria. Antes de arquivar, exporte o banco de controle com `pg_dump -Fc`, proteja o arquivo, teste uma restauração isolada e documente a data, a versão do serviço e o período coberto. Não apague runs ainda referenciados por baseline, alerta ou evidência de uma ação. Os PDFs seguem a retenção de artefatos já descrita em [Sprint 20](#sprint-20).

## Terceira etapa

_capacidades, diagnóstico e resultados._

### Implantação e retorno

Faça backup verificável do banco de controle antes da implantação. A API aplica `07_third_stage.sql`, `08_action_workload.sql`, `09_query_action_metrics.sql`, `10_account_activity.sql` e `11_quality_validation.sql` uma vez e registra o checksum em `schema_migration`. A migração adiciona `audit_environment.engine` com valor padrão `postgresql` e tabelas de diagnósticos, eventos e medições. Não executa DDL no banco auditado. Confira o estado da migração, o boot da API e as rotas de capacidades após atualizar. Em caso de falha, interrompa a API, restaure o backup em instância separada, valide a versão do binário e só então aponte o serviço para a restauração. Não apague tabelas para tentar reverter: elas guardam decisões e evidências.

### Contrato de mecanismos

O contrato `capabilities.Fact` separa fato, escopo, valor e confiança; `Adapter` declara mecanismo e capacidades. Há coletores PostgreSQL/TimescaleDB e MongoDB. O adaptador MongoDB lê metadados de coleções e índices, sem ler documentos; regras PostgreSQL não são aplicadas a ele. A matriz de capacidades identifica regras Timescale como aplicáveis apenas quando a extensão foi observada na execução. Mecanismos desconhecidos não são coletados por um conector PostgreSQL. Um novo mecanismo exige adaptador, testes de leitura, mapeamento de fatos, regras aplicáveis e versão de contrato antes da habilitação. Ausência de capacidade não equivale a ausência de problema.

### Contas e permissões do banco auditado

Na coleta manual ou mensal do PostgreSQL, o auditor registra papéis que podem entrar, prazo de validade e se havia uma sessão ativa naquele instante. Conta com validade expirada recebe sugestão de revisão. Se ao menos duas coletas ao longo de 30 dias não observaram sessão ativa, o auditor sinaliza uma **hipótese** de inatividade. Sessões podem acontecer entre as coletas: o sinal não é prova de último login nem autorização para remover a conta. Confirme identidade, aplicação e uso real com o administrador antes de alterar acesso externamente. A tela de avaliação da tabela mostra permissões efetivas por papel e objeto, incluindo herança; verifique a cobertura da coleta antes de concluir que um privilégio não existe. Nenhuma credencial é armazenada.

### Diagnóstico opcional de qualidade

`AUDITOR_DATA_QUALITY_ENABLED=1` habilita a criação de diagnósticos; o padrão é desligado. Um auditor escolhe banco, esquema, tabela, até 1.000 linhas, colunas esperadas não nulas, chaves candidatas e intervalos de data. Há no máximo 32 verificações, oito relações simples e 20 segundos por solicitação, com limite de 5 segundos por instrução. O conector abre transação somente leitura e recusa papéis administrativos ou com permissão de escrita na tabela. Configure uma conta dedicada de leitura mínima em `AUDITOR_TARGET_N_*`; conceda acesso apenas aos objetos aprovados. A solicitação pode ser cancelada pela conexão HTTP.

São persistidos apenas escopo, contagens, usuário, data, decisões e evidências textuais inseridas pelo usuário. Valores e linhas da amostra não saem do banco auditado. Não inclua dados pessoais em justificativas ou evidências. Em tabelas com estimativa maior que o dobro do limite, o método usa `TABLESAMPLE SYSTEM` com semente fixa e limita a até 1.000 linhas. Isso seleciona páginas, não linhas independentes; não existe margem de erro estatística calculável. Em tabelas menores, usa leitura limitada em ordem física. Mudanças na tabela podem trocar a amostra. Contagens de diagnósticos diferentes só aparecem como comparáveis quando método, limite e quantidade de linhas coincidem, ainda com ressalva de amostra. Confirme a população e a regra de negócio antes de agir. Chaves estrangeiras simples visíveis ao papel de leitura permitem contagem de órfãos; relações compostas ficam fora deste diagnóstico.

### Plano externo de saneamento

Cada contagem gera roteiro de significado, confirmação, execução externa, validação e risco para nulos, duplicidades, órfãos, datas anômalas ou concentração. O auditor registra estado, responsável, justificativa, resultado e evento de decisão. O estado validado exige diagnóstico posterior da mesma verificação com método, limite e quantidade de linhas iguais; seu ID fica na ação e no evento. Texto livre não substitui essa nova coleta. Antes de alterar dados em outro sistema, confirme dependências e proprietário da regra, obtenha backup restaurável, planeje lotes e janela, teste o procedimento e documente rollback. O auditor nunca executa correção, DDL, DML ou configuração no banco analisado. Para validar, registre evidência externa e repita o diagnóstico quando possível; diferenças de amostra não demonstram por si só que o saneamento funcionou.

### Medições de ações

Uma medição liga um achado a duas execuções cronológicas do mesmo ambiente. O auditor aceita `finding_observed` (0/1), `table_size_bytes` e, para achados com fingerprint de consulta, latência média em microssegundos e blocos lidos por mil chamadas. Estas duas últimas usam estatísticas acumuladas do `pg_stat_statements`; só são comparáveis se o `stats_reset` for igual nas duas coletas. Mesmo assim, a mistura de cargas no intervalo pode afetar a média. O usuário informa hipótese e janela. A comparação só é marcada válida com valores presentes, execuções e análises concluídas, cobertura completa, mesmo perfil, coletor e conjunto de regras **e confirmação explícita de carga semelhante pelo operador**. Medições antigas ficam não confirmadas pela migração 08. A diferença observada não prova causalidade: documente carga, mudanças simultâneas e período de observação. Para `index.unused` com tamanho coletado, o painel mostra o tamanho do índice como limite superior de espaço candidato; para `vacuum.high_dead_tuples`, mostra proporção estimada de bytes associados a tuplas mortas. Nenhum desses números promete economia no sistema de arquivos. Se um achado volta a ser observado após uma medição comparável, uma ação validada ou executada externamente retorna para análise com evento na trilha. O painel pagina ações e medições e exporta todo o histórico do ambiente em JSONL ou CSV por streaming; o marcador final confirma que o arquivo terminou. Proteja a exportação como o banco de controle.

### PDF, retenção e capacidade

O renderizador impõe 8 MiB de texto de entrada, 200 páginas e 16 MiB ao PDF final, com erro amigável quando o escopo excede o limite. Artefatos expiram após 30 dias: a limpeza remove o binário, mas preserva o job como trilha de proveniência. O worker recupera jobs interrompidos após reinício conforme a política existente de três tentativas. O PDF fica na tabela `report_artifact` do banco de controle; proteja o volume e os backups com criptografia, acesso mínimo e política de descarte. A aplicação não criptografa o PDF separadamente. Monitore volume, duração, falhas de geração e crescimento de `report_job`, `quality_scan`, `quality_issue_event` e `finding_action_measurement` no banco de controle. O histórico não tem exclusão automática. Defina retenção conforme obrigação de auditoria: faça `pg_dump -Fc`, restaure em instância isolada, documente janela e versão, preserve referências a baselines, achados e medições, e só então arquive dados fora da aplicação com procedimento aprovado. Uma expiração de PDF não elimina histórico nem constitui autorização para apagar runs.
