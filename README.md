# FIAP X - Processing Service

Microsservico responsavel por processar videos no ecossistema **FIAP X**. Recebe solicitacoes de processamento, extrai frames dos videos e disponibiliza os resultados como arquivo ZIP.

---

## Como Funciona

1. O servico escuta a fila `video.processing` no RabbitMQ aguardando solicitacoes
2. Ao receber uma solicitacao, baixa o video do MinIO (bucket `uploads`)
3. Extrai 1 frame por segundo do video no formato PNG usando FFmpeg
4. Empacota todos os frames em um arquivo ZIP
5. Faz upload do ZIP para o MinIO (bucket `zips`)
6. Registra o job no PostgreSQL e publica o resultado na fila `video.status`
7. Limpa todos os arquivos temporarios do disco

---

## Regras de Funcionamento

### Retentativas

- Quando o processamento falha por um erro temporario (servico fora do ar, timeout, etc), o servico tenta novamente automaticamente
- Sao ate **7 tentativas** (configuravel), com espera crescente entre elas: 1s, 2s, 4s, 8s, 16s, 32s, 60s
- Se todas as tentativas falharem, a mensagem vai para a fila de erros (DLQ) e o usuario recebe um **email de notificacao** informando a falha

### Mensagens Invalidas

- Se a mensagem recebida estiver malformada (JSON invalido, campos faltando), ela vai direto para a DLQ sem tentativas de reprocessamento

### Processamento Paralelo

- O servico processa **3 videos simultaneamente** por padrao (configuravel via `WORKER_COUNT`)
- Todos os arquivos temporarios sao gravados em disco para manter o consumo de memoria baixo

### Desligamento

- Ao receber sinal de parada (SIGINT/SIGTERM), o servico para de aceitar novos videos
- Processamentos em andamento sao cancelados
- Na proxima inicializacao, os videos pendentes serao reprocessados do zero

---

## Estrutura do Projeto

O projeto segue **Clean Architecture** com separacao em camadas:

```
fiapx-processing-service/
├── cmd/worker/main.go              # Ponto de entrada - conecta todas as dependencias
│
├── internal/
│   ├── domain/                     # Regras de negocio (sem dependencias externas)
│   │   ├── entity/
│   │   │   ├── job.go              # Entidade Job (estados: PENDING > PROCESSING > COMPLETED/FAILED)
│   │   │   └── message.go          # Estrutura das mensagens de entrada e saida
│   │   └── port/                   # Contratos (interfaces) que a infra deve implementar
│   │
│   ├── usecase/
│   │   └── process_video.go        # Orquestra o pipeline completo de processamento
│   │
│   └── infra/                      # Implementacoes concretas
│       ├── config/                 # Carregamento de variaveis de ambiente
│       ├── postgres/               # Persistencia de jobs no banco
│       ├── rabbitmq/               # Consumo e publicacao de mensagens
│       ├── minio/                  # Download e upload de arquivos
│       ├── ffmpeg/                 # Extracao de frames e criacao de ZIP
│       ├── email/                  # Notificacao por email (MailHog)
│       ├── metrics/                # Metricas Prometheus e health check
│       └── tracing/                # Rastreamento distribuido (Jaeger)
│
├── migrations/                     # Scripts de criacao do banco de dados
├── pkg/logger/                     # Logger JSON estruturado
├── tests/integration/              # Testes de ponta a ponta
├── Dockerfile                      # Imagem Docker multi-stage
├── Makefile                        # Comandos uteis
├── .env.example                    # Variaveis de ambiente com valores padrao
└── go.mod                          # Dependencias Go
```

---

## Como Executar

### Pre-requisitos

- Docker
- Infraestrutura FIAP X rodando (`fiapx-infra-docker`)

### Com Docker

```bash
make docker-build
make docker-run
```

### Local (desenvolvimento)

Requer Go 1.22+ e FFmpeg instalados.

```bash
make build
make run
```

---

## Configuracao

Todas as configuracoes sao feitas via variaveis de ambiente. Veja [.env.example](.env.example) para a lista completa.

---

## Testes

```bash
make test               # Unitarios
make test-integration   # Ponta a ponta (requer Docker)
```

Os testes de integracao sobem PostgreSQL, RabbitMQ e MinIO automaticamente via containers temporarios, publicam uma mensagem na fila e validam que o ZIP foi gerado corretamente.
