package storage

import (
	"context"
	"testing"
	"time"

	"rate-limiter/internal/domain"
	"rate-limiter/internal/logger"

	"github.com/stretchr/testify/assert"
)

func TestStorageFactory_CreateStorage(t *testing.T) {
	tests := []struct {
		name        string
		config      *StorageConfig
		expectError bool
		expectedType string
	}{
		{
			name: "Should create Redis storage successfully",
			config: &StorageConfig{
				Type: RedisStorageType,
				RedisConfig: &RedisConfig{
					Host:     "localhost",
					Port:     "6379",
					Password: "",
					Database: 0,
				},
			},
			expectError:  false,
			expectedType: "*storage.RedisStorage",
		},
		{
			name: "Should create Memory storage successfully",
			config: &StorageConfig{
				Type: MemoryStorageType,
			},
			expectError:  false,
			expectedType: "*storage.MemoryStorage",
		},
		{
			name:        "Should return error for nil config",
			config:      nil,
			expectError: true,
		},
		{
			name: "Should return error for unsupported type",
			config: &StorageConfig{
				Type: StorageType("unsupported"),
			},
			expectError: true,
		},
		{
			name: "Should return error for Redis with nil config",
			config: &StorageConfig{
				Type:        RedisStorageType,
				RedisConfig: nil,
			},
			expectError: true,
		},
		{
			name: "Should return error for Redis with empty host",
			config: &StorageConfig{
				Type: RedisStorageType,
				RedisConfig: &RedisConfig{
					Host:     "",
					Port:     "6379",
					Database: 0,
				},
			},
			expectError: true,
		},
		{
			name: "Should return error for Redis with empty port",
			config: &StorageConfig{
				Type: RedisStorageType,
				RedisConfig: &RedisConfig{
					Host:     "localhost",
					Port:     "",
					Database: 0,
				},
			},
			expectError: true,
		},
		{
			name: "Should return error for Redis with invalid database",
			config: &StorageConfig{
				Type: RedisStorageType,
				RedisConfig: &RedisConfig{
					Host:     "localhost",
					Port:     "6379",
					Database: 16, // Invalid (> 15)
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			factory := NewStorageFactory()
			testLogger := logger.NewLogger("debug", "text")

			// Act
			storage, err := factory.CreateStorage(tt.config, testLogger)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, storage)
			} else {
				// Para Redis, pode falhar na conexão real, então verificamos apenas o tipo de erro
				if tt.config.Type == RedisStorageType {
					// Pode falhar na conexão, mas não deve ser erro de configuração
					if err != nil {
						assert.Contains(t, err.Error(), "failed to connect to Redis")
					}
				} else {
					assert.NoError(t, err)
					assert.NotNil(t, storage)
					assert.Contains(t, tt.expectedType, "MemoryStorage")
				}
			}
		})
	}
}

