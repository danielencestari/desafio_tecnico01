package middleware

import (
	"context"
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

// setupTestRouter cria um router Gin para testes
func setupTestRouter(middleware gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware)
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})
	return router
}

// TestRateLimiterMiddleware_AllowedRequest testa requisições permitidas
func TestRateLimiterMiddleware_AllowedRequest(t *testing.T) {
	// Arrange
	mockService := new(MockRateLimiterService)
	mockLogger := new(MockLogger)

	middleware := NewRateLimiterMiddleware(mockService, mockLogger)
	router := setupTestRouter(middleware)

	result := &domain.RateLimitResult{
		Allowed:     true,
		Limit:       10,
		Remaining:   8,
		ResetTime:   time.Now().Add(time.Minute),
		LimiterType: domain.IPLimiter,
	}

	// Mock expectations - usar mock.Anything para contexto
	mockService.On("CheckLimit", mock.Anything, "192.168.1.1", "").Return(result, nil)
	mockLogger.On("WithContext", mock.Anything).Return(mockLogger)
	mockLogger.On("Debug", mock.AnythingOfType("string"), mock.Anything).Maybe()

	// Act
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "10", w.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "8", w.Header().Get("X-RateLimit-Remaining"))
	assert.Equal(t, "ip", w.Header().Get("X-RateLimit-Type"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Reset"))

	mockService.AssertExpectations(t)
}

// TestRateLimiterMiddleware_BlockedRequest testa requisições bloqueadas
func TestRateLimiterMiddleware_BlockedRequest(t *testing.T) {
	// Arrange
	mockService := new(MockRateLimiterService)
	mockLogger := new(MockLogger)

	middleware := NewRateLimiterMiddleware(mockService, mockLogger)
	router := setupTestRouter(middleware)

	result := &domain.RateLimitResult{
		Allowed:     false,
		Limit:       10,
		Remaining:   0,
		ResetTime:   time.Now().Add(time.Minute),
		LimiterType: domain.IPLimiter,
	}

	// Mock expectations
	mockService.On("CheckLimit", mock.Anything, "192.168.1.100", "").Return(result, nil)
	mockLogger.On("WithContext", mock.Anything).Return(mockLogger)
	mockLogger.On("Debug", mock.AnythingOfType("string"), mock.Anything).Maybe()
	mockLogger.On("Info", mock.AnythingOfType("string"), mock.Anything).Maybe()

	// Act
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.100")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "you have reached the maximum number of requests or actions allowed within a certain time frame")
	assert.Equal(t, "0", w.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Reset"))

	mockService.AssertExpectations(t)
}

// TestRateLimiterMiddleware_IPExtraction testa extração de IP
func TestRateLimiterMiddleware_IPExtraction(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		remoteAddr     string
		expectedIP     string
	}{
		{
			name: "Should extract IP from X-Forwarded-For",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.1, 70.41.3.18, 150.172.238.178",
			},
			expectedIP: "203.0.113.1",
		},
		{
			name: "Should extract IP from X-Real-IP",
			headers: map[string]string{
				"X-Real-IP": "203.0.113.2",
			},
			expectedIP: "203.0.113.2",
		},
		{
			name: "Should fallback to RemoteAddr",
			headers:    map[string]string{},
			remoteAddr: "192.168.1.100:12345",
			expectedIP: "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockService := new(MockRateLimiterService)
			mockLogger := new(MockLogger)

			middleware := NewRateLimiterMiddleware(mockService, mockLogger)
			router := setupTestRouter(middleware)

			result := &domain.RateLimitResult{
				Allowed:     true,
				Limit:       10,
				Remaining:   5,
				ResetTime:   time.Now().Add(time.Minute),
				LimiterType: domain.IPLimiter,
			}

			// Mock expectations - verifica se o IP correto é passado
			mockService.On("CheckLimit", mock.Anything, tt.expectedIP, "").Return(result, nil)
			mockLogger.On("WithContext", mock.Anything).Return(mockLogger)
			mockLogger.On("Debug", mock.AnythingOfType("string"), mock.Anything).Maybe()

			// Act
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tt.remoteAddr
			for headerName, headerValue := range tt.headers {
				req.Header.Set(headerName, headerValue)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Assert
			assert.Equal(t, http.StatusOK, w.Code)
			mockService.AssertExpectations(t)
		})
	}
}

// TestRateLimiterMiddleware_TokenExtraction testa extração de token
func TestRateLimiterMiddleware_TokenExtraction(t *testing.T) {
	tests := []struct {
		name          string
		headers       map[string]string
		expectedToken string
	}{
		{
			name: "Should extract token from X-Api-Token",
			headers: map[string]string{
				"X-Api-Token": "premium_token",
			},
			expectedToken: "premium_token",
		},
		{
			name: "Should extract token from Api-Token",
			headers: map[string]string{
				"Api-Token": "basic_token",
			},
			expectedToken: "basic_token",
		},
		{
			name:          "Should handle no token",
			headers:       map[string]string{},
			expectedToken: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockService := new(MockRateLimiterService)
			mockLogger := new(MockLogger)

			middleware := NewRateLimiterMiddleware(mockService, mockLogger)
			router := setupTestRouter(middleware)

			result := &domain.RateLimitResult{
				Allowed:     true,
				Limit:       100,
				Remaining:   50,
				ResetTime:   time.Now().Add(time.Minute),
				LimiterType: func() domain.LimiterType {
					if tt.expectedToken != "" {
						return domain.TokenLimiter
					}
					return domain.IPLimiter
				}(),
			}

			// Mock expectations - verifica se o token correto é passado
			mockService.On("CheckLimit", mock.Anything, mock.AnythingOfType("string"), tt.expectedToken).Return(result, nil)
			mockLogger.On("WithContext", mock.Anything).Return(mockLogger)
			mockLogger.On("Debug", mock.AnythingOfType("string"), mock.Anything).Maybe()

			// Act
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("X-Forwarded-For", "192.168.1.1")
			for headerName, headerValue := range tt.headers {
				req.Header.Set(headerName, headerValue)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Assert
			assert.Equal(t, http.StatusOK, w.Code)
			mockService.AssertExpectations(t)
		})
	}
}

// TestRateLimiterMiddleware_ServiceError testa tratamento de erros do service
func TestRateLimiterMiddleware_ServiceError(t *testing.T) {
	// Arrange
	mockService := new(MockRateLimiterService)
	mockLogger := new(MockLogger)

	middleware := NewRateLimiterMiddleware(mockService, mockLogger)
	router := setupTestRouter(middleware)

	// Mock expectations - simula erro do service
	mockService.On("CheckLimit", mock.Anything, "192.168.1.1", "").Return(nil, assert.AnError)
	mockLogger.On("WithContext", mock.Anything).Return(mockLogger)
	mockLogger.On("Debug", mock.AnythingOfType("string"), mock.Anything).Maybe()
	mockLogger.On("Error", mock.AnythingOfType("string"), assert.AnError, mock.Anything).Once()

	// Act
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "internal server error")

	mockService.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

// Helper functions
func timePtr(t time.Time) *time.Time {
	return &t
} 