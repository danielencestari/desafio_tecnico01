# 📂 FUNÇÃO DOS ARQUIVOS - RATE LIMITER

## 🏗️ ESTRUTURA DO PROJETO

### 📁 Raiz do Projeto
```
rate-limiter/
├── go.mod                    # Dependências Go
├── go.sum                    # Lock de versões
├── .env                      # Configurações ambiente
├── tokens.json               # Configurações tokens
├── .gitignore               # Exclusões Git
├── Dockerfile               # Build container
├── docker-compose.yml       # Orquestração Docker
├── cmd/                     # Entry points
├── internal/                # Código interno
└── step-by-step/           # Documentação progresso
```

---

## 📁 CMD - ENTRY POINTS

### `cmd/api/main.go`
**Status:** 🔄 Pendente (Fase 8)
- **Função:** Entry point da aplicação
- **Responsabilidades:**
  - Inicialização da aplicação
  - Setup de dependências
  - Configuração do servidor Gin
  - Graceful shutdown
  - Health checks

---

## 📁 INTERNAL - CÓDIGO INTERNO

### 🎯 Domain Layer (Fase 2) ✅

#### `internal/domain/entities.go`
**Status:** ✅ Implementado
- **Função:** Entidades core do domínio
- **Responsabilidades:**
  - `LimiterType` (IP/Token)
  - `RateLimitRule` (regras de limitação)
  - `RateLimitStatus` (estado atual)
  - `RateLimitResult` (resultado da verificação)
  - `TokenConfig` (configuração por token)
  - `RateLimitConfig` (configuração geral)

#### `internal/domain/interfaces.go`
**Status:** ✅ Implementado
- **Função:** Contratos e interfaces do sistema
- **Responsabilidades:**
  - `RateLimiterStorage` (Strategy Pattern)
  - `RateLimiterService` (lógica de negócio)
  - `Logger` (sistema de logs)
  - `ConfigLoader` (configurações)

---

### ⚙️ Infrastructure Layer

#### `internal/config/config.go` (Fase 3) ✅
**Status:** ✅ Implementado  
- **Função:** Carregamento e validação de configurações
- **Responsabilidades:**
  - Implementa `domain.ConfigLoader`
  - Carrega `.env` com godotenv
  - Parse `tokens.json` com validação
  - Validação robusta de parâmetros
  - Valores padrão para configurações
  - Tratamento graceful de arquivos ausentes

#### `internal/config/config_test.go` (Fase 3) ✅
**Status:** ✅ Implementado
- **Função:** Testes do sistema de configurações
- **Cobertura:** 11 testes abrangentes
- **Cenários:** Carregamento válido, validações, defaults, JSON malformado

#### `internal/logger/logger.go` (Fase 4) ✅
**Status:** ✅ Implementado
- **Função:** Sistema de logging estruturado
- **Responsabilidades:**
  - Implementa `domain.Logger`
  - Integração com logrus
  - Context com request info
  - Token masking para segurança
  - Métodos específicos: RateLimit, Config, Storage events
  - Campos estruturados (component, version, timestamp)

#### `internal/logger/logger_test.go` (Fase 4) ✅
**Status:** ✅ Implementado
- **Função:** Testes do sistema de logging
- **Cobertura:** 9 testes abrangentes
- **Cenários:** Formatos JSON/text, masking, context, campos estruturados

---

### 💾 Storage Layer (Fase 5) ✅

#### `internal/storage/redis.go` ✅
**Status:** ✅ Implementado
- **Função:** Implementação Redis do RateLimiterStorage
- **Responsabilidades:**
  - Strategy Pattern para Redis
  - Script Lua para operações atômicas
  - Pool de conexões otimizado (20 conn, 5 idle min)
  - Health check e graceful shutdown
  - Logging detalhado com latência
  - TTL automático e bloqueios temporários
  - Chaves padronizadas (`rate_limit:ip:X`, `rate_limit:token:X`)

#### `internal/storage/memory.go` ✅
**Status:** ✅ Implementado
- **Função:** Implementação Memory do RateLimiterStorage
- **Responsabilidades:**
  - Strategy Pattern para Memory
  - Thread-safe com `sync.RWMutex`
  - Cleanup automático (goroutine background)
  - TTL com goroutines independentes
  - Estatísticas de uso (`GetStats()`)
  - Operações atômicas de increment
  - Bloqueios específicos com expiração

