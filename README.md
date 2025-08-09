# Rate Limiter - Documentação Técnica

Sistema de controle de tráfego que limita o número de requisições por IP ou token de acesso, implementado em Go com arquitetura limpa e estratégias de armazenamento plugáveis.

## 📖 Como Funciona

### Conceito Básico

O rate limiter controla o tráfego de requisições aplicando limites baseados em:
- **Endereço IP**: Limita requisições por IP de origem
- **Token de Acesso**: Limita requisições por token específico (via header `API_KEY`)

### Fluxo de Funcionamento

```
1. Requisição chega → Middleware extrai IP e Token
2. Service detecta tipo de limitação (Token tem prioridade sobre IP)
3. Storage verifica se chave está bloqueada
4. Se não bloqueada → incrementa contador
5. Se exceder limite → bloqueia por tempo configurável
6. Retorna resultado (permitido/bloqueado) com headers informativos
```

### Detecção Automática de Tipo

```go
// Prioridade de detecção:
if token_fornecido && token_não_vazio {
    return TokenLimiter  // Usa configuração do token
} else {
    return IPLimiter     // Usa configuração de IP
}
```

**Exemplo**: Se IP tem limite de 10 req/min e token tem 100 req/min, o sistema usará 100 req/min quando o token for fornecido.

### Sliding Window

O sistema usa **sliding window** para contagem de requisições:
- Janela de tempo configurável (ex: 60 segundos)
- Contador reseta automaticamente após a janela
- Bloqueio temporal quando limite excedido

## ⚙️ Configuração

### 1. Variáveis de Ambiente (.env)

```bash
# === RATE LIMITING ===
DEFAULT_IP_LIMIT=10        # Limite padrão por IP (req/min)
DEFAULT_TOKEN_LIMIT=100    # Limite padrão por token (req/min)
RATE_WINDOW=60            # Janela de tempo em segundos
BLOCK_DURATION=180        # Tempo de bloqueio em segundos (3min)

# === REDIS (Storage Principal) ===
REDIS_HOST=localhost      # Host do Redis
REDIS_PORT=6379          # Porta do Redis
REDIS_PASSWORD=          # Senha (opcional)
REDIS_DB=0              # Database (0-15)

# === STORAGE STRATEGY ===
STORAGE_TYPE=redis       # "redis" ou "memory"

# === SERVIDOR ===
SERVER_PORT=8080         # Porta da aplicação
GIN_MODE=debug          # "debug" ou "release"

# === LOGGING ===
LOG_LEVEL=info          # debug, info, warn, error
LOG_FORMAT=json         # json ou text

# === TOKENS CUSTOMIZADOS ===
TOKEN_CONFIG_FILE=internal/config/tokens.json
```

### 2. Configuração de Tokens Específicos

Arquivo: `internal/config/tokens.json`

```json
{
  "tokens": {
    "premium_token_abc123": {
      "token": "premium_token_abc123",
      "limit": 1000,
      "description": "Token premium com limite alto"
    },
    "basic_token_def456": {
      "token": "basic_token_def456", 
      "limit": 50,
      "description": "Token básico com limite baixo"
    },
    "enterprise_xyz789": {
      "token": "enterprise_xyz789",
      "limit": 5000,
      "description": "Token enterprise para clientes corporativos"
    }
  }
}
```

### 3. Estratégias de Storage

#### Redis (Recomendado para Produção)
- **Vantagens**: Persistente, distribuído, alta performance
- **Configuração**: Definir variáveis REDIS_* no .env
- **Fallback**: Se Redis falhar, usa Memory automaticamente

#### Memory (Desenvolvimento/Fallback)
- **Vantagens**: Sem dependências externas, setup zero
- **Limitações**: Dados perdidos ao reiniciar, não distribuído
- **Configuração**: `STORAGE_TYPE=memory`

## 🔧 Como Usar

### 1. Middleware Injetável

```go
// Setup no servidor Gin
router := gin.New()

// Middleware aplicado a rotas específicas
rateLimiterMiddleware := middleware.NewRateLimiterMiddleware(service, logger)

protected := router.Group("/api")
protected.Use(rateLimiterMiddleware)
{
    protected.GET("/users", getUsersHandler)
    protected.POST("/orders", createOrderHandler)
}
```

### 2. Headers de Requisição

```bash
# Limitação por IP (automática)
curl http://localhost:8080/api/users

# Limitação por Token (prioritária)
curl -H "API_KEY: premium_token_abc123" http://localhost:8080/api/users
```

### 3. Headers de Resposta

O sistema sempre retorna headers informativos:

```http
# Em requisições permitidas (HTTP 200)
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1640995200
X-RateLimit-Type: token

# Em requisições bloqueadas (HTTP 429)
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1640995200
X-RateLimit-Type: token
Retry-After: 180
```

### 4. Resposta HTTP 429

Quando o limite é excedido:

```json
{
  "error": "rate_limit_exceeded",
  "message": "you have reached the maximum number of requests or actions allowed within a certain time frame",
  "details": {
    "limit": 100,
    "remaining": 0,
    "reset_time": 1640995200,
    "limiter_type": "token",
    "blocked_until": 1640995380
  }
}
```

