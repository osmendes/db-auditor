# DB Auditor

O DB Auditor olha para um banco de dados e conta, em linguagem clara, o que encontrou: o que existe, o que parece em risco e o que mudou desde a última vez.

Ele foi feito para quem cuida de bancos PostgreSQL, TimescaleDB ou do catálogo do MongoDB e precisa de um panorama confiável — sem abrir dezenas de consultas à mão e sem medo de alterar alguma coisa por engano.

## Qual problema ele resolve

Bancos crescem, ganham tabelas, índices, permissões e rotinas. Com o tempo fica difícil responder perguntas simples:

- O que existe neste ambiente, e quanto espaço isso ocupa?
- Há sinais de risco, como permissão ampla, índice sem uso ou rotina privilegiada?
- Isso piorou ou melhorou em relação à coleta anterior?
- Dá para mostrar um resumo para quem não lê SQL?

O DB Auditor faz uma **coleta somente leitura**, guarda o retrato e mostra o resultado em telas: painel, inventário, achados, comparação entre execuções e relatório em PDF.

Um achado é um **sinal para investigar**, não uma ordem para mudar o banco. A pessoa responsável confirma antes de agir. O auditor não apaga dados, não revoga acessos e não executa manutenção no banco analisado.

## O que você consegue fazer

- Entrar com uma conta e escolher o ambiente.
- Rodar uma auditoria e ver se a coleta foi completa, parcial ou falhou.
- Percorrer tabelas, índices, visões, funções e, no TimescaleDB, tabelas temporais e agregados.
- No MongoDB, ver coleções e índices. O auditor não lê os documentos.
- Ler achados com o que foi observado, a evidência e um próximo passo sugerido.
- Comparar duas execuções, acompanhar regressões e baixar um PDF.
- Opcionalmente, pedir um código do aplicativo autenticador (TOTP) para contas de operador.

Os endereços e senhas dos bancos analisados ficam na configuração do servidor. Eles não são digitados na tela e não aparecem nos relatórios.

Quem vai usar o dia a dia pode seguir o [guia do usuário](docs/guia-do-usuario.md). Quem instala, faz backup ou coloca no ar segue o [guia de operação](docs/ops.md).

## O que precisa estar instalado

No computador onde o sistema vai rodar, basta **Podman** ou **Docker**, com **Compose**. Não é preciso instalar Go, Node ou banco na máquina: o projeto sobe as peças dentro de containers.

No Fedora ou RHEL, por exemplo:

```bash
sudo dnf install podman podman-compose
```

Se usar Docker, os mesmos comandos `make` funcionam. Antes, avise o projeto:

```bash
make up COMPOSE="docker compose"
```

Você pode gravar essa escolha no ambiente, para não repetir em todo comando.

## Como colocar para funcionar

1. Copie o modelo de configuração:

   ```bash
   cp .env.example .env
   ```

2. Abra o arquivo `.env` e troque pelo menos estes valores:

   | O que é | Variável | O que fazer |
   | --- | --- | --- |
   | Senha do banco interno do auditor | `POSTGRES_PASSWORD` | Use uma senha sua. O exemplo `change-me-locally` não deve ir para um servidor exposto. |
   | Usuário do primeiro acesso | `AUDITOR_BOOTSTRAP_USER` | O exemplo é `admin`. |
   | Senha do primeiro acesso | `AUDITOR_BOOTSTRAP_PASSWORD` | Precisa ter **no mínimo 16 caracteres**. O exemplo local é `db-auditor-local-1`. Troque antes de expor o serviço. |
   | Nome da empresa | `AUDITOR_COMPANY_NAME` | Aparece debaixo do título DB Auditor. O padrão é `db-auditor`. |

3. Suba o sistema:

   ```bash
   make up
   ```

4. Abra no navegador: <http://localhost:5173>

   Entre com o usuário e a senha do passo 2.

5. Se quiser só conferir se a API respondeu:

   ```bash
   make smoke
   ```

Para parar: `make down`. Para ver o que está acontecendo: `make logs`.

### Primeiro acesso, em poucas regras

