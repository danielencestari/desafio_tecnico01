package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"rate-limiter/internal/domain"
)

// MockStorage é um mock do RateLimiterStorage para testes
type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) Get(ctx context.Context, key string) (*domain.RateLimitStatus, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.RateLimitStatus), args.Error(1)
}

func (m *MockStorage) Set(ctx context.Context, key string, status *domain.RateLimitStatus, ttl time.Duration) error {
	args := m.Called(ctx, key, status, ttl)
	return args.Error(0)
}

func (m *MockStorage) Increment(ctx context.Context, key string, limit int, window time.Duration) (int, time.Time, error) {
	args := m.Called(ctx, key, limit, window)
	return args.Int(0), args.Get(1).(time.Time), args.Error(2)
}

func (m *MockStorage) IsBlocked(ctx context.Context, key string) (bool, *time.Time, error) {
	args := m.Called(ctx, key)
	var blockTime *time.Time
	if args.Get(1) != nil {
		blockTime = args.Get(1).(*time.Time)
	}
	return args.Bool(0), blockTime, args.Error(2)
}

func (m *MockStorage) Block(ctx context.Context, key string, duration time.Duration) error {
	args := m.Called(ctx, key, duration)
	return args.Error(0)
}

func (m *MockStorage) Reset(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *MockStorage) Health(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockStorage) Close() error {
	args := m.Called()
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

// Helper para criar configuração de teste
func createTestConfig() *domain.RateLimitConfig {
	return &domain.RateLimitConfig{
		DefaultIPLimit:    10,
		DefaultTokenLimit: 100,
		Window:           60,
		BlockDuration:    180, // 3 minutos
		TokenConfigs: map[string]domain.TokenConfig{
			"premium_token": {
				Token:       "premium_token",
				Limit:       1000,
				Description: "Token premium com alto limite",
			},
			"basic_token": {
				Token:       "basic_token", 
				Limit:       50,
				Description: "Token básico com limite reduzido",
			},
		},
	}
}

// TestRateLimiterService_CheckLimit_IPLimiting testa limitação por IP
func TestRateLimiterService_CheckLimit_IPLimiting(t *testing.T) {
	tests := []struct {
		name           string
		ip             string
		token          string
		currentCount   int
		isBlocked      bool
		blockTime      *time.Time
		expectedResult *domain.RateLimitResult
		expectError    bool
	}{
		{
			name:         "Should allow IP request within limit",
			ip:           "192.168.1.1",
			token:        "",
			currentCount: 5,
			isBlocked:    false,
			expectedResult: &domain.RateLimitResult{
				Allowed:     true,
				Limit:       10,
				Remaining:   5,
				LimiterType: domain.IPLimiter,
			},
			expectError: false,
		},
        {
            name:         "Should allow IP request at limit",
			ip:           "192.168.1.2",
			token:        "",
			currentCount: 10,
			isBlocked:    false,
			expectedResult: &domain.RateLimitResult{
                Allowed:     true,
				Limit:       10,
                Remaining:   0,
				LimiterType: domain.IPLimiter,
			},
			expectError: false,
		},
		{
			name:         "Should reject blocked IP",
			ip:           "192.168.1.3",
			token:        "",
			currentCount: 0,
			isBlocked:    true,
			blockTime:    timePtr(time.Now().Add(time.Minute)),
			expectedResult: &domain.RateLimitResult{
				Allowed:     false,
				Limit:       10,
				Remaining:   0,
				LimiterType: domain.IPLimiter,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockStorage := new(MockStorage)
			mockLogger := new(MockLogger)
			config := createTestConfig()

			service := NewRateLimiterService(mockStorage, config, mockLogger)

			ctx := context.Background()
			expectedKey := fmt.Sprintf("rate_limit:ip:%s", tt.ip)

			// Mock expectations
			mockStorage.On("IsBlocked", ctx, expectedKey).Return(tt.isBlocked, tt.blockTime, nil)
			
			if !tt.isBlocked {
				resetTime := time.Now().Add(time.Duration(config.Window) * time.Second)
				mockStorage.On("Increment", ctx, expectedKey, config.DefaultIPLimit, time.Duration(config.Window)*time.Second).
					Return(tt.currentCount, resetTime, nil)
				
                if tt.currentCount > config.DefaultIPLimit {
					blockDuration := time.Duration(config.BlockDuration) * time.Second
					mockStorage.On("Block", ctx, expectedKey, blockDuration).Return(nil)
				}
			}

			mockLogger.On("Debug", mock.AnythingOfType("string"), mock.AnythingOfType("map[string]interface {}")).Maybe()
			mockLogger.On("Info", mock.AnythingOfType("string"), mock.AnythingOfType("map[string]interface {}")).Maybe()

			// Act
			result, err := service.CheckLimit(ctx, tt.ip, tt.token)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedResult.Allowed, result.Allowed)
				assert.Equal(t, tt.expectedResult.Limit, result.Limit)
				assert.Equal(t, tt.expectedResult.LimiterType, result.LimiterType)
				
				if tt.isBlocked && tt.blockTime != nil {
					assert.NotNil(t, result.BlockedUntil)
				}
			}

			mockStorage.AssertExpectations(t)
		})
	}
}