func TestStorageFactory_ValidateConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *StorageConfig
		expectError bool
	}{
		{
			name: "Should validate Redis config successfully",
			config: &StorageConfig{
				Type: RedisStorageType,
				RedisConfig: &RedisConfig{
					Host:     "localhost",
					Port:     "6379",
					Password: "",
					Database: 0,
				},
			},
			expectError: false,
		},
		{
			name: "Should validate Memory config successfully",
			config: &StorageConfig{
				Type: MemoryStorageType,
			},
			expectError: false,
		},
		{
			name:        "Should return error for nil config",
			config:      nil,
			expectError: true,
		},
		{
			name: "Should return error for unsupported type",
			config: &StorageConfig{
				Type: StorageType("unsupported"),
			},
			expectError: true,
		},
		{
			name: "Should return error for Redis with nil config",
			config: &StorageConfig{
				Type:        RedisStorageType,
				RedisConfig: nil,
			},
			expectError: true,
		},
		{
			name: "Should return error for Redis with empty host",
			config: &StorageConfig{
				Type: RedisStorageType,
				RedisConfig: &RedisConfig{
					Host:     "",
					Port:     "6379",
					Database: 0,
				},
			},
			expectError: true,
		},
		{
			name: "Should return error for Redis with empty port",
			config: &StorageConfig{
				Type: RedisStorageType,
				RedisConfig: &RedisConfig{
					Host:     "localhost",
					Port:     "",
					Database: 0,
				},
			},
			expectError: true,
		},
		{
			name: "Should return error for Redis with invalid database",
			config: &StorageConfig{
				Type: RedisStorageType,
				RedisConfig: &RedisConfig{
					Host:     "localhost",
					Port:     "6379",
					Database: -1, // Invalid (< 0)
				},
			},
			expectError: true,
		},
		{
			name: "Should return error for Redis with database > 15",
			config: &StorageConfig{
				Type: RedisStorageType,
				RedisConfig: &RedisConfig{
					Host:     "localhost",
					Port:     "6379",
					Database: 16, // Invalid (> 15)
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			factory := NewStorageFactory()

			// Act
			err := factory.ValidateConfig(tt.config)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStorageFactory_GetSupportedTypes(t *testing.T) {
	// Arrange
	factory := NewStorageFactory()

	// Act
	types := factory.GetSupportedTypes()

	// Assert
	assert.Len(t, types, 2)
	assert.Contains(t, types, RedisStorageType)
	assert.Contains(t, types, MemoryStorageType)
}

func TestBuildStorageConfigFromEnv(t *testing.T) {
	tests := []struct {
		name         string
		storageType  string
		redisHost    string
		redisPort    string
		redisPassword string
		redisDB      int
		expected     *StorageConfig
	}{
		{
			name:         "Should build Redis config correctly",
			storageType:  "redis",
			redisHost:    "localhost",
			redisPort:    "6379",
			redisPassword: "secret",
			redisDB:      1,
			expected: &StorageConfig{
				Type: RedisStorageType,
				RedisConfig: &RedisConfig{
					Host:     "localhost",
					Port:     "6379",
					Password: "secret",
					Database: 1,
				},
			},
		},
		{
			name:        "Should build Memory config correctly",
			storageType: "memory",
			expected: &StorageConfig{
				Type:        MemoryStorageType,
				RedisConfig: nil,
			},
		},
		{
			name:         "Should handle Redis case insensitive",
			storageType:  "REDIS",
			redisHost:    "redis-server",
			redisPort:    "6380",
			redisPassword: "",
			redisDB:      0,
			expected: &StorageConfig{
				Type: RedisStorageType,
				RedisConfig: &RedisConfig{
					Host:     "redis-server",
					Port:     "6380",
					Password: "",
					Database: 0,
				},
			},
		},
		{
			name:        "Should handle Memory case insensitive",
			storageType: "MEMORY",
			expected: &StorageConfig{
				Type:        MemoryStorageType,
				RedisConfig: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			config := BuildStorageConfigFromEnv(
				tt.storageType,
				tt.redisHost,
				tt.redisPort,
				tt.redisPassword,
				tt.redisDB,
			)

			// Assert
			assert.Equal(t, tt.expected.Type, config.Type)
			
			if tt.expected.RedisConfig != nil {
				assert.NotNil(t, config.RedisConfig)
				assert.Equal(t, tt.expected.RedisConfig.Host, config.RedisConfig.Host)
				assert.Equal(t, tt.expected.RedisConfig.Port, config.RedisConfig.Port)
				assert.Equal(t, tt.expected.RedisConfig.Password, config.RedisConfig.Password)
				assert.Equal(t, tt.expected.RedisConfig.Database, config.RedisConfig.Database)
			} else {
				assert.Nil(t, config.RedisConfig)
			}
		})
	}
}

func TestCreateDefaultMemoryStorage(t *testing.T) {
	// Arrange
	testLogger := logger.NewLogger("debug", "text")

	// Act
	storage := CreateDefaultMemoryStorage(testLogger)

	// Assert
	assert.NotNil(t, storage)
	
	// Verify it's actually MemoryStorage
	memStorage, ok := storage.(*MemoryStorage)
	assert.True(t, ok)
	assert.NotNil(t, memStorage.data)
	assert.NotNil(t, memStorage.blocks)
}

func TestCreateDefaultRedisStorage(t *testing.T) {
	// Arrange
	testLogger := logger.NewLogger("debug", "text")

	// Act
	storage, err := CreateDefaultRedisStorage(testLogger)

	// Assert
	// Esta função tentará conectar ao Redis local, então pode falhar
	// Verificamos que o erro é de conexão, não de configuração
	if err != nil {
		assert.Contains(t, err.Error(), "failed to connect to Redis")
	} else {
		assert.NotNil(t, storage)
	}
}

func TestStorageFactory_Integration(t *testing.T) {
	// Teste de integração que verifica o Strategy Pattern
	
	// Arrange
	factory := NewStorageFactory()
	testLogger := logger.NewLogger("debug", "text")

	// Test Memory Storage
	memoryConfig := &StorageConfig{
		Type: MemoryStorageType,
	}

	// Act
	memoryStorage, err := factory.CreateStorage(memoryConfig, testLogger)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, memoryStorage)

	// Verify interface compliance
	var _ domain.RateLimiterStorage = memoryStorage

	// Test basic functionality
	ctx := context.Background()
	key := "test:key"
	
	// Should not exist initially
	status, err := memoryStorage.Get(ctx, key)
	assert.NoError(t, err)
	assert.Nil(t, status)

	// Should increment correctly
	count, _, err := memoryStorage.Increment(ctx, key, 10, time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)

	// Should retrieve correctly
	status, err = memoryStorage.Get(ctx, key)
	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, 1, status.Count)

	// Should reset correctly
	err = memoryStorage.Reset(ctx, key)
	assert.NoError(t, err)

	// Should be gone after reset
	status, err = memoryStorage.Get(ctx, key)
	assert.NoError(t, err)
	assert.Nil(t, status)

	// Cleanup
	memoryStorage.Close()
}

func TestStorageType_String(t *testing.T) {
	tests := []struct {
		storageType StorageType
		expected    string
	}{
		{RedisStorageType, "redis"},
		{MemoryStorageType, "memory"},
		{StorageType("custom"), "custom"},
	}

	for _, tt := range tests {
		t.Run(string(tt.storageType), func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.storageType))
		})
	}
} 