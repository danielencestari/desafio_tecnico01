package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"rate-limiter/internal/domain"
)

// RateLimiterService implementa a lógica de negócio do rate limiting
// Separada do middleware conforme requisito fc_rate_limiter
type RateLimiterService struct {
	storage domain.RateLimiterStorage
	config  *domain.RateLimitConfig
	logger  domain.Logger
}

// NewRateLimiterService cria uma nova instância do serviço
func NewRateLimiterService(
	storage domain.RateLimiterStorage,
	config *domain.RateLimitConfig,
	logger domain.Logger,
) domain.RateLimiterService {
	return &RateLimiterService{
		storage: storage,
		config:  config,
		logger:  logger,
	}
}

// CheckLimit implementa a lógica principal de verificação de rate limit
// Detecta automaticamente se deve limitar por IP ou Token
func (s *RateLimiterService) CheckLimit(ctx context.Context, ip, token string) (*domain.RateLimitResult, error) {
	// Detecta o tipo de limitação automaticamente
	limiterType, key := s.detectLimiterType(ip, token)
	
	s.logger.Debug("Rate limit check initiated", map[string]interface{}{
		"ip":           ip,
		"token":        s.maskToken(token),
		"limiter_type": limiterType,
		"key":          key,
	})

	// Monta a chave de storage
	storageKey := s.buildStorageKey(key, limiterType)

	// Verifica se a chave está bloqueada
	isBlocked, blockedUntil, err := s.storage.IsBlocked(ctx, storageKey)
	if err != nil {
		s.logger.Error("Failed to check blocked status", err, map[string]interface{}{
			"storage_key": storageKey,
		})
		return nil, fmt.Errorf("failed to check blocked status: %w", err)
	}

	// Se está bloqueada, retorna negação
	if isBlocked {
		s.logger.Info("Request blocked", map[string]interface{}{
			"storage_key":   storageKey,
			"blocked_until": blockedUntil,
		})

		rule := s.GetConfig(key, limiterType)
		return &domain.RateLimitResult{
			Allowed:      false,
			Limit:        rule.Limit,
			Remaining:    0,
			ResetTime:    time.Now().Add(time.Duration(rule.Window) * time.Second),
			BlockedUntil: blockedUntil,
			LimiterType:  limiterType,
		}, nil
	}

	// Obtém a configuração para a chave
	rule := s.GetConfig(key, limiterType)

	// Incrementa o contador e verifica limite
	currentCount, resetTime, err := s.storage.Increment(
		ctx,
		storageKey,
		rule.Limit,
		time.Duration(rule.Window)*time.Second,
	)
	if err != nil {
		s.logger.Error("Failed to increment counter", err, map[string]interface{}{
			"storage_key": storageKey,
			"limit":       rule.Limit,
		})
		return nil, fmt.Errorf("failed to increment counter: %w", err)
	}

    // Calcula remaining
    remaining := rule.Limit - currentCount
    if remaining < 0 {
        remaining = 0
    }

    // Verifica se excedeu o limite
    // Importante: permitir até o limite inclusivo (ex.: 10ª requisição ainda é permitida)
    allowed := currentCount <= rule.Limit
	
	// Se excedeu o limite, bloqueia por X minutos
	if !allowed {
		blockDuration := time.Duration(rule.BlockDuration) * time.Second
		if err := s.storage.Block(ctx, storageKey, blockDuration); err != nil {
			s.logger.Error("Failed to block key", err, map[string]interface{}{
				"storage_key":    storageKey,
				"block_duration": blockDuration,
			})
			// Não retorna erro aqui para não impedir a resposta HTTP 429
		}

		blockTime := time.Now().Add(blockDuration)
		s.logger.Info("Rate limit exceeded, key blocked", map[string]interface{}{
			"storage_key":    storageKey,
			"current_count":  currentCount,
			"limit":          rule.Limit,
			"blocked_until":  blockTime,
		})

		return &domain.RateLimitResult{
			Allowed:      false,
			Limit:        rule.Limit,
			Remaining:    0,
			ResetTime:    resetTime,
			BlockedUntil: &blockTime,
			LimiterType:  limiterType,
		}, nil
	}

	// Requisição permitida
	s.logger.Debug("Request allowed", map[string]interface{}{
		"storage_key":   storageKey,
		"current_count": currentCount,
		"limit":         rule.Limit,
		"remaining":     remaining,
	})

	return &domain.RateLimitResult{
		Allowed:     true,
		Limit:       rule.Limit,
		Remaining:   remaining,
		ResetTime:   resetTime,
		LimiterType: limiterType,
	}, nil
}

