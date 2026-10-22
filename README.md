# BVX Pulse · Cliente 360

MVP desenvolvido para o **Hackathon da Fatec Ourinhos em parceria com a BVX Tecnologia** (Desafio "Cliente 360").

O sistema monitora PDVs (Pontos de Venda) espalhados em clientes locais e identifica **proativamente** riscos de falha de backup, desatualização de sistema e churn, por meio de um **Health Score de 0 a 100** por cliente. Ele também prioriza quem o suporte deve atender primeiro e prescreve a ação a ser tomada.

## Componentes

| Componente | Descrição |
|---|---|
| `server` | Servidor central: API REST, motor de Health Score, motor de recomendações, SSE e dashboard web. |
| `pdv-simulator` | CLI interativo que simula o PDV enviando telemetria (heartbeats HTTP POST em JSON). |

## Stack

- **Go 1.22+**, apenas biblioteca padrão mais `modernc.org/sqlite` (SQLite puro, **sem CGO**).
- Frontend em arquivo único embutido no binário (`go:embed`): TailwindCSS, Lucide e Chart.js via CDN. **A máquina que abre o dashboard precisa de internet** para carregar os CDNs.
- Tempo real: Server-Sent Events (`/api/v1/events`) mais polling a cada 2 segundos.

## Como executar

Pré-requisito: Go 1.22 ou superior.

```bash
# Terminal 1: servidor + dashboard
go run ./cmd/server
# abra http://localhost:8080

# Terminal 2: simulador de PDV
go run ./cmd/pdv-simulator
```

Flags opcionais:
- Servidor: `-addr :8080` e `-db bvx-pulse.db`.
- Simulador: `-server http://localhost:8080`.

Na primeira execução o banco SQLite é criado e populado com 15 clientes (supermercados, padarias, restaurantes, postos e mercearias de Santa Cruz do Rio Pardo, Ourinhos, Marília, Assis e Bauru). Para reiniciar a demo do zero, apague os arquivos `bvx-pulse.db*` com o servidor parado.

Testes do motor de score:
```bash
go test ./...
```

## Regras do Health Score

```
Score = round( Backup×0,35 + Vendas×0,30 + Chamados×0,20 + Versão×0,15 )
```

Cada pilar vale de 0 a 100. O score é **calculado a cada leitura** a partir dos dados brutos e não é gravado no banco.

### Backup (35%)
A primeira condição que se aplica define a nota:

| Condição | Nota |
|---|---|
| `backup_integrity_ok == false` | 0 |
| `backup_size_mb` < 50% de `previous_backup_size_mb` | 0 |
| Menos de 1 dia (últimas 24h) | 100 |
| 1 dia | 75 |
| 2 a 3 dias | 50 |
| 4 a 6 dias | 20 |
| 7 dias ou mais | 0 |

### Vendas / Churn (30%)
`queda% = (sales_avg_weekly − sales_last_7d) / sales_avg_weekly × 100`

| Queda | Nota |
|---|---|
| Menos de 15% (estável ou crescendo) | 100 |
| 15% a menos de 30% | 60 |
| 30% a menos de 40% | 20 |
| 40% ou mais | 0 |

### Chamados (20%)

| `tickets_last_30d` | Nota |
|---|---|
| 0 a 2 | 100 |
| 3 a 5 | 60 |
| 6 ou mais | 10 |

`days_since_last_interaction` é recebido e exibido, mas **não** entra no cálculo.

### Versão (15%)
Versão oficial: `v5.2`.

| Versão | Nota |
|---|---|
| `v5.2` | 100 |
| `v5.1` | 60 |
| `v5.0` ou inferior | 10 |

### Classificação

| Score | Status | Cor |
|---|---|---|
| 75 a 100 | Baixo Risco | Verde |
| 50 a 74 | Atenção | Amarelo |
| 0 a 49 | Alto Risco | Vermelho |

### Premissas adotadas
O enunciado não definia alguns casos, e eu os preenchi assim:
- Backup com 1 dia de idade vale 75.
- Queda de vendas abaixo de 15% vale 100.
- Sem nenhuma ação aplicável e com score abaixo de 75, o motor mostra uma recomendação genérica de acompanhamento.

### Valores de referência dos clientes da demo

| Cliente | Score | Status |
|---|---|---|
| Supermercado Alvorada (Matriz): v5.0, backup de 7 dias, vendas −40%, 8 chamados | **4** | Alto Risco |
| Padaria & Confeitaria Estrela (Ourinhos): v5.2, backup de hoje 450MB, vendas em alta, 1 chamado | **100** | Baixo Risco |