// TestRateLimiterService_CheckLimit_TokenLimiting testa limitação por token
func TestRateLimiterService_CheckLimit_TokenLimiting(t *testing.T) {
	tests := []struct {
		name           string
		ip             string
		token          string
		currentCount   int
		isBlocked      bool
		expectedLimit  int
		expectedResult *domain.RateLimitResult
		expectError    bool
	}{
		{
			name:          "Should allow premium token request",
			ip:            "192.168.1.1",
			token:         "premium_token",
			currentCount:  500,
			isBlocked:     false,
			expectedLimit: 1000,
			expectedResult: &domain.RateLimitResult{
				Allowed:     true,
				Limit:       1000,
				Remaining:   500,
				LimiterType: domain.TokenLimiter,
			},
			expectError: false,
		},
		{
			name:          "Should allow basic token request",
			ip:            "192.168.1.2",
			token:         "basic_token",
			currentCount:  25,
			isBlocked:     false,
			expectedLimit: 50,
			expectedResult: &domain.RateLimitResult{
				Allowed:     true,
				Limit:       50,
				Remaining:   25,
				LimiterType: domain.TokenLimiter,
			},
			expectError: false,
		},
		{
			name:          "Should use default token limit for unknown token",
			ip:            "192.168.1.3",
			token:         "unknown_token",
			currentCount:  80,
			isBlocked:     false,
			expectedLimit: 100,
			expectedResult: &domain.RateLimitResult{
				Allowed:     true,
				Limit:       100,
				Remaining:   20,
				LimiterType: domain.TokenLimiter,
			},
			expectError: false,
		},
        {
            name:          "Should allow basic token at limit",
			ip:            "192.168.1.4",
			token:         "basic_token",
			currentCount:  50,
			isBlocked:     false,
			expectedLimit: 50,
			expectedResult: &domain.RateLimitResult{
                Allowed:     true,
				Limit:       50,
				Remaining:   0,
				LimiterType: domain.TokenLimiter,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockStorage := new(MockStorage)
			mockLogger := new(MockLogger)
			config := createTestConfig()

			service := NewRateLimiterService(mockStorage, config, mockLogger)

			ctx := context.Background()
			expectedKey := fmt.Sprintf("rate_limit:token:%s", tt.token)

			// Mock expectations
			mockStorage.On("IsBlocked", ctx, expectedKey).Return(tt.isBlocked, (*time.Time)(nil), nil)
			
			if !tt.isBlocked {
				resetTime := time.Now().Add(time.Duration(config.Window) * time.Second)
				mockStorage.On("Increment", ctx, expectedKey, tt.expectedLimit, time.Duration(config.Window)*time.Second).
					Return(tt.currentCount, resetTime, nil)
				
                if tt.currentCount > tt.expectedLimit {
					blockDuration := time.Duration(config.BlockDuration) * time.Second
					mockStorage.On("Block", ctx, expectedKey, blockDuration).Return(nil)
				}
			}

			mockLogger.On("Debug", mock.AnythingOfType("string"), mock.AnythingOfType("map[string]interface {}")).Maybe()
			mockLogger.On("Info", mock.AnythingOfType("string"), mock.AnythingOfType("map[string]interface {}")).Maybe()

			// Act
			result, err := service.CheckLimit(ctx, tt.ip, tt.token)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedResult.Allowed, result.Allowed)
				assert.Equal(t, tt.expectedResult.Limit, result.Limit)
				assert.Equal(t, tt.expectedResult.LimiterType, result.LimiterType)
			}

			mockStorage.AssertExpectations(t)
		})
	}
}

