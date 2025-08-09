package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		format   string
		expected logrus.Level
	}{
		{
			name:     "Debug level JSON format",
			level:    "debug",
			format:   "json",
			expected: logrus.DebugLevel,
		},
		{
			name:     "Info level text format",
			level:    "info",
			format:   "text",
			expected: logrus.InfoLevel,
		},
		{
			name:     "Invalid level defaults to info",
			level:    "invalid",
			format:   "json",
			expected: logrus.InfoLevel,
		},
		{
			name:     "Warn level",
			level:    "warn",
			format:   "json",
			expected: logrus.WarnLevel,
		},
		{
			name:     "Error level",
			level:    "error",
			format:   "json",
			expected: logrus.ErrorLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewLogger(tt.level, tt.format)
			structLogger, ok := logger.(*StructuredLogger)
			require.True(t, ok)
			assert.Equal(t, tt.expected, structLogger.logger.GetLevel())
		})
	}
}

func TestStructuredLogger_LogLevels(t *testing.T) {
	// Captura a saída do logger
	var buf bytes.Buffer
	
	// Cria logger com nível debug
	structLogger := &StructuredLogger{
		logger: &logrus.Logger{
			Out:       &buf,
			Formatter: &logrus.JSONFormatter{},
			Level:     logrus.DebugLevel,
		},
		fields: make(logrus.Fields),
	}

	tests := []struct {
		name     string
		logFunc  func()
		expected string
	}{
		{
			name: "Debug log",
			logFunc: func() {
				structLogger.Debug("Debug message", map[string]interface{}{"key": "value"})
			},
			expected: "debug",
		},
		{
			name: "Info log",
			logFunc: func() {
				structLogger.Info("Info message", map[string]interface{}{"key": "value"})
			},
			expected: "info",
		},
		{
			name: "Warn log",
			logFunc: func() {
				structLogger.Warn("Warn message", map[string]interface{}{"key": "value"})
			},
			expected: "warning",
		},
		{
			name: "Error log",
			logFunc: func() {
				structLogger.Error("Error message", errors.New("test error"), map[string]interface{}{"key": "value"})
			},
			expected: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.logFunc()
			
			output := buf.String()
			assert.Contains(t, output, tt.expected)
			assert.Contains(t, output, "component")
			assert.Contains(t, output, "rate_limiter")
		})
	}
}

func TestStructuredLogger_WithContext(t *testing.T) {
	var buf bytes.Buffer
	
	structLogger := &StructuredLogger{
		logger: &logrus.Logger{
			Out:       &buf,
			Formatter: &logrus.JSONFormatter{},
			Level:     logrus.DebugLevel,
		},
		fields: make(logrus.Fields),
	}

	// Cria contexto com informações da requisição
	ctx := context.Background()
	ctx = ContextWithRequestInfo(ctx, "req-123", "192.168.1.1", "abc123456789", "test-agent")

	// Cria logger com contexto
	contextLogger := structLogger.WithContext(ctx)
	
	// Testa log com contexto
	contextLogger.Info("Test message with context", nil)
	
	output := buf.String()
	
	// Verifica se os campos do contexto estão presentes
	assert.Contains(t, output, "req-123")
	assert.Contains(t, output, "192.168.1.1")
	assert.Contains(t, output, "abc12345***") // Token mascarado
	assert.Contains(t, output, "test-agent")
}

func TestStructuredLogger_LogRateLimitEvent(t *testing.T) {
	var buf bytes.Buffer
	
	structLogger := &StructuredLogger{
		logger: &logrus.Logger{
			Out:       &buf,
			Formatter: &logrus.JSONFormatter{},
			Level:     logrus.DebugLevel,
		},
		fields: make(logrus.Fields),
	}

	tests := []struct {
		name      string
		eventType string
		ip        string
		token     string
		allowed   bool
		limit     int
		remaining int
	}{
		{
			name:      "Rate limit passed",
			eventType: "rate_limit_check",
			ip:        "192.168.1.1",
			token:     "abc123",
			allowed:   true,
			limit:     10,
			remaining: 5,
		},
		{
			name:      "Rate limit exceeded",
			eventType: "rate_limit_exceeded",
			ip:        "192.168.1.2",
			token:     "",
			allowed:   false,
			limit:     10,
			remaining: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			structLogger.LogRateLimitEvent(tt.eventType, tt.ip, tt.token, tt.allowed, tt.limit, tt.remaining, nil)
			
			output := buf.String()
			
			assert.Contains(t, output, tt.eventType)
			assert.Contains(t, output, tt.ip)
			assert.Contains(t, output, "allowed")
			assert.Contains(t, output, "limit")
			assert.Contains(t, output, "remaining")
			
			if tt.token != "" {
				assert.Contains(t, output, tt.token+"***")
			}
		})
	}
}

