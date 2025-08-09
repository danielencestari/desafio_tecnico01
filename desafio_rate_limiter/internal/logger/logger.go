package logger

import (
	"context"
	"os"
	"strings"

	"rate-limiter/internal/domain"

	"github.com/sirupsen/logrus"
)

// StructuredLogger implementa a interface domain.Logger
type StructuredLogger struct {
	logger *logrus.Logger
	fields logrus.Fields
}

// contextKey define chaves para contexto
type contextKey string

const (
	RequestIDKey contextKey = "request_id"
	IPKey        contextKey = "ip"
	TokenKey     contextKey = "token"
	UserAgentKey contextKey = "user_agent"
)

// NewLogger cria uma nova instância do logger estruturado
func NewLogger(level, format string) domain.Logger {
	logger := logrus.New()

	// Configura o nível de log
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		logLevel = logrus.InfoLevel
	}
	logger.SetLevel(logLevel)

	// Configura o formato de saída
	switch strings.ToLower(format) {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
				logrus.FieldKeyFunc:  "function",
			},
		})
	default:
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})
	}

	// Define saída
	logger.SetOutput(os.Stdout)

	return &StructuredLogger{
		logger: logger,
		fields: make(logrus.Fields),
	}
}

// Debug registra uma mensagem de debug
func (l *StructuredLogger) Debug(msg string, fields map[string]interface{}) {
	l.logWithFields(logrus.DebugLevel, msg, fields)
}

// Info registra uma mensagem informativa
func (l *StructuredLogger) Info(msg string, fields map[string]interface{}) {
	l.logWithFields(logrus.InfoLevel, msg, fields)
}

// Warn registra uma mensagem de warning
func (l *StructuredLogger) Warn(msg string, fields map[string]interface{}) {
	l.logWithFields(logrus.WarnLevel, msg, fields)
}

// Error registra uma mensagem de erro
func (l *StructuredLogger) Error(msg string, err error, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	if err != nil {
		fields["error"] = err.Error()
	}
	l.logWithFields(logrus.ErrorLevel, msg, fields)
}

// WithContext cria um novo logger com contexto da requisição
func (l *StructuredLogger) WithContext(ctx context.Context) domain.Logger {
	contextFields := l.extractContextFields(ctx)
	
	// Mescla campos do contexto com campos existentes
	mergedFields := make(logrus.Fields)
	for k, v := range l.fields {
		mergedFields[k] = v
	}
	for k, v := range contextFields {
		mergedFields[k] = v
	}

	return &StructuredLogger{
		logger: l.logger,
		fields: mergedFields,
	}
}

// logWithFields registra uma mensagem com campos específicos
func (l *StructuredLogger) logWithFields(level logrus.Level, msg string, fields map[string]interface{}) {
	// Mescla todos os campos
	allFields := make(logrus.Fields)
	
	// Adiciona campos do logger
	for k, v := range l.fields {
		allFields[k] = v
	}
	
	// Adiciona campos da mensagem
	if fields != nil {
		for k, v := range fields {
			allFields[k] = v
		}
	}

	// Adiciona informações específicas do rate limiter
	l.addRateLimiterFields(allFields)

	// Log da mensagem
	l.logger.WithFields(allFields).Log(level, msg)
}

// extractContextFields extrai campos relevantes do contexto
func (l *StructuredLogger) extractContextFields(ctx context.Context) logrus.Fields {
	fields := make(logrus.Fields)

	if ctx == nil {
		return fields
	}

	// Extrai request ID
	if requestID := ctx.Value(RequestIDKey); requestID != nil {
		fields["request_id"] = requestID
	}

	// Extrai IP
	if ip := ctx.Value(IPKey); ip != nil {
		fields["ip"] = ip
	}

	// Extrai token (apenas os primeiros 8 caracteres por segurança)
	if token := ctx.Value(TokenKey); token != nil {
		if tokenStr, ok := token.(string); ok && len(tokenStr) > 0 {
			if len(tokenStr) > 8 {
				fields["token"] = tokenStr[:8] + "***"
			} else {
				fields["token"] = tokenStr + "***"
			}
		}
	}

	// Extrai user agent
	if userAgent := ctx.Value(UserAgentKey); userAgent != nil {
		fields["user_agent"] = userAgent
	}

	return fields
}

// addRateLimiterFields adiciona campos específicos do rate limiter
func (l *StructuredLogger) addRateLimiterFields(fields logrus.Fields) {
	// Adiciona componente
	fields["component"] = "rate_limiter"
	
	// Adiciona versão se disponível
	if version := os.Getenv("APP_VERSION"); version != "" {
		fields["version"] = version
	}
}

// WithFields cria um novo logger com campos específicos
func (l *StructuredLogger) WithFields(fields map[string]interface{}) domain.Logger {
	newFields := make(logrus.Fields)
	
	// Copia campos existentes
	for k, v := range l.fields {
		newFields[k] = v
	}
	
	// Adiciona novos campos
	for k, v := range fields {
		newFields[k] = v
	}

	return &StructuredLogger{
		logger: l.logger,
		fields: newFields,
	}
}

// LogRateLimitEvent registra eventos específicos de rate limiting
func (l *StructuredLogger) LogRateLimitEvent(eventType string, ip, token string, allowed bool, limit, remaining int, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}

	// Adiciona campos específicos do rate limit
	fields["event_type"] = eventType
	fields["ip"] = ip
	fields["allowed"] = allowed
	fields["limit"] = limit
	fields["remaining"] = remaining

	// Adiciona token mascarado se presente
	if token != "" {
		if len(token) > 8 {
			fields["token"] = token[:8] + "***"
		} else {
			fields["token"] = token + "***"
		}
	}

	if allowed {
		l.Info("Rate limit check passed", fields)
	} else {
		l.Warn("Rate limit exceeded", fields)
	}
}

// LogConfigEvent registra eventos de configuração
func (l *StructuredLogger) LogConfigEvent(eventType string, details map[string]interface{}) {
	if details == nil {
		details = make(map[string]interface{})
	}
	details["event_type"] = eventType
	
	l.Info("Configuration event", details)
}

// LogStorageEvent registra eventos do storage
func (l *StructuredLogger) LogStorageEvent(operation string, key string, success bool, latency float64, err error) {
	fields := map[string]interface{}{
		"operation": operation,
		"key":       key,
		"success":   success,
		"latency_ms": latency,
	}

	if err != nil {
		fields["error"] = err.Error()
		l.Error("Storage operation failed", err, fields)
	} else {
		l.Debug("Storage operation completed", fields)
	}
}

// ContextWithRequestInfo adiciona informações da requisição ao contexto
func ContextWithRequestInfo(ctx context.Context, requestID, ip, token, userAgent string) context.Context {
	ctx = context.WithValue(ctx, RequestIDKey, requestID)
	ctx = context.WithValue(ctx, IPKey, ip)
	if token != "" {
		ctx = context.WithValue(ctx, TokenKey, token)
	}
	ctx = context.WithValue(ctx, UserAgentKey, userAgent)
	return ctx
}

// GetRequestID extrai o request ID do contexto
func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if requestID := ctx.Value(RequestIDKey); requestID != nil {
		if id, ok := requestID.(string); ok {
			return id
		}
	}
	return ""
} 