> **Nota:** o enunciado indicava ~31 para o Alvorada e 95 para a Padaria. Aplicando as regras de pontuação exatamente como especificadas, esses valores não são atingíveis (o Alvorada soma 0 + 0 + 2 + 1,5 = 3,5). Optamos por manter as regras fiéis ao enunciado. Se a organização desejar calibrar pesos ou faixas, a lógica está isolada em `internal/health/health.go`.

## Motor de recomendação

Todas as ações aplicáveis são listadas juntas:

| Condição | Ação |
|---|---|
| Backup atrasado ≥ 3 dias, integridade falsa ou queda brusca de tamanho | `URGENTE [Técnico]: Falha grave/atraso de Backup…` |
| Queda de vendas ≥ 30% | `CRÍTICO [Comercial/CS]: Sinal forte de Churn…` |
| Chamados ≥ 5 e versão ≠ `v5.2` | `AÇÃO [Suporte N2]: Alto volume de chamados em versão defasada…` |
| Nenhuma das anteriores | `Cliente saudável. Manter monitoramento preventivo padrão.` |

## Dashboard

1. **Alertas em tempo real:** sino com contador, toast e banner vermelho quando um cliente é rebaixado para Alto Risco, mais um log de eventos ao vivo.
2. **Visão geral:** 4 KPIs (total, saudáveis, atenção, alto risco) e gráficos de distribuição de versões e saúde dos backups nas últimas 24h.
3. **Fila inteligente de priorização:** ordenada pelo menor Health Score, com versão, status do backup, variação de uso, chamados e score.
4. **Cliente 360:** painel lateral com o medidor do score, os 4 sinais detalhados, as ações recomendadas e os botões de ticket.
5. **Tickets:** "Assumir Atendimento / Abrir Ticket Preventivo" abre um ticket e mostra uma **ficha de atendimento com dados fictícios** (contato, plano, técnico etc.); "Cancelar Ticket" o encerra.

## Roteiro de demonstração

Com servidor e dashboard abertos, no simulador:

1. **`[1]` Heartbeat saudável:** a Padaria Estrela continua em 100.
2. **`[2]` Falha silenciosa de backup:** backup corrompido de 2MB com checksum inválido; o score cai para **65** (Atenção).
3. **`[3]` Crise completa:** v5.1, backup de 7 dias, vendas −40% e 7 chamados; o score cai para **11** (Alto Risco) e dispara o **alerta crítico** no dashboard.
4. **`[1]` de novo:** a Padaria volta a 100.
5. **`[4]` Modo caos:** telemetria aleatória de vários PDVs a cada 5 segundos para movimentar o dashboard (ENTER para parar).

No dashboard, abra o **Cliente 360** da Padaria, leia as recomendações e abra (e depois cancele) um ticket.

## API

| Método | Rota | Descrição |
|---|---|---|
| `POST` | `/api/v1/telemetry` | Recebe telemetria (parcial). Responde `old_score`, `new_score`, `status`, `critical_alert`, `recommended_actions`. |
| `GET` | `/api/v1/clients` | Resumo (KPIs) mais lista de clientes ordenada por prioridade. |
| `GET` | `/api/v1/clients/{id}` | Detalhe de um cliente. |
| `POST` | `/api/v1/clients/{id}/ticket` | Abre ticket (idempotente). |
| `DELETE` | `/api/v1/clients/{id}/ticket` | Cancela o ticket aberto. |
| `GET` | `/api/v1/events` | Stream SSE de eventos de telemetria. |
| `GET` | `/api/v1/events/recent` | Últimos 50 eventos. |

Exemplo de payload:
```json
{
  "client_id": "padaria-estrela",
  "version": "v5.2",
  "last_backup_date": "2026-10-22T10:00:00Z",
  "backup_size_mb": 450,
  "previous_backup_size_mb": 450,
  "backup_integrity_ok": true,
  "backup_checksum": "sha256:9f2c1a7be4d05a31",
  "sales_last_7d": 12500,
  "sales_avg_weekly": 10000,
  "tickets_last_30d": 1,
  "days_since_last_interaction": 35
}
```
Campos omitidos mantêm o valor anterior.

## Estrutura

```
cmd/server/            API, SQLite, seeder, hub SSE, ficha mockada e web/index.html
cmd/pdv-simulator/     CLI de simulação de PDV
internal/health/       Motor de Health Score e recomendações (com testes)
```

## Limitações conhecidas (MVP)

- **Sem autenticação:** não exponha o servidor na internet. Em produção use API key ou mTLS por PDV.
- Os eventos ao vivo ficam em memória e somem ao reiniciar. Clientes e tickets persistem no SQLite.
- Os dados da ficha de atendimento e do seeder são **fictícios**.
- `tickets_last_30d` vem da telemetria e não é ligado aos tickets abertos pelo painel.

## Equipe

Hackathon Fatec Ourinhos × BVX Tecnologia. Emanoel Benedito Monica e Gabriel Verri Sanches.