func TestStructuredLogger_LogStorageEvent(t *testing.T) {
	var buf bytes.Buffer
	
	structLogger := &StructuredLogger{
		logger: &logrus.Logger{
			Out:       &buf,
			Formatter: &logrus.JSONFormatter{},
			Level:     logrus.DebugLevel,
		},
		fields: make(logrus.Fields),
	}

	tests := []struct {
		name      string
		operation string
		key       string
		success   bool
		latency   float64
		err       error
	}{
		{
			name:      "Successful operation",
			operation: "GET",
			key:       "rate_limit:ip:192.168.1.1",
			success:   true,
			latency:   1.5,
			err:       nil,
		},
		{
			name:      "Failed operation",
			operation: "SET",
			key:       "rate_limit:token:abc123",
			success:   false,
			latency:   0.0,
			err:       errors.New("connection failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			structLogger.LogStorageEvent(tt.operation, tt.key, tt.success, tt.latency, tt.err)
			
			output := buf.String()
			
			assert.Contains(t, output, tt.operation)
			assert.Contains(t, output, tt.key)
			assert.Contains(t, output, "success")
			assert.Contains(t, output, "latency_ms")
			
			if tt.err != nil {
				assert.Contains(t, output, "error")
				assert.Contains(t, output, tt.err.Error())
			}
		})
	}
}

func TestContextWithRequestInfo(t *testing.T) {
	ctx := context.Background()
	
	requestID := "req-456"
	ip := "10.0.0.1"
	token := "token123"
	userAgent := "Mozilla/5.0"
	
	enrichedCtx := ContextWithRequestInfo(ctx, requestID, ip, token, userAgent)
	
	// Verifica se os valores estão no contexto
	assert.Equal(t, requestID, enrichedCtx.Value(RequestIDKey))
	assert.Equal(t, ip, enrichedCtx.Value(IPKey))
	assert.Equal(t, token, enrichedCtx.Value(TokenKey))
	assert.Equal(t, userAgent, enrichedCtx.Value(UserAgentKey))
}

func TestGetRequestID(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		expected string
	}{
		{
			name:     "Nil context",
			ctx:      nil,
			expected: "",
		},
		{
			name:     "Context without request ID",
			ctx:      context.Background(),
			expected: "",
		},
		{
			name:     "Context with request ID",
			ctx:      context.WithValue(context.Background(), RequestIDKey, "req-789"),
			expected: "req-789",
		},
		{
			name:     "Context with invalid request ID type",
			ctx:      context.WithValue(context.Background(), RequestIDKey, 123),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetRequestID(tt.ctx)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStructuredLogger_TokenMasking(t *testing.T) {
	var buf bytes.Buffer
	
	structLogger := &StructuredLogger{
		logger: &logrus.Logger{
			Out:       &buf,
			Formatter: &logrus.JSONFormatter{},
			Level:     logrus.DebugLevel,
		},
		fields: make(logrus.Fields),
	}

	tests := []struct {
		name          string
		token         string
		expectedMask  string
	}{
		{
			name:         "Long token",
			token:        "verylongtoken123456789",
			expectedMask: "verylong***",
		},
		{
			name:         "Short token",
			token:        "short",
			expectedMask: "short***",
		},
		{
			name:         "Exact 8 chars",
			token:        "exactly8",
			expectedMask: "exactly8***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			
			ctx := ContextWithRequestInfo(context.Background(), "req-1", "1.1.1.1", tt.token, "agent")
			contextLogger := structLogger.WithContext(ctx)
			contextLogger.Info("Test token masking", nil)
			
			output := buf.String()
			assert.Contains(t, output, tt.expectedMask)
			// Verifica que o token completo não aparece, apenas a parte mascarada
			if len(tt.token) > 8 {
				assert.NotContains(t, output, tt.token) // Token original não deve aparecer
			}
		})
	}
}

func TestStructuredLogger_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	
	structLogger := &StructuredLogger{
		logger: &logrus.Logger{
			Out:       &buf,
			Formatter: &logrus.JSONFormatter{},
			Level:     logrus.InfoLevel,
		},
		fields: make(logrus.Fields),
	}

	structLogger.Info("Test JSON format", map[string]interface{}{
		"test_field": "test_value",
		"number":     123,
	})

	output := buf.String()
	
	// Verifica se é um JSON válido
	var logEntry map[string]interface{}
	err := json.Unmarshal([]byte(strings.TrimSpace(output)), &logEntry)
	require.NoError(t, err)
	
	// Verifica campos obrigatórios
	assert.Contains(t, logEntry, "msg")  // logrus usa "msg" por padrão
	assert.Contains(t, logEntry, "level")
	assert.Contains(t, logEntry, "component")
	assert.Contains(t, logEntry, "test_field")
	assert.Equal(t, "rate_limiter", logEntry["component"])
	assert.Equal(t, "test_value", logEntry["test_field"])
	assert.Equal(t, float64(123), logEntry["number"])
} 