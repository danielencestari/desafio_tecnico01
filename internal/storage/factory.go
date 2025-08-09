package storage

import (
	"fmt"
	"strings"

	"rate-limiter/internal/domain"
)

// StorageType define os tipos de storage disponíveis
type StorageType string

const (
	RedisStorageType  StorageType = "redis"
	MemoryStorageType StorageType = "memory"
)

// StorageConfig contém configurações para criação de storage
type StorageConfig struct {
	Type     StorageType
	RedisConfig *RedisConfig
}

// RedisConfig contém configurações específicas do Redis
type RedisConfig struct {
	Host     string
	Port     string
	Password string
	Database int
}

// StorageFactory cria instâncias de storage seguindo Strategy Pattern
type StorageFactory struct{}

// NewStorageFactory cria uma nova instância da factory
func NewStorageFactory() *StorageFactory {
	return &StorageFactory{}
}

// CreateStorage cria uma instância de storage baseada na configuração
func (f *StorageFactory) CreateStorage(config *StorageConfig, logger domain.Logger) (domain.RateLimiterStorage, error) {
	if config == nil {
		return nil, fmt.Errorf("storage config cannot be nil")
	}

	switch strings.ToLower(string(config.Type)) {
	case string(RedisStorageType):
		return f.createRedisStorage(config.RedisConfig, logger)
	case string(MemoryStorageType):
		return f.createMemoryStorage(logger)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", config.Type)
	}
}

// createRedisStorage cria uma instância de Redis storage
func (f *StorageFactory) createRedisStorage(config *RedisConfig, logger domain.Logger) (domain.RateLimiterStorage, error) {
	if config == nil {
		return nil, fmt.Errorf("Redis config cannot be nil")
	}

	// Validações básicas
	if config.Host == "" {
		return nil, fmt.Errorf("Redis host cannot be empty")
	}
	if config.Port == "" {
		return nil, fmt.Errorf("Redis port cannot be empty")
	}
	if config.Database < 0 || config.Database > 15 {
		return nil, fmt.Errorf("Redis database must be between 0 and 15")
	}

	storage, err := NewRedisStorage(config.Host, config.Port, config.Password, config.Database, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create Redis storage: %w", err)
	}

	if logger != nil {
		logger.Info("Redis storage created successfully", map[string]interface{}{
			"host":     config.Host,
			"port":     config.Port,
			"database": config.Database,
		})
	}

	return storage, nil
}

// createMemoryStorage cria uma instância de Memory storage
func (f *StorageFactory) createMemoryStorage(logger domain.Logger) (domain.RateLimiterStorage, error) {
	storage := NewMemoryStorage(logger)

	if logger != nil {
		logger.Info("Memory storage created successfully", nil)
	}

	return storage, nil
}

// GetSupportedTypes retorna os tipos de storage suportados
func (f *StorageFactory) GetSupportedTypes() []StorageType {
	return []StorageType{RedisStorageType, MemoryStorageType}
}

// ValidateConfig valida uma configuração de storage
func (f *StorageFactory) ValidateConfig(config *StorageConfig) error {
	if config == nil {
		return fmt.Errorf("storage config cannot be nil")
	}

	switch strings.ToLower(string(config.Type)) {
	case string(RedisStorageType):
		return f.validateRedisConfig(config.RedisConfig)
	case string(MemoryStorageType):
		// Memory storage não precisa de configurações específicas
		return nil
	default:
		return fmt.Errorf("unsupported storage type: %s", config.Type)
	}
}

// validateRedisConfig valida configuração do Redis
func (f *StorageFactory) validateRedisConfig(config *RedisConfig) error {
	if config == nil {
		return fmt.Errorf("Redis config cannot be nil")
	}

	if config.Host == "" {
		return fmt.Errorf("Redis host cannot be empty")
	}

	if config.Port == "" {
		return fmt.Errorf("Redis port cannot be empty")
	}

	if config.Database < 0 || config.Database > 15 {
		return fmt.Errorf("Redis database must be between 0 and 15, got: %d", config.Database)
	}

	return nil
}

// BuildStorageConfigFromEnv constrói configuração de storage a partir de variáveis de ambiente
func BuildStorageConfigFromEnv(storageType, redisHost, redisPort, redisPassword string, redisDB int) *StorageConfig {
	config := &StorageConfig{
		Type: StorageType(strings.ToLower(storageType)),
	}

	if config.Type == RedisStorageType {
		config.RedisConfig = &RedisConfig{
			Host:     redisHost,
			Port:     redisPort,
			Password: redisPassword,
			Database: redisDB,
		}
	}

	return config
}

// CreateDefaultMemoryStorage cria um storage em memória com configuração padrão
func CreateDefaultMemoryStorage(logger domain.Logger) domain.RateLimiterStorage {
	return NewMemoryStorage(logger)
}

// CreateDefaultRedisStorage cria um storage Redis com configuração padrão
func CreateDefaultRedisStorage(logger domain.Logger) (domain.RateLimiterStorage, error) {
	config := &RedisConfig{
		Host:     "localhost",
		Port:     "6379",
		Password: "",
		Database: 0,
	}

	return NewRedisStorage(config.Host, config.Port, config.Password, config.Database, logger)
} 