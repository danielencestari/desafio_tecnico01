# Rate Limiter - Notas de Arquitetura

## 🏗️ Arquitetura Geral
**Padrão:** Clean Architecture  
**Linguagem:** Go 1.21+  
**Framework Web:** Gin  
**Storage:** Redis (com Strategy Pattern)  
**Containerização:** Docker + Docker Compose

## 📁 Estrutura do Projeto

```
rate-limiter/
├── cmd/api/
│   └── main.go                    # Entry point da aplicação
├── internal/
│   ├── config/
│   │   ├── config.go             # Configurações da aplicação
│   │   └── tokens.json           # Configurações específicas de tokens
│   ├── domain/
│   │   ├── entities.go           # Entidades do domínio
│   │   └── interfaces.go         # Interfaces/contratos
│   ├── service/
│   │   └── rate_limiter.go       # Lógica de negócio do rate limiter
│   ├── storage/
│   │   ├── redis.go              # Implementação Redis
│   │   └── memory.go             # Implementação em memória (para testes)
│   ├── middleware/
│   │   └── rate_limiter.go       # Middleware do Gin
│   ├── handler/
│   │   └── test_handler.go       # Endpoints de teste
│   └── logger/
│       └── logger.go             # Sistema de logs
├── docker-compose.yml
├── Dockerfile
├── .env
├── go.mod
├── go.sum
└── README.md
```

## 🔧 Especificações Técnicas

### Rate Limiting
- **Janela de Contagem:** 60 segundos
- **Tempo de Bloqueio:** 3 minutos (180s)
- **Algoritmo:** Sliding Window Counter
- **Prioridade:** Token > IP

### Configurações
- **Variáveis de Ambiente:** `.env`
- **Configuração de Tokens:** `tokens.json`
- **Limites Padrão:** 
  - IP: 10 req/60s
  - Token: 100 req/60s

### Storage Strategy
- **Interface:** `RateLimiterStorage`
- **Implementação Primária:** Redis
- **Implementação Teste:** Memory
- **Chaves Redis:** 
  - `rate_limit:ip:{ip}`
  - `rate_limit:token:{token}`

### Logging
- **Formato:** JSON estruturado
- **Níveis:** DEBUG, INFO, WARN, ERROR
- **Contexto:** Request ID, IP, Token, Ação

## 🎯 Fluxo de Requisição

1. **Middleware:** Captura IP e API_KEY
2. **Service:** Verifica limites (Token > IP)
3. **Storage:** Consulta/atualiza contadores
4. **Response:** 200 (OK) ou 429 (Rate Limited)
5. **Logging:** Registra ação tomada

## 📊 Monitoramento

### Métricas Importantes
- Requisições por segundo
- Taxa de bloqueio por IP/Token
- Latência do rate limiter
- Conexões Redis

### Headers de Resposta
- `X-RateLimit-Limit`: Limite configurado
- `X-RateLimit-Remaining`: Requisições restantes
- `X-RateLimit-Reset`: Timestamp do reset

## 🔒 Considerações de Segurança
- Validação de IP real (X-Forwarded-For)
- Sanitização de tokens
- Rate limiting também para endpoints de configuração
- Logs não devem expor tokens completos 