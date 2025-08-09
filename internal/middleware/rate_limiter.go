package middleware

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"rate-limiter/internal/domain"
)

// RateLimiterMiddleware implementa o middleware de rate limiting
// Injetável no servidor web conforme requisito fc_rate_limiter
type RateLimiterMiddleware struct {
	service domain.RateLimiterService
	logger  domain.Logger
}

// NewRateLimiterMiddleware cria uma nova instância do middleware
func NewRateLimiterMiddleware(
	service domain.RateLimiterService,
	logger domain.Logger,
) gin.HandlerFunc {
	middleware := &RateLimiterMiddleware{
		service: service,
		logger:  logger,
	}
	
	return middleware.Handle
}

// Handle é o handler principal do middleware
func (m *RateLimiterMiddleware) Handle(c *gin.Context) {
	// Criar contexto com timeout para operações
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Gerar Request ID se não existir
	requestID := m.getRequestID(c)
	
	// Adicionar informações ao contexto
	ctx = m.enrichContext(ctx, c, requestID)
	
	// Obter logger com contexto
	logger := m.logger.WithContext(ctx)

	// Extrair IP e Token da requisição
	clientIP := m.extractClientIP(c)
	apiToken := m.extractAPIToken(c)

	logger.Debug("Rate limiter middleware initiated", map[string]interface{}{
		"client_ip":   clientIP,
		"api_token":   m.maskToken(apiToken),
		"user_agent":  c.GetHeader("User-Agent"),
		"method":      c.Request.Method,
		"path":        c.Request.URL.Path,
		"request_id":  requestID,
	})

	// Verificar rate limit usando o service
	result, err := m.service.CheckLimit(ctx, clientIP, apiToken)
	if err != nil {
		logger.Error("Rate limiter service error", err, map[string]interface{}{
			"client_ip":  clientIP,
			"api_token":  m.maskToken(apiToken),
			"request_id": requestID,
		})
		
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal server error",
			"message": "Unable to process rate limit check",
		})
		c.Abort()
		return
	}

	// Adicionar headers de rate limiting sempre
	m.setRateLimitHeaders(c, result)

	// Verificar se a requisição foi permitida
	if !result.Allowed {
		logger.Info("Request rate limited", map[string]interface{}{
			"client_ip":     clientIP,
			"api_token":     m.maskToken(apiToken),
			"limiter_type":  result.LimiterType,
			"limit":         result.Limit,
			"remaining":     result.Remaining,
			"blocked_until": result.BlockedUntil,
			"request_id":    requestID,
		})

		// Resposta HTTP 429 conforme fc_rate_limiter
		response := gin.H{
			"error":   "rate_limit_exceeded",
			"message": "you have reached the maximum number of requests or actions allowed within a certain time frame",
			"details": gin.H{
				"limit":       result.Limit,
				"remaining":   result.Remaining,
				"reset_time":  result.ResetTime.Unix(),
				"limiter_type": result.LimiterType,
			},
		}

		// Adicionar blocked_until se presente
		if result.BlockedUntil != nil {
			response["details"].(gin.H)["blocked_until"] = result.BlockedUntil.Unix()
		}

		c.JSON(http.StatusTooManyRequests, response)
		c.Abort()
		return
	}

	// Requisição permitida - continuar pipeline
	logger.Debug("Request allowed by rate limiter", map[string]interface{}{
		"client_ip":    clientIP,
		"api_token":    m.maskToken(apiToken),
		"limiter_type": result.LimiterType,
		"limit":        result.Limit,
		"remaining":    result.Remaining,
		"request_id":   requestID,
	})

	c.Next()
}

