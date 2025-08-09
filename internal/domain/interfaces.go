package domain

import (
	"context"
	"time"
)

// RateLimiterStorage define a interface para armazenamento do rate limiter
// Implementa o Strategy Pattern conforme requisito do fc_rate_limiter
type RateLimiterStorage interface {
	// Get recupera o status atual de rate limit para uma chave
	Get(ctx context.Context, key string) (*RateLimitStatus, error)
	
	// Set define o status de rate limit para uma chave
	Set(ctx context.Context, key string, status *RateLimitStatus, ttl time.Duration) error
	
	// Increment incrementa o contador para uma chave e retorna o novo valor
	Increment(ctx context.Context, key string, limit int, window time.Duration) (int, time.Time, error)
	
	// IsBlocked verifica se uma chave está bloqueada
	IsBlocked(ctx context.Context, key string) (bool, *time.Time, error)
	
	// Block bloqueia uma chave por um período específico
	Block(ctx context.Context, key string, duration time.Duration) error
	
	// Reset limpa os dados de uma chave
	Reset(ctx context.Context, key string) error
	
	// Health verifica se o storage está saudável
	Health(ctx context.Context) error
	
	// Close fecha a conexão com o storage
	Close() error
}

// RateLimiterService define a interface para o serviço de rate limiting
// Separação da lógica do middleware conforme requisito
type RateLimiterService interface {
	// CheckLimit verifica se uma requisição deve ser permitida
	CheckLimit(ctx context.Context, ip, token string) (*RateLimitResult, error)
	
	// IsAllowed verifica se uma chave específica está permitida
	IsAllowed(ctx context.Context, key string, limiterType LimiterType) (bool, error)
	
	// GetConfig retorna a configuração para uma chave específica
	GetConfig(key string, limiterType LimiterType) *RateLimitRule
	
	// GetStatus retorna o status atual de uma chave
	GetStatus(ctx context.Context, key string, limiterType LimiterType) (*RateLimitStatus, error)
	
	// Reset limpa os dados de rate limit para uma chave
	Reset(ctx context.Context, key string, limiterType LimiterType) error
}

// Logger define a interface para logging estruturado
type Logger interface {
	Debug(msg string, fields map[string]interface{})
	Info(msg string, fields map[string]interface{})
	Warn(msg string, fields map[string]interface{})
	Error(msg string, err error, fields map[string]interface{})
	WithContext(ctx context.Context) Logger
}

// ConfigLoader define a interface para carregamento de configurações
type ConfigLoader interface {
	LoadConfig() (*RateLimitConfig, error)
	LoadTokenConfigs() (map[string]TokenConfig, error)
	Reload() error
} 