#### `internal/storage/factory.go` ✅
**Status:** ✅ Implementado
- **Função:** Factory para Strategy Pattern
- **Responsabilidades:**
  - `StorageFactory` criação de storages
  - Suporte Redis e Memory
  - Configuração via environment variables
  - Validação robusta de configurações
  - Funções helper para defaults
  - `BuildStorageConfigFromEnv()` integração

#### `internal/storage/memory_test.go` ✅
**Status:** ✅ Implementado
- **Função:** Testes Memory Storage
- **Cobertura:** 16 testes abrangentes
- **Cenários:** CRUD, increment, bloqueios, TTL, cleanup, concorrência, health, stats

#### `internal/storage/factory_test.go` ✅
**Status:** ✅ Implementado
- **Função:** Testes Factory Pattern
- **Cobertura:** 8 testes + integração
- **Cenários:** Criação Redis/Memory, validação configs, environment, integração E2E

#### `internal/storage/redis_test.go.bak` ⚠️
**Status:** ⚠️ Temporariamente removido
- **Função:** Testes Redis Storage (com problemas de mock)
- **Motivo:** Mock incompleto da interface `redis.Cmdable`
- **Solução:** Implementar mock completo ou usar testcontainers

---

### 🎯 Service Layer (Fase 6) 🔄

#### `internal/service/rate_limiter.go`
**Status:** 🔄 Próxima fase
- **Função:** Lógica de negócio do rate limiting
- **Responsabilidades:**
  - Implementa `domain.RateLimiterService`
  - Lógica de contagem de requisições
  - Detecção automática IP vs Token
  - Aplicação de regras de rate limiting
  - Bloqueio automático após exceder limite
  - Reset manual de contadores
  - Métricas e monitoramento

#### `internal/service/rate_limiter_test.go`
**Status:** 🔄 Próxima fase
- **Função:** Testes da lógica de negócio
- **Cobertura planejada:** ~15 testes
- **Cenários:** IP/Token detection, rate limiting rules, blocking, reset, edge cases

---

### 🌐 Presentation Layer (Fases 7-8) 🔄

#### `internal/middleware/rate_limiter.go`
**Status:** 🔄 Pendente (Fase 7)
- **Função:** Middleware Gin injetável
- **Responsabilidades:**
  - Extração de IP e Token do request
  - Integração com RateLimiterService
  - Resposta HTTP 429 padronizada
  - Headers informativos (X-RateLimit-*)
  - Context enriquecido para logging

#### `internal/middleware/rate_limiter_test.go`
**Status:** 🔄 Pendente (Fase 7)
- **Função:** Testes do middleware
- **Cenários:** IP extraction, token parsing, HTTP responses, headers

#### `internal/handler/health.go`
**Status:** 🔄 Pendente (Fase 8)
- **Função:** Health check endpoints
- **Responsabilidades:**
  - `/health` - status da aplicação
  - `/health/storage` - status do storage
  - `/metrics` - métricas básicas

#### `internal/handler/admin.go`
**Status:** 🔄 Pendente (Fase 8)
- **Função:** Endpoints administrativos
- **Responsabilidades:**
  - `/admin/reset/{ip|token}` - reset manual
  - `/admin/status/{ip|token}` - status atual
  - `/admin/metrics` - métricas detalhadas

---

## 📚 STEP-BY-STEP - DOCUMENTAÇÃO

### `step-by-step/progress-log.md` ✅
**Status:** ✅ Atualizado
- **Função:** Log de progresso geral
- **Conteúdo:** Status das fases, checkboxes, próximos passos

### `step-by-step/file-functions.md` ✅
**Status:** ✅ Atualizado
- **Função:** Documenta função de cada arquivo
- **Conteúdo:** Responsabilidades, status, dependências

### `step-by-step/changes-log.md` ✅
**Status:** ✅ Atualizado
- **Função:** Log detalhado de alterações
- **Conteúdo:** Mudanças por fase, arquivos implementados, métricas

### `step-by-step/architecture-notes.md` ✅
**Status:** ✅ Atualizado
- **Função:** Notas sobre arquitetura
- **Conteúdo:** Decisões arquiteturais, patterns, trade-offs

