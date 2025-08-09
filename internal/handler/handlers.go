package handler

import (
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"rate-limiter/internal/domain"
	"rate-limiter/internal/middleware"
)

// Handlers contém os handlers da API
type Handlers struct {
	service   domain.RateLimiterService
	logger    domain.Logger
	startTime time.Time
}

// NewHandlers cria uma nova instância dos handlers
func NewHandlers(service domain.RateLimiterService, logger domain.Logger) *Handlers {
	return &Handlers{
		service:   service,
		logger:    logger,
		startTime: time.Now(),
	}
}

// SetupRoutes configura as rotas da API
func (h *Handlers) SetupRoutes(router *gin.Engine) {
	// Middleware de rate limiting para rotas protegidas
	rateLimiterMiddleware := middleware.NewRateLimiterMiddleware(h.service, h.logger)

	// Rotas públicas (sem rate limiting)
	router.GET("/health", h.HealthHandler)
	router.GET("/metrics", h.MetricsHandler)

	// Rotas protegidas por rate limiting
	protected := router.Group("/")
	protected.Use(rateLimiterMiddleware)
	{
		protected.GET("/", h.ExampleHandler)
	}

	// Rotas administrativas (sem rate limiting)
	admin := router.Group("/admin")
	{
		admin.GET("/status", h.AdminStatusHandler)
		admin.POST("/reset", h.AdminResetHandler)
	}
}

// HealthHandler implementa health check básico
func (h *Handlers) HealthHandler(c *gin.Context) {
	response := gin.H{
		"status":    "healthy",
		"service":   "Rate Limiter API",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   "1.0.0",
	}

	c.JSON(http.StatusOK, response)
}

// ExampleHandler implementa um endpoint de exemplo protegido por rate limiting
func (h *Handlers) ExampleHandler(c *gin.Context) {
	ctx := c.Request.Context()
	logger := h.logger.WithContext(ctx)

	// Obter informações da requisição
	clientIP := middleware.GetClientIP(c)
	apiToken := middleware.GetAPIToken(c)

	logger.Debug("Example endpoint accessed", map[string]interface{}{
		"client_ip": clientIP,
		"api_token": h.maskToken(apiToken),
		"path":      c.Request.URL.Path,
	})

	response := gin.H{
		"message":   "Hello from Rate Limiter API!",
		"service":   "Rate Limiter API",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"client_ip": clientIP,
		"path":      c.Request.URL.Path,
		"method":    c.Request.Method,
	}

	// Adicionar informações do token se presente
	if apiToken != "" {
		response["api_token"] = h.maskToken(apiToken)
	}

	c.JSON(http.StatusOK, response)
}

// MetricsHandler implementa endpoint de métricas do sistema
func (h *Handlers) MetricsHandler(c *gin.Context) {
	ctx := c.Request.Context()
	logger := h.logger.WithContext(ctx)

	logger.Debug("Metrics endpoint accessed", map[string]interface{}{
		"client_ip": middleware.GetClientIP(c),
		"path":      c.Request.URL.Path,
	})

	// Calcular uptime
	uptime := time.Since(h.startTime)

	// Obter estatísticas do sistema
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	response := gin.H{
		"service":   "Rate Limiter API",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"uptime":    uptime.String(),
		"uptime_seconds": int64(uptime.Seconds()),
		"system": gin.H{
			"go_version":     runtime.Version(),
			"goroutines":     runtime.NumGoroutine(),
			"memory_alloc":   formatBytes(m.Alloc),
			"memory_total":   formatBytes(m.TotalAlloc),
			"memory_sys":     formatBytes(m.Sys),
			"gc_runs":        m.NumGC,
		},
	}

	c.JSON(http.StatusOK, response)
}

