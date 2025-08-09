package domain

import "time"

// LimiterType define os tipos de rate limiting disponíveis
type LimiterType string

const (
	IPLimiter    LimiterType = "ip"
	TokenLimiter LimiterType = "token"
)

// RateLimitRule define as regras de rate limiting
type RateLimitRule struct {
	ID            string      `json:"id"`
	Type          LimiterType `json:"type"`
	Key           string      `json:"key"` // IP ou Token
	Limit         int         `json:"limit"`
	Window        int         `json:"window"`        // Janela em segundos
	BlockDuration int         `json:"blockDuration"` // Duração do bloqueio em segundos
	Description   string      `json:"description"`
}

// RateLimitStatus representa o status atual de um rate limit
type RateLimitStatus struct {
	Key         string    `json:"key"`
	Type        LimiterType `json:"type"`
	Count       int       `json:"count"`
	Limit       int       `json:"limit"`
	Window      int       `json:"window"`
	LastReset   time.Time `json:"lastReset"`
	BlockedUntil *time.Time `json:"blockedUntil,omitempty"`
	IsBlocked   bool      `json:"isBlocked"`
}

// RateLimitResult representa o resultado de uma verificação de rate limit
type RateLimitResult struct {
	Allowed      bool          `json:"allowed"`
	Limit        int           `json:"limit"`
	Remaining    int           `json:"remaining"`
	ResetTime    time.Time     `json:"resetTime"`
	BlockedUntil *time.Time    `json:"blockedUntil,omitempty"`
	LimiterType  LimiterType   `json:"limiterType"`
}

// TokenConfig representa a configuração de um token específico
type TokenConfig struct {
	Token       string `json:"token"`
	Limit       int    `json:"limit"`
	Description string `json:"description"`
}

// RateLimitConfig representa todas as configurações do rate limiter
type RateLimitConfig struct {
	DefaultIPLimit    int                    `json:"defaultIpLimit"`
	DefaultTokenLimit int                    `json:"defaultTokenLimit"`
	Window           int                    `json:"window"`
	BlockDuration    int                    `json:"blockDuration"`
	TokenConfigs     map[string]TokenConfig `json:"tokenConfigs"`
} 