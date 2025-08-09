package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"rate-limiter/internal/domain"
)

// MockRateLimiterService é um mock do RateLimiterService para testes
type MockRateLimiterService struct {
	mock.Mock
}

func (m *MockRateLimiterService) CheckLimit(ctx context.Context, ip, token string) (*domain.RateLimitResult, error) {
	args := m.Called(ctx, ip, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.RateLimitResult), args.Error(1)
}

func (m *MockRateLimiterService) IsAllowed(ctx context.Context, key string, limiterType domain.LimiterType) (bool, error) {
	args := m.Called(ctx, key, limiterType)
	return args.Bool(0), args.Error(1)
}

func (m *MockRateLimiterService) GetConfig(key string, limiterType domain.LimiterType) *domain.RateLimitRule {
	args := m.Called(key, limiterType)
	return args.Get(0).(*domain.RateLimitRule)
}

func (m *MockRateLimiterService) GetStatus(ctx context.Context, key string, limiterType domain.LimiterType) (*domain.RateLimitStatus, error) {
	args := m.Called(ctx, key, limiterType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.RateLimitStatus), args.Error(1)
}

func (m *MockRateLimiterService) Reset(ctx context.Context, key string, limiterType domain.LimiterType) error {
	args := m.Called(ctx, key, limiterType)
	return args.Error(0)
}

// MockLogger é um mock do Logger para testes
type MockLogger struct {
	mock.Mock
}

func (m *MockLogger) Debug(msg string, fields map[string]interface{}) {
	m.Called(msg, fields)
}

func (m *MockLogger) Info(msg string, fields map[string]interface{}) {
	m.Called(msg, fields)
}

func (m *MockLogger) Warn(msg string, fields map[string]interface{}) {
	m.Called(msg, fields)
}

func (m *MockLogger) Error(msg string, err error, fields map[string]interface{}) {
	m.Called(msg, err, fields)
}

func (m *MockLogger) WithContext(ctx context.Context) domain.Logger {
	args := m.Called(ctx)
	return args.Get(0).(domain.Logger)
}

// setupTestRouter cria um router para testes
func setupTestRouter(h *Handlers) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h.SetupRoutes(router)
	return router
}

// TestHealthHandler testa o endpoint de health check
func TestHealthHandler(t *testing.T) {
	// Arrange
	handlers := NewHandlers(nil, nil)
	
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/health", handlers.HealthHandler)

	// Act
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "healthy", response["status"])
	assert.Equal(t, "Rate Limiter API", response["service"])
	assert.NotEmpty(t, response["timestamp"])
}

// TestExampleHandler testa o endpoint de exemplo (protegido por rate limiter)
func TestExampleHandler(t *testing.T) {
	// Arrange
	mockService := new(MockRateLimiterService)
	mockLogger := new(MockLogger)
	
	handlers := NewHandlers(mockService, mockLogger)
	
	// Configurar router sem middleware para testar apenas o handler
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/", handlers.ExampleHandler)

	// Mock expectations
	mockLogger.On("WithContext", mock.Anything).Return(mockLogger)
	mockLogger.On("Debug", mock.AnythingOfType("string"), mock.Anything).Once()

	// Act
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Rate Limiter API", response["service"])
	assert.NotEmpty(t, response["timestamp"])
	
	mockLogger.AssertExpectations(t)
}

// TestAdminStatusHandler testa o endpoint de status administrativo
func TestAdminStatusHandler(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		mockSetup      func(*MockRateLimiterService, *MockLogger)
		expectedStatus int
		expectedFields []string
	}{
		{
			name:        "Should return IP status",
			queryParams: "?key=192.168.1.1&type=ip",
			mockSetup: func(service *MockRateLimiterService, logger *MockLogger) {
				status := &domain.RateLimitStatus{
					Key:         "rate_limit:ip:192.168.1.1",
					Type:        domain.IPLimiter,
					Count:       5,
					Limit:       10,
					Window:      60,
					LastReset:   time.Now().Add(-30*time.Second),
					IsBlocked:   false,
				}
				service.On("GetStatus", mock.Anything, "192.168.1.1", domain.IPLimiter).Return(status, nil)
				logger.On("WithContext", mock.Anything).Return(logger)
				logger.On("Debug", mock.AnythingOfType("string"), mock.Anything).Maybe()
			},
			expectedStatus: http.StatusOK,
			expectedFields: []string{"key", "limit", "current", "remaining", "reset_time", "is_blocked", "limiter_type"},
		},
		{
			name:        "Should return Token status",
			queryParams: "?key=premium_token&type=token",
			mockSetup: func(service *MockRateLimiterService, logger *MockLogger) {
				status := &domain.RateLimitStatus{
					Key:         "rate_limit:token:premium_token",
					Type:        domain.TokenLimiter,
					Count:       100,
					Limit:       1000,
					Window:      60,
					LastReset:   time.Now().Add(-30*time.Second),
					IsBlocked:   false,
				}
				service.On("GetStatus", mock.Anything, "premium_token", domain.TokenLimiter).Return(status, nil)
				logger.On("WithContext", mock.Anything).Return(logger)
				logger.On("Debug", mock.AnythingOfType("string"), mock.Anything).Maybe()
			},
			expectedStatus: http.StatusOK,
			expectedFields: []string{"key", "limit", "current", "remaining", "reset_time", "is_blocked", "limiter_type"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockService := new(MockRateLimiterService)
			mockLogger := new(MockLogger)
			
			tt.mockSetup(mockService, mockLogger)
			
			handlers := NewHandlers(mockService, mockLogger)
			router := setupTestRouter(handlers)

			// Act
			req := httptest.NewRequest("GET", "/admin/status"+tt.queryParams, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Assert
			assert.Equal(t, tt.expectedStatus, w.Code)
			
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			
			for _, field := range tt.expectedFields {
				assert.Contains(t, response, field)
			}

			mockService.AssertExpectations(t)
		})
	}
}