// TestRateLimiterService_GetConfig testa obtenção de configuração
func TestRateLimiterService_GetConfig(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		limiterType   domain.LimiterType
		expectedLimit int
	}{
		{
			name:          "Should get IP config",
			key:           "192.168.1.1",
			limiterType:   domain.IPLimiter,
			expectedLimit: 10,
		},
		{
			name:          "Should get premium token config",
			key:           "premium_token",
			limiterType:   domain.TokenLimiter,
			expectedLimit: 1000,
		},
		{
			name:          "Should get basic token config",
			key:           "basic_token",
			limiterType:   domain.TokenLimiter,
			expectedLimit: 50,
		},
		{
			name:          "Should get default token config for unknown token",
			key:           "unknown_token",
			limiterType:   domain.TokenLimiter,
			expectedLimit: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockStorage := new(MockStorage)
			mockLogger := new(MockLogger)
			config := createTestConfig()

			service := NewRateLimiterService(mockStorage, config, mockLogger)

			// Act
			rule := service.GetConfig(tt.key, tt.limiterType)

			// Assert
			assert.NotNil(t, rule)
			assert.Equal(t, tt.expectedLimit, rule.Limit)
			assert.Equal(t, tt.limiterType, rule.Type)
			assert.Equal(t, tt.key, rule.Key)
		})
	}
}

// TestRateLimiterService_Reset testa reset de contadores
func TestRateLimiterService_Reset(t *testing.T) {
	// Arrange
	mockStorage := new(MockStorage)
	mockLogger := new(MockLogger)
	config := createTestConfig()

	service := NewRateLimiterService(mockStorage, config, mockLogger)

	ctx := context.Background()
	key := "192.168.1.1"
	limiterType := domain.IPLimiter
	expectedStorageKey := "rate_limit:ip:192.168.1.1"

	// Mock expectations
	mockStorage.On("Reset", ctx, expectedStorageKey).Return(nil)
	mockLogger.On("Info", mock.AnythingOfType("string"), mock.AnythingOfType("map[string]interface {}")).Once()

	// Act
	err := service.Reset(ctx, key, limiterType)

	// Assert
	assert.NoError(t, err)
	mockStorage.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

// TestRateLimiterService_GetStatus testa obtenção de status
func TestRateLimiterService_GetStatus(t *testing.T) {
	// Arrange
	mockStorage := new(MockStorage)
	mockLogger := new(MockLogger)
	config := createTestConfig()

	service := NewRateLimiterService(mockStorage, config, mockLogger)

	ctx := context.Background()
	key := "192.168.1.1"
	limiterType := domain.IPLimiter
	expectedStorageKey := "rate_limit:ip:192.168.1.1"

	expectedStatus := &domain.RateLimitStatus{
		Key:       expectedStorageKey,
		Type:      limiterType,
		Count:     5,
		Limit:     10,
		Window:    60,
		LastReset: time.Now(),
		IsBlocked: false,
	}

	// Mock expectations
	mockStorage.On("Get", ctx, expectedStorageKey).Return(expectedStatus, nil)

	// Act
	status, err := service.GetStatus(ctx, key, limiterType)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, expectedStatus.Count, status.Count)
	assert.Equal(t, expectedStatus.Limit, status.Limit)
	mockStorage.AssertExpectations(t)
}