- A conta de operador só é criada se ainda não existir nenhuma conta.
- Mudar o `.env` **depois** não troca uma senha que já foi gravada.
- A sessão dura até 8 horas. Recarregar a página não pede login de novo enquanto ela valer.
- Se todas as contas de operador se perderem, não apague os usuários. Siga o procedimento de recuperação no [guia de operação](docs/ops.md#recuperar-o-acesso-de-operador).

### Ligar um banco para ser auditado

No mesmo `.env`, dê nome aos ambientes que a tela vai mostrar. Isso vale na primeira criação do banco interno. Use um UUID em cada id. O exemplo já traz dois, para você trocar:

```bash
AUDITOR_COMPANY_NAME=Nome da empresa

AUDITOR_TARGET_1_NAME=Banco principal
AUDITOR_TARGET_1_ENVIRONMENT_ID=00000000-0000-0000-0000-000000000001
AUDITOR_TARGET_1_TYPE=self_hosted
AUDITOR_TARGET_1_DISCOVERY=single_database
AUDITOR_TARGET_1_HOST=endereco-do-banco
AUDITOR_TARGET_1_PORT=5432
AUDITOR_TARGET_1_DATABASE=nome_do_banco
AUDITOR_TARGET_1_USER=usuario_somente_leitura
AUDITOR_TARGET_1_PASSWORD=senha-desse-usuario
AUDITOR_TARGET_1_SSLMODE=verify-full
```

O segundo espaço segue o mesmo desenho (`AUDITOR_TARGET_2_...`), até o espaço 20. Se nenhum id for preenchido, uma instalação nova cria dois ambientes de demonstração.

Um volume que já existe não apaga ambientes antigos. Se você mantiver o mesmo id e mudar só o nome, a API atualiza o nome na próxima subida. Um id novo cria outro ambiente. Para recomeçar do zero em desenvolvimento, `make reset-volume` apaga o banco interno — não use isso onde já há coletas.

Use uma conta **somente leitura**. O padrão de TLS é `verify-full` (confere o certificado). Informe o endereço em `AUDITOR_TARGET_ALLOWED_HOSTS`; sem isso, o auditor recusa a conexão. Conexão sem essa verificação só é aceita se o host estiver, de propósito, em `AUDITOR_TARGET_INSECURE_HOSTS`. Deixe essa lista vazia, a menos que a política da rede exija o contrário.

Depois de salvar o `.env`:

```bash
make down && make up
```

Não rode `make reset-volume` numa instalação que já tem coletas. Esse comando apaga o banco interno: snapshots, achados e relatórios.

Para um servidor de verdade (domínio, HTTPS, backup), use o [guia de operação](docs/ops.md) e o exemplo em `deploy/env.prod.example`.

> **Atenção: porta 9090 (métricas).** A API expõe `GET /metrics` em `:9090` **sem autenticação**: os contadores revelam volume de execuções, erros e rotas. O Compose não publica essa porta no host, e o Caddy não encaminha `/metrics`. O scrape fica em `api:9090`, dentro da rede do Compose. Não acrescente `9090:9090` (nem `127.0.0.1:9090:9090` num host compartilhado) em `ports:` e não abra 9090 no firewall da VPS. Detalhes no [guia de operação](docs/ops.md).

### Endereços neste computador

| O quê | Onde abrir |
| --- | --- |
| Telas | <http://localhost:5173> |
| API | <http://localhost:8080> |
| Saúde da API | <http://localhost:8080/api/v1/status> |

A tela fala com a API pelo mesmo endereço do navegador. Isso é o que mantém o login no cookie. Não aponte o navegador direto para outra origem sem ajustar `AUDITOR_CORS_ORIGINS`.

As métricas internas não ficam publicadas na sua máquina. Elas só existem na rede dos containers.

## Pastas do projeto

```text
.
├── backend/       o programa que coleta, analisa e guarda os resultados
├── frontend/      as telas do navegador
├── deploy/        como publicar em um servidor, com proxy e scripts de backup
├── docs/          guias para usar e operar
├── compose.yaml   sobe banco interno, API e telas juntos
├── Makefile       atalhos: subir, parar, testar
├── .env.example   modelo de configuração (copie para .env)
└── README.md      este texto
```

Dentro de `backend/`, o que mais importa no dia a dia:

- `cmd/` — ponto de entrada do programa
- `migrations/` — evolução do banco interno, aplicada pela própria API
- `sql/` — consultas versionadas

Dentro de `frontend/`, o código das telas fica em `src/`.

`docs/guia-do-usuario.md` e `docs/ops.md` são os dois textos principais. Os outros arquivos em `docs/` são notas de apoio.

## Tecnologias

| Peça | Para quê |
| --- | --- |
| Go | API e regras de análise |
| PostgreSQL | banco interno, onde ficam contas, coletas e achados |
| MongoDB (driver) | leitura do catálogo de coleções e índices, quando o ambiente é MongoDB |
| React, TypeScript e Vite | telas |
| Tailwind CSS | visual das telas |
| Bun | instalar e verificar o frontend |
| Podman ou Docker Compose | subir tudo sem instalar as linguagens na máquina |
| Caddy | entregar o site com HTTPS na instalação de produção |

## Como contribuir

Toda ajuda é bem-vinda: texto mais claro, correção, teste ou função nova.

1. Crie um ramo a partir de `main` com um nome curto, por exemplo `docs/ajuste-do-guia`.
2. Faça a mudança pequena e fácil de revisar.
3. **Nunca** envie o arquivo `.env`, senha, chave ou connection string. O modelo certo é o `.env.example`, com valores falsos.
4. Se a mudança mexe em comportamento, rode as verificações:

   ```bash
   make backend-fmt backend-check backend-test
   make frontend-check frontend-typecheck frontend-test
   ```

   A CI roda os testes de integração do backend em PostgreSQL 14, 15, 16, 17 e 18
   (detalhes em [docs/ops.md](docs/ops.md#matriz-de-versões-do-postgresql-na-ci-134)).

5. Abra um Pull Request neste repositório. No texto, diga o que mudou e por quê. Se existir uma issue, cite o número.

Não é obrigatório saber toda a arquitetura para corrigir documentação ou um texto da tela. Se a dúvida for de produto, comece pelo [guia do usuário](docs/guia-do-usuario.md).

## Cuidados que valem para todo mundo

- O banco analisado é só de leitura. Recomendações não são executadas sozinhas.
- Segredos ficam fora do Git.
- Senhas e endereços de conexão não devem aparecer em logs.
- Listas de bancos e schemas podem limitar o que entra na coleta. O que está na denylist nunca entra.
- Um PDF pedido fica guardado por 30 dias. O registro de que ele foi pedido permanece.
