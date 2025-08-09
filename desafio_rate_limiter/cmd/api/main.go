package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"rate-limiter/internal/config"
	"rate-limiter/internal/handler"
	"rate-limiter/internal/logger"
	"rate-limiter/internal/service"
	"rate-limiter/internal/storage"
)

func main() {
	// Carregar configurações
	configLoader := config.NewConfigLoader()
	cfg, err := configLoader.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Obter configurações do servidor
	serverConfig := configLoader.GetConfig()

	// Inicializar logger
	appLogger := logger.NewLogger(serverConfig.LogLevel, serverConfig.LogFormat)
	appLogger.Info("Starting Rate Limiter API", map[string]interface{}{
		"version":   "1.0.0",
		"log_level": serverConfig.LogLevel,
		"port":      serverConfig.ServerPort,
	})

	// Inicializar storage (Memory por padrão para simplicidade)
	appLogger.Info("Using memory storage", nil)
	rateLimiterStorage := storage.NewMemoryStorage(appLogger)

	// Inicializar service
	rateLimiterService := service.NewRateLimiterService(rateLimiterStorage, cfg, appLogger)

	// Inicializar handlers
	handlers := handler.NewHandlers(rateLimiterService, appLogger)

	// Configurar Gin
	if serverConfig.GinMode == "release" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	// Criar router
	router := gin.New()

	// Middlewares globais
	router.Use(gin.Recovery())
	
	// Middleware de logging customizado
	router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("[%s] \"%s %s %s %d %s \"%s\" %s\"\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			param.Method,
			param.Path,
			param.Request.Proto,
			param.StatusCode,
			param.Latency,
			param.Request.UserAgent(),
			param.ErrorMessage,
		)
	}))

	// Configurar rotas
	handlers.SetupRoutes(router)

	// Configurar servidor HTTP
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", serverConfig.ServerPort),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Iniciar servidor em goroutine
	go func() {
		appLogger.Info("Starting HTTP server", map[string]interface{}{
			"port": serverConfig.ServerPort,
			"addr": server.Addr,
		})
		
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			appLogger.Error("Failed to start server", err, nil)
			os.Exit(1)
		}
	}()

	// Aguardar sinais de interrupção
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	
	appLogger.Info("🚀 Rate Limiter API is running!", map[string]interface{}{
		"port": serverConfig.ServerPort,
		"endpoints": []string{
			"GET  /health",
			"GET  /metrics", 
			"GET  /             (rate limited)",
			"GET  /admin/status",
			"POST /admin/reset",
		},
		"rate_limits": map[string]interface{}{
			"default_ip":    cfg.DefaultIPLimit,
			"default_token": cfg.DefaultTokenLimit,
			"window":        cfg.Window,
			"block_duration": cfg.BlockDuration,
		},
	})

	// Bloquear até receber sinal
	<-quit
	appLogger.Info("Shutting down server...", nil)

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		appLogger.Error("Server forced to shutdown", err, nil)
		os.Exit(1)
	}

	appLogger.Info("Server stopped gracefully", nil)
} 