// extractClientIP extrai o IP do cliente considerando proxies e load balancers
func (m *RateLimiterMiddleware) extractClientIP(c *gin.Context) string {
	// Prioridade: X-Forwarded-For > X-Real-IP > RemoteAddr
	
	// X-Forwarded-For pode conter múltiplos IPs separados por vírgula
	// O primeiro é o IP original do cliente
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			clientIP := strings.TrimSpace(ips[0])
			if clientIP != "" {
				return clientIP
			}
		}
	}

	// X-Real-IP é usado por alguns proxies
	if xri := c.GetHeader("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fallback para RemoteAddr (remove porta se presente)
	if host, _, err := net.SplitHostPort(c.Request.RemoteAddr); err == nil {
		return host
	}

	// Se net.SplitHostPort falhar, retorna RemoteAddr como está
	return c.Request.RemoteAddr
}

// extractAPIToken extrai o token de API dos headers
func (m *RateLimiterMiddleware) extractAPIToken(c *gin.Context) string {
    // Prioridade: API_KEY (especificação) > X-Api-Token > Api-Token
    if token := c.GetHeader("API_KEY"); token != "" {
        return strings.TrimSpace(token)
    }

    if token := c.GetHeader("X-Api-Token"); token != "" {
        return strings.TrimSpace(token)
    }

    if token := c.GetHeader("Api-Token"); token != "" {
        return strings.TrimSpace(token)
    }

    return ""
}

// setRateLimitHeaders define headers informativos de rate limiting
func (m *RateLimiterMiddleware) setRateLimitHeaders(c *gin.Context, result *domain.RateLimitResult) {
	c.Header("X-RateLimit-Limit", strconv.Itoa(result.Limit))
	c.Header("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
	c.Header("X-RateLimit-Reset", strconv.FormatInt(result.ResetTime.Unix(), 10))
	c.Header("X-RateLimit-Type", string(result.LimiterType))

	// Adicionar Retry-After para requisições bloqueadas
	if !result.Allowed && result.BlockedUntil != nil {
		retryAfter := int(time.Until(*result.BlockedUntil).Seconds())
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
	}
}

// getRequestID obtém ou gera um Request ID para tracking
func (m *RateLimiterMiddleware) getRequestID(c *gin.Context) string {
	// Verifica se já existe no header
	if requestID := c.GetHeader("X-Request-ID"); requestID != "" {
		return requestID
	}

	// Gera novo UUID
	requestID := uuid.New().String()
	c.Header("X-Request-ID", requestID)
	return requestID
}

// enrichContext adiciona informações relevantes ao contexto
func (m *RateLimiterMiddleware) enrichContext(ctx context.Context, c *gin.Context, requestID string) context.Context {
	// Adiciona informações da requisição ao contexto para logging
	type contextKey string
	
	const (
		requestIDKey contextKey = "request_id"
		clientIPKey  contextKey = "client_ip"
		userAgentKey contextKey = "user_agent"
		methodKey    contextKey = "method"
		pathKey      contextKey = "path"
	)

	ctx = context.WithValue(ctx, requestIDKey, requestID)
	ctx = context.WithValue(ctx, clientIPKey, m.extractClientIP(c))
	ctx = context.WithValue(ctx, userAgentKey, c.GetHeader("User-Agent"))
	ctx = context.WithValue(ctx, methodKey, c.Request.Method)
	ctx = context.WithValue(ctx, pathKey, c.Request.URL.Path)

	return ctx
}

// maskToken mascara o token para logs de segurança
func (m *RateLimiterMiddleware) maskToken(token string) string {
	if token == "" {
		return ""
	}
	
	if len(token) <= 8 {
		return token + "***"
	}
	
	return token[:8] + "***"
}

// GetClientIP é uma função utilitária exportada para uso externo
func GetClientIP(c *gin.Context) string {
	middleware := &RateLimiterMiddleware{}
	return middleware.extractClientIP(c)
}

// GetAPIToken é uma função utilitária exportada para uso externo
func GetAPIToken(c *gin.Context) string {
	middleware := &RateLimiterMiddleware{}
	return middleware.extractAPIToken(c)
} 