### `step-by-step/next-steps.md`
**Status:** 🔄 Pendente
- **Função:** Próximos passos detalhados
- **Conteúdo:** Roadmap Fase 6+, dependências, prioridades

---

## 🐳 INFRAESTRUTURA E DEPLOYMENT

### `Dockerfile` ✅
**Status:** ✅ Implementado
- **Função:** Build multi-stage otimizado
- **Características:**
  - Stage 1: Build com Go 1.21-alpine
  - Stage 2: Runtime mínimo com ca-certificates
  - Imagem final ~15MB
  - Otimizado para produção

### `docker-compose.yml` ✅
**Status:** ✅ Implementado
- **Função:** Orquestração local
- **Serviços:**
  - Redis 7-alpine (porta 6379)
  - Rate Limiter (porta 8080)
  - Network isolada
  - Volume para persistência Redis

### `.env` ✅
**Status:** ✅ Implementado
- **Função:** Configurações padrão
- **Parâmetros:**
  - Rate limits: IP (10/60s), Token (100/60s)
  - Redis: localhost:6379, DB 0
  - Server: porta 8080
  - Logs: info level, text format
  - Timing: 60s window, 3min block

### `tokens.json` ✅
**Status:** ✅ Implementado
- **Função:** Configuração específica de tokens
- **Formato:**
  ```json
  {
    "token_id": {"limit": 1000, "window": 3600}
  }
  ```

---

## 📈 DEPENDÊNCIAS ENTRE ARQUIVOS

### Fase 5 (Storage Layer) - Dependências Satisfeitas:
- ✅ `domain.RateLimiterStorage` interface
- ✅ `domain.Logger` interface  
- ✅ Strategy Pattern implementado
- ✅ Configuração via environment
- ✅ Testes passando (24/24)

### Próxima Fase 6 (Service Layer) - Ready:
- ✅ Storage abstraction pronta
- ✅ Factory pattern funcional
- ✅ Logging estruturado
- ✅ Configuration loading
- ✅ Base sólida para lógica de negócio

### Arquitetura Limpa Mantida:
```
Domain (entities, interfaces) 
    ↑
Service (business logic) 
    ↑  
Storage (infrastructure) + Config + Logger
    ↑
Middleware + Handlers (presentation)
    ↑
Main (entry point)
``` 

## 📁 MIDDLEWARE LAYER (Fase 7)

### `internal/middleware/rate_limiter.go`
**Finalidade:** Middleware Gin para rate limiting HTTP
**Funcionalidades:**
- Middleware injetável `gin.HandlerFunc`
- Extração robusta de IP (X-Forwarded-For > X-Real-IP > RemoteAddr)
- Extração de token API (X-Api-Token > Api-Token)
- Integração com `RateLimiterService`
- Headers informativos (X-RateLimit-*)
- Resposta HTTP 429 conforme fc_rate_limiter
- Context propagation com Request ID
- Token masking para segurança
- Error handling robusto

**Principais Componentes:**
```go
type RateLimiterMiddleware struct {
    service domain.RateLimiterService
    logger  domain.Logger
}

func NewRateLimiterMiddleware(service, logger) gin.HandlerFunc
func (m *RateLimiterMiddleware) Handle(c *gin.Context)
func (m *RateLimiterMiddleware) extractClientIP(c *gin.Context) string
func (m *RateLimiterMiddleware) extractAPIToken(c *gin.Context) string
func (m *RateLimiterMiddleware) setRateLimitHeaders(c *gin.Context, result *domain.RateLimitResult)

// Funções utilitárias exportadas
func GetClientIP(c *gin.Context) string
func GetAPIToken(c *gin.Context) string
```

### `internal/middleware/rate_limiter_test.go`
**Finalidade:** Testes do middleware rate limiter
**Cobertura:** 6 testes focados
- Request permitida com headers corretos
- Request bloqueada com HTTP 429
- Extração de IP (3 cenários)
- Extração de token (3 cenários)  
- Tratamento de erros do service

**Mocks Utilizados:**
- `MockRateLimiterService`: Mock do service layer
- `MockLogger`: Mock do sistema de logging
- Router Gin de teste com middleware

--- 