// IsAllowed verifica se uma chave específica está permitida (não bloqueada)
func (s *RateLimiterService) IsAllowed(ctx context.Context, key string, limiterType domain.LimiterType) (bool, error) {
	storageKey := s.buildStorageKey(key, limiterType)
	
	isBlocked, _, err := s.storage.IsBlocked(ctx, storageKey)
	if err != nil {
		return false, fmt.Errorf("failed to check if key is allowed: %w", err)
	}
	
	return !isBlocked, nil
}

// GetConfig retorna a configuração apropriada para uma chave
func (s *RateLimiterService) GetConfig(key string, limiterType domain.LimiterType) *domain.RateLimitRule {
	var limit int
	var description string

	switch limiterType {
	case domain.IPLimiter:
		limit = s.config.DefaultIPLimit
		description = fmt.Sprintf("Default IP limit for %s", key)

	case domain.TokenLimiter:
		// Verifica se há configuração específica para o token
		if tokenConfig, exists := s.config.TokenConfigs[key]; exists {
			limit = tokenConfig.Limit
			description = tokenConfig.Description
		} else {
			// Usa limite padrão para tokens
			limit = s.config.DefaultTokenLimit
			description = fmt.Sprintf("Default token limit for %s", key)
		}

	default:
		// Fallback para IP se tipo desconhecido
		limit = s.config.DefaultIPLimit
		description = fmt.Sprintf("Fallback IP limit for %s", key)
	}

	return &domain.RateLimitRule{
		ID:            fmt.Sprintf("%s:%s", limiterType, key),
		Type:          limiterType,
		Key:           key,
		Limit:         limit,
		Window:        s.config.Window,
		BlockDuration: s.config.BlockDuration,
		Description:   description,
	}
}

// GetStatus retorna o status atual de uma chave
func (s *RateLimiterService) GetStatus(ctx context.Context, key string, limiterType domain.LimiterType) (*domain.RateLimitStatus, error) {
	storageKey := s.buildStorageKey(key, limiterType)
	
	status, err := s.storage.Get(ctx, storageKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}
    
    // Enriquecer o status com o tipo de limiter solicitado
    if status != nil {
        status.Type = limiterType
    }
    
    return status, nil
}

// Reset limpa os dados de rate limit para uma chave
func (s *RateLimiterService) Reset(ctx context.Context, key string, limiterType domain.LimiterType) error {
	storageKey := s.buildStorageKey(key, limiterType)
	
	if err := s.storage.Reset(ctx, storageKey); err != nil {
		return fmt.Errorf("failed to reset key: %w", err)
	}
	
	s.logger.Info("Rate limit reset", map[string]interface{}{
		"key":          key,
		"limiter_type": limiterType,
		"storage_key":  storageKey,
	})
	
	return nil
}

// detectLimiterType detecta automaticamente o tipo baseado nos parâmetros
// Prioriza token se fornecido, senão usa IP
func (s *RateLimiterService) detectLimiterType(ip, token string) (domain.LimiterType, string) {
	// Remove espaços em branco do token
	token = strings.TrimSpace(token)
	
	// Se token foi fornecido e não está vazio, usa limitação por token
	if token != "" {
		return domain.TokenLimiter, token
	}
	
	// Senão, usa limitação por IP
	return domain.IPLimiter, ip
}

// buildStorageKey constrói a chave de storage no formato padrão
func (s *RateLimiterService) buildStorageKey(key string, limiterType domain.LimiterType) string {
	return fmt.Sprintf("rate_limit:%s:%s", limiterType, key)
}

// maskToken mascara o token para logs de segurança
func (s *RateLimiterService) maskToken(token string) string {
	if token == "" {
		return ""
	}
	
	if len(token) <= 8 {
		return token + "***"
	}
	
	return token[:8] + "***"
} 