## 📊 Monitoramento e Administração

### 1. Health Check

```bash
curl http://localhost:8080/health
```

```json
{
  "status": "healthy",
  "service": "Rate Limiter API",
  "timestamp": "2025-01-01T15:30:00Z",
  "version": "1.0.0"
}
```

### 2. Métricas do Sistema

```bash
curl http://localhost:8080/metrics
```

```json
{
  "service": "Rate Limiter API",
  "uptime": "2h30m15s",
  "memory": {
    "alloc": "12.5 MB",
    "total_alloc": "45.2 MB",
    "sys": "25.1 MB"
  },
  "goroutines": 15,
  "rate_limiter": {
    "storage_type": "redis",
    "default_ip_limit": 10,
    "default_token_limit": 100
  }
}
```

### 3. Status de Rate Limiting

```bash
# Verificar status de um IP
curl "http://localhost:8080/admin/status?key=192.168.1.100&type=ip"

# Verificar status de um token
curl "http://localhost:8080/admin/status?key=premium_token_abc123&type=token"
```

```json
{
  "key": "192.168.1.100",
  "limiter_type": "ip",
  "limit": 10,
  "current": 7,
  "remaining": 3,
  "reset_time": "2025-01-01T15:31:00Z",
  "blocked": false,
  "blocked_until": null
}
```

### 4. Reset de Contadores

```bash
# Reset de IP
curl -X POST http://localhost:8080/admin/reset \
  -H "Content-Type: application/json" \
  -d '{"key": "192.168.1.100", "type": "ip"}'

# Reset de Token
curl -X POST http://localhost:8080/admin/reset \
  -H "Content-Type: application/json" \
  -d '{"key": "premium_token_abc123", "type": "token"}'
```

## 🏗️ Arquitetura Técnica

### Clean Architecture

```
┌─────────────────┐
│   Handlers      │ ← HTTP endpoints, validação de entrada
├─────────────────┤
│   Middleware    │ ← Extração IP/Token, orquestração
├─────────────────┤
│   Service       │ ← Lógica de negócio, detecção de tipo
├─────────────────┤
│   Storage       │ ← Persistência (Redis/Memory)
├─────────────────┤
│   Domain        │ ← Entidades, interfaces, regras
└─────────────────┘
```

### Strategy Pattern

```go
// Interface comum
type RateLimiterStorage interface {
    Increment(ctx, key string, limit int, window time.Duration) (int, time.Time, error)
    IsBlocked(ctx, key string) (bool, *time.Time, error)
    Block(ctx, key string, duration time.Duration) error
    // ... outros métodos
}

// Implementações intercambiáveis
type RedisStorage struct { ... }
type MemoryStorage struct { ... }

// Factory para criação
factory.CreateStorage(config, logger) // Retorna implementação baseada na config
```

### Separação de Responsabilidades

- **Middleware**: Extrai IP/Token, chama service, define headers HTTP
- **Service**: Contém toda lógica de rate limiting, detecção de tipo
- **Storage**: Operações de persistência, contadores, bloqueios
- **Config**: Carregamento de configurações (.env, tokens.json)

## 🧪 Exemplos Práticos

### Cenário 1: Limitação por IP

```bash
# Configuração: DEFAULT_IP_LIMIT=5, RATE_WINDOW=60

# Requisições 1-5: HTTP 200
for i in {1..5}; do
  curl http://localhost:8080/
done

# Requisição 6: HTTP 429 (bloqueada)
curl http://localhost:8080/
# Resposta: rate_limit_exceeded

# Após 60 segundos: Contador reseta
# Após BLOCK_DURATION: Desbloqueio automático
```

### Cenário 2: Token Sobrepõe IP

```bash
# IP 192.168.1.100 já atingiu limite (bloqueado)
curl http://localhost:8080/
# HTTP 429

# Mesmo IP com token válido: Permitido
curl -H "API_KEY: premium_token_abc123" http://localhost:8080/
# HTTP 200 (usa limite do token, não do IP)
```

### Cenário 3: Tokens Diferentes

```bash
# Token premium (limite 1000)
curl -H "API_KEY: premium_token_abc123" http://localhost:8080/
# HTTP 200

# Token básico (limite 50)
curl -H "API_KEY: basic_token_def456" http://localhost:8080/
# HTTP 200 (contador separado)

# Token desconhecido (usa DEFAULT_TOKEN_LIMIT=100)
curl -H "API_KEY: unknown_token" http://localhost:8080/
# HTTP 200
```

## 🚀 Instalação e Execução

```bash
# 1. Clone o repositório
git clone <repository-url>
cd desafio_tecnico01

# 2. Instale dependências
go mod tidy

# 3. Configure ambiente (opcional)
cp .env.example .env
nano .env

# 4. Inicie Redis (opcional)
docker-compose up -d redis

# 5. Execute aplicação
go run cmd/api/main.go

# 6. Teste funcionamento
curl http://localhost:8080/health
```

---

Este rate limiter implementa todas as funcionalidades necessárias para controle de tráfego em APIs de produção, com configuração flexível, monitoramento completo e arquitetura escalável.