// TestRateLimiterService_IsAllowed testa verificação de permissão
func TestRateLimiterService_IsAllowed(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		limiterType  domain.LimiterType
		isBlocked    bool
		blockTime    *time.Time
		expectedAllowed bool
	}{
		{
			name:            "Should allow non-blocked key",
			key:             "192.168.1.1",
			limiterType:     domain.IPLimiter,
			isBlocked:       false,
			blockTime:       nil,
			expectedAllowed: true,
		},
		{
			name:            "Should not allow blocked key",
			key:             "192.168.1.2",
			limiterType:     domain.IPLimiter,
			isBlocked:       true,
			blockTime:       timePtr(time.Now().Add(time.Minute)),
			expectedAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockStorage := new(MockStorage)
			mockLogger := new(MockLogger)
			config := createTestConfig()

			service := NewRateLimiterService(mockStorage, config, mockLogger)

			ctx := context.Background()
			expectedStorageKey := buildStorageKey(tt.key, tt.limiterType)

			// Mock expectations
			mockStorage.On("IsBlocked", ctx, expectedStorageKey).Return(tt.isBlocked, tt.blockTime, nil)

			// Act
			allowed, err := service.IsAllowed(ctx, tt.key, tt.limiterType)

			// Assert
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedAllowed, allowed)
			mockStorage.AssertExpectations(t)
		})
	}
}

// Helper functions
func timePtr(t time.Time) *time.Time {
	return &t
}

func buildStorageKey(key string, limiterType domain.LimiterType) string {
	return fmt.Sprintf("rate_limit:%s:%s", limiterType, key)
}

// TestRateLimiterService_DetectLimiterType testa detecção automática do tipo
func TestRateLimiterService_DetectLimiterType(t *testing.T) {
	tests := []struct {
		name         string
		ip           string
		token        string
		expectedType domain.LimiterType
		expectedKey  string
	}{
		{
			name:         "Should detect token limiter when token provided",
			ip:           "192.168.1.1",
			token:        "abc123",
			expectedType: domain.TokenLimiter,
			expectedKey:  "abc123",
		},
		{
			name:         "Should detect IP limiter when no token",
			ip:           "192.168.1.1",
			token:        "",
			expectedType: domain.IPLimiter,
			expectedKey:  "192.168.1.1",
		},
		{
			name:         "Should detect IP limiter when empty token",
			ip:           "192.168.1.1",
			token:        " ",
			expectedType: domain.IPLimiter,
			expectedKey:  "192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockStorage := new(MockStorage)
			mockLogger := new(MockLogger)
			config := createTestConfig()

			service := NewRateLimiterService(mockStorage, config, mockLogger)

			// Act - usando um método público que internamente chama detectLimiterType
			ctx := context.Background()
			
			// Mock correto com tipos específicos
			mockStorage.On("IsBlocked", ctx, mock.AnythingOfType("string")).Return(false, (*time.Time)(nil), nil)
			mockStorage.On("Increment", ctx, mock.AnythingOfType("string"), mock.AnythingOfType("int"), mock.AnythingOfType("time.Duration")).Return(1, time.Now(), nil)
			mockLogger.On("Debug", mock.AnythingOfType("string"), mock.AnythingOfType("map[string]interface {}")).Maybe()

			result, _ := service.CheckLimit(ctx, tt.ip, tt.token)

			// Assert
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedType, result.LimiterType)
			
			mockStorage.AssertExpectations(t)
		})
	}
} 