// TestAdminStatusHandler_ValidationErrors testa validação de parâmetros
func TestAdminStatusHandler_ValidationErrors(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Should require key parameter",
			queryParams:    "?type=ip",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "key parameter is required",
		},
		{
			name:           "Should require type parameter", 
			queryParams:    "?key=192.168.1.1",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "type parameter is required",
		},
		{
			name:           "Should validate type parameter",
			queryParams:    "?key=192.168.1.1&type=invalid",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "type must be 'ip' or 'token'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockService := new(MockRateLimiterService)
			mockLogger := new(MockLogger)
			
			handlers := NewHandlers(mockService, mockLogger)
			router := setupTestRouter(handlers)

			// Act
			req := httptest.NewRequest("GET", "/admin/status"+tt.queryParams, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Assert
			assert.Equal(t, tt.expectedStatus, w.Code)
			
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Contains(t, response["error"], tt.expectedError)
		})
	}
}

// TestAdminResetHandler testa o endpoint de reset administrativo
func TestAdminResetHandler(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		mockSetup      func(*MockRateLimiterService, *MockLogger)
		expectedStatus int
	}{
		{
			name: "Should reset IP rate limit",
			requestBody: map[string]interface{}{
				"key":  "192.168.1.1",
				"type": "ip",
			},
			mockSetup: func(service *MockRateLimiterService, logger *MockLogger) {
				service.On("Reset", mock.Anything, "192.168.1.1", domain.IPLimiter).Return(nil)
				logger.On("WithContext", mock.Anything).Return(logger)
				logger.On("Info", mock.AnythingOfType("string"), mock.Anything).Once()
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Should reset Token rate limit",
			requestBody: map[string]interface{}{
				"key":  "premium_token",
				"type": "token",
			},
			mockSetup: func(service *MockRateLimiterService, logger *MockLogger) {
				service.On("Reset", mock.Anything, "premium_token", domain.TokenLimiter).Return(nil)
				logger.On("WithContext", mock.Anything).Return(logger)
				logger.On("Info", mock.AnythingOfType("string"), mock.Anything).Once()
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockService := new(MockRateLimiterService)
			mockLogger := new(MockLogger)
			
			tt.mockSetup(mockService, mockLogger)
			
			handlers := NewHandlers(mockService, mockLogger)
			router := setupTestRouter(handlers)

			// Preparar request body
			bodyBytes, _ := json.Marshal(tt.requestBody)

			// Act
			req := httptest.NewRequest("POST", "/admin/reset", bytes.NewBuffer(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Assert
			assert.Equal(t, tt.expectedStatus, w.Code)
			
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, "success", response["status"])

			mockService.AssertExpectations(t)
			mockLogger.AssertExpectations(t)
		})
	}
}

// TestMetricsHandler testa o endpoint de métricas
func TestMetricsHandler(t *testing.T) {
	// Arrange
	mockLogger := new(MockLogger)
	handlers := NewHandlers(nil, mockLogger)
	
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/metrics", handlers.MetricsHandler)

	// Mock expectations
	mockLogger.On("WithContext", mock.Anything).Return(mockLogger)
	mockLogger.On("Debug", mock.AnythingOfType("string"), mock.Anything).Once()

	// Act
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	
	// Verifica estrutura básica das métricas
	assert.Equal(t, "Rate Limiter API", response["service"])
	assert.Contains(t, response, "uptime")
	assert.Contains(t, response, "timestamp")
	assert.Contains(t, response, "system")
	
	mockLogger.AssertExpectations(t)
} 