// AdminStatusHandler implementa endpoint de status administrativo
func (h *Handlers) AdminStatusHandler(c *gin.Context) {
	ctx := c.Request.Context()
	
	// Extrair parâmetros da query
	key := strings.TrimSpace(c.Query("key"))
	typeParam := strings.TrimSpace(c.Query("type"))

	// Log apenas se logger estiver configurado
	if h.logger != nil {
		logger := h.logger.WithContext(ctx)
		logger.Debug("Admin status endpoint accessed", map[string]interface{}{
			"key":  h.maskToken(key),
			"type": typeParam,
		})
	}

	// Validação de parâmetros
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": "key parameter is required",
		})
		return
	}

	if typeParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": "type parameter is required",
		})
		return
	}

	// Validar tipo de limiter
	var limiterType domain.LimiterType
	switch strings.ToLower(typeParam) {
	case "ip":
		limiterType = domain.IPLimiter
	case "token":
		limiterType = domain.TokenLimiter
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": "type must be 'ip' or 'token'",
		})
		return
	}

	// Obter status do rate limiter
	status, err := h.service.GetStatus(ctx, key, limiterType)
	if err != nil {
		if h.logger != nil {
			logger := h.logger.WithContext(ctx)
			logger.Error("Failed to get rate limiter status", err, map[string]interface{}{
				"key":  h.maskToken(key),
				"type": typeParam,
			})
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_server_error",
			"message": "Failed to retrieve rate limiter status",
		})
		return
	}

	// Preparar resposta
	response := gin.H{
		"key":          status.Key,
		"limit":        status.Limit,
		"current":      status.Count,
		"remaining":    max(0, status.Limit-status.Count),
		"reset_time":   status.LastReset.Add(time.Duration(status.Window)*time.Second).Unix(),
		"is_blocked":   status.IsBlocked,
		"limiter_type": string(status.Type),
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
	}

	// Adicionar blocked_until se presente
	if status.BlockedUntil != nil {
		response["blocked_until"] = status.BlockedUntil.Unix()
	}

	c.JSON(http.StatusOK, response)
}

// AdminResetRequest representa o corpo da requisição para reset
type AdminResetRequest struct {
	Key  string `json:"key" binding:"required"`
	Type string `json:"type" binding:"required"`
}

// AdminResetHandler implementa endpoint de reset administrativo
func (h *Handlers) AdminResetHandler(c *gin.Context) {
	ctx := c.Request.Context()

	// Parse do JSON
	var req AdminResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}

	// Limpar e validar parâmetros
	req.Key = strings.TrimSpace(req.Key)
	req.Type = strings.TrimSpace(strings.ToLower(req.Type))

	if h.logger != nil {
		logger := h.logger.WithContext(ctx)
		logger.Info("Admin reset endpoint accessed", map[string]interface{}{
			"key":  h.maskToken(req.Key),
			"type": req.Type,
		})
	}

	// Validar tipo de limiter
	var limiterType domain.LimiterType
	switch req.Type {
	case "ip":
		limiterType = domain.IPLimiter
	case "token":
		limiterType = domain.TokenLimiter
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": "type must be 'ip' or 'token'",
		})
		return
	}

	// Executar reset
	err := h.service.Reset(ctx, req.Key, limiterType)
	if err != nil {
		if h.logger != nil {
			logger := h.logger.WithContext(ctx)
			logger.Error("Failed to reset rate limiter", err, map[string]interface{}{
				"key":  h.maskToken(req.Key),
				"type": req.Type,
			})
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_server_error",
			"message": "Failed to reset rate limiter",
		})
		return
	}

	if h.logger != nil {
		logger := h.logger.WithContext(ctx)
		logger.Info("Rate limiter reset successfully", map[string]interface{}{
			"key":  h.maskToken(req.Key),
			"type": req.Type,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"message":   "Rate limiter reset successfully",
		"key":       h.maskToken(req.Key),
		"type":      req.Type,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// maskToken mascara tokens para logs de segurança
func (h *Handlers) maskToken(token string) string {
	if token == "" {
		return ""
	}
	
	if len(token) <= 8 {
		return token + "***"
	}
	
	return token[:8] + "***"
}

// max retorna o maior dos dois valores
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// formatBytes formata bytes em formato legível
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return strconv.FormatUint(bytes, 10) + " B"
	}
	
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	
	return strconv.FormatFloat(float64(bytes)/float64(div), 'f', 1, 64) + " " + "KMGTPE"[exp:exp+1] + "B"
} 