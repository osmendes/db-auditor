# DB Auditor

**Comece pelo [guia do usuário](docs/guia-do-usuario.md).** Operação, backup e retenção ficam no [guia operacional](docs/ops.md). Estes dois são a documentação canônica; os demais arquivos em `docs/` são notas de apoio.

O [registro de verificação do backlog](docs/verificacao-do-backlog.md) reúne os testes executados e os aceites que dependem da infraestrutura e de usuários em homologação.

Serviço **somente leitura** de inventário, comparação e diagnóstico de ambientes PostgreSQL/TimescaleDB e catálogo de metadados MongoDB.

**MVP 0.12** — sprints 0–12 no backlog concluídas no repositório (fundação → inventário → findings → performance → ops → dashboard → release).

## Pré-requisito no host

Apenas **Podman** (ou Docker) com **Compose**. Não é necessário instalar Go, Bun, Node, sqlc ou curl na máquina local — o `Makefile` executa ferramentas em containers.

```bash
# exemplo Fedora / RHEL
sudo dnf install podman podman-compose
```

## Estrutura canônica

```text
.
├── backend/          # API Go + sqlc + migrations + Containerfile
├── frontend/         # React + TS + Tailwind + Bun
├── deploy/           # compose.prod, Caddy, Grafana stub
├── docs/             # ops, mvp-release, hardening
├── compose.yaml
├── Makefile
└── README.md
```

## Ambiente local

```bash
cp .env.example .env   # defina POSTGRES_PASSWORD
make up
make smoke
```

| Serviço | URL |
|---------|-----|
| Frontend | http://localhost:5173 |
| API | http://localhost:8080 |
| Metrics | `api:9090` inside the Compose network only. The port is not published on the host. |
| Status | http://localhost:8080/api/v1/status |
| Dashboard KPIs | http://localhost:8080/api/v1/analytics/kpis |

### Primeiro acesso

Usuário e senha vêm do `.env`. O exemplo local é:

| Campo | Valor em `.env.example` |
| --- | --- |
| Usuário | `admin` (`AUDITOR_BOOTSTRAP_USER`) |
| Senha | `db-auditor-local-1` (`AUDITOR_BOOTSTRAP_PASSWORD`, 18 caracteres) |

Copie essas duas variáveis para o seu `.env` se ele foi criado antes. A senha precisa ter pelo menos 16 caracteres. Troque o valor antes de expor o serviço.

```bash
make up    # recompila a API
```

Abra http://localhost:5173 e entre com esse par. A conta `operator` só é criada se `auditor_user` estiver vazia. Mudar o `.env` depois **não** troca uma senha já gravada. Se já houver um operador autenticado, ele pode criar outra conta pela API administrativa. Se todos os acessos de operador forem perdidos, preserve o banco interno e use o procedimento de recuperação controlada descrito em [operações da segunda etapa](docs/second-stage-operations.md); não apague `auditor_user`, pois isso remove contas existentes. A sessão dura 8 horas e usa cookie HttpOnly; um reload verifica a sessão com a API e não exige novo login enquanto ela estiver válida. O frontend deve acessar `/api` na mesma origem por proxy.

O schema está em `backend/migrations/01_baseline.sql` mais o seed local `02_seed_demo.sql`. Volumes antigos não reaplicam esse diretório; a API aplica migrações incrementais e cria as tabelas de login se faltarem. Para diagnóstico opcional de dados, capacidades por mecanismo e medições de ações, veja [docs/third-stage-operations.md](docs/third-stage-operations.md).

Produção (VPS): ver `docs/ops.md` e `deploy/compose.prod.yaml`. Configure `AUDITOR_BOOTSTRAP_USER` e `AUDITOR_BOOTSTRAP_PASSWORD` no ambiente do Compose; o arquivo de produção os repassa ao container da API.

Release MVP: `docs/mvp-release.md`.

### Relatórios PDF (Sprints 18–19)

As funcionalidades da segunda etapa (planos de ação, gestão de contas, redação de PDFs, acompanhamento e regressões) estão descritas em [docs/second-stage-operations.md](docs/second-stage-operations.md). As recomendações são orientações para revisão humana: o auditor não executa alterações no banco analisado.

Na Sprint 20, relatórios e aprovação de baseline usam a sessão da conta local. O primeiro acesso está na seção acima. Use HTTPS em produção.

Os pedidos são assíncronos e idempotentes por execução, versão de regras, tipo e filtros. Os PDFs ficam no snapshot store por 30 dias; o registro do pedido permanece como trilha de proveniência até que uma política de arquivamento aprovada seja aplicada. Uma execução parcial aparece com cobertura limitada e não é usada para inferir resolução de findings. A geração é limitada a 500 bancos, 5.000 tabelas, 2.000 findings e 16 MiB por PDF, com truncamento declarado no documento.

Em uma instalação com volume PostgreSQL existente, não rode `make reset-volume`: ele apaga snapshots, findings e relatórios. A API cria as tabelas de login se o volume for anterior a elas. Veja [o guia operacional da Sprint 20](docs/sprint20-operations.md) e `backend/migrations/README.md`.

Relatórios só podem ser solicitados para execuções de auditoria concluídas cuja análise de regras também tenha terminado com sucesso. O trabalho registra o hash do catálogo de regras da análise (ou a versão do analisador para execuções anteriores à migração).

## Qualidade

```bash
make backend-fmt backend-check backend-staticcheck backend-lint backend-test
make frontend-check frontend-typecheck frontend-test
```

## Segurança

- Ambientes auditados: somente leitura
- Secrets fora do repositório
- Connection strings sanitizadas em logs
- Allowlist/denylist de databases e schemas
- Analyzers **nunca** recomendam DROP/REVOKE/VACUUM automático

## Documentação de produto

Notion: *Anotações / DB Auditor* (EF, Backlog, Fluxo de Desenvolvimento).
