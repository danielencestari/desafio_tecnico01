package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"rate-limiter/internal/domain"

	"github.com/joho/godotenv"
)

// Config representa todas as configurações da aplicação
type Config struct {
	// Redis Configuration
	RedisHost     string
	RedisPort     string
	RedisPassword string
	RedisDB       int

	// Rate Limiting Configuration
	DefaultIPLimit    int
	DefaultTokenLimit int
	RateWindow        int // em segundos
	BlockDuration     int // em segundos

	// Server Configuration
	ServerPort string
	GinMode    string

	// Logging Configuration
	LogLevel  string
	LogFormat string

	// Token Configuration File
	TokenConfigFile string
}

// TokensFile representa a estrutura do arquivo tokens.json
type TokensFile struct {
	Tokens map[string]domain.TokenConfig `json:"tokens"`
}

// ConfigLoader implementa a interface domain.ConfigLoader
type ConfigLoader struct {
	config      *Config
	tokenConfigs map[string]domain.TokenConfig
}

// NewConfigLoader cria uma nova instância do ConfigLoader
func NewConfigLoader() *ConfigLoader {
	return &ConfigLoader{
		tokenConfigs: make(map[string]domain.TokenConfig),
	}
}

// LoadConfig carrega as configurações do .env
func (c *ConfigLoader) LoadConfig() (*domain.RateLimitConfig, error) {
	// Carrega o arquivo .env se existir
	if err := godotenv.Load(); err != nil {
		// Se não encontrar .env, continua com variáveis do sistema
		fmt.Println("Warning: .env file not found, using system environment variables")
	}

	// Carrega configurações do ambiente
	config, err := c.loadFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to load environment config: %w", err)
	}

	c.config = config

	// Carrega configurações de tokens
	tokenConfigs, err := c.LoadTokenConfigs()
	if err != nil {
		return nil, fmt.Errorf("failed to load token configs: %w", err)
	}

	// Cria a configuração do rate limiter
	rateLimitConfig := &domain.RateLimitConfig{
		DefaultIPLimit:    config.DefaultIPLimit,
		DefaultTokenLimit: config.DefaultTokenLimit,
		Window:           config.RateWindow,
		BlockDuration:    config.BlockDuration,
		TokenConfigs:     tokenConfigs,
	}

	return rateLimitConfig, nil
}

// LoadTokenConfigs carrega as configurações de tokens do arquivo JSON
func (c *ConfigLoader) LoadTokenConfigs() (map[string]domain.TokenConfig, error) {
	tokenFile := c.getTokenConfigFile()
	
	// Verifica se o arquivo existe
	if _, err := os.Stat(tokenFile); os.IsNotExist(err) {
		fmt.Printf("Warning: Token config file %s not found, using only environment defaults\n", tokenFile)
		return make(map[string]domain.TokenConfig), nil
	}

	// Lê o arquivo
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read token config file: %w", err)
	}

	// Parse do JSON
	var tokensFile TokensFile
	if err := json.Unmarshal(data, &tokensFile); err != nil {
		return nil, fmt.Errorf("failed to parse token config file: %w", err)
	}

	// Valida as configurações de tokens
	for token, config := range tokensFile.Tokens {
		if config.Limit <= 0 {
			return nil, fmt.Errorf("invalid token limit for token %s: must be greater than 0", token)
		}
		// Adiciona o token à configuração se não estiver presente
		if config.Token == "" {
			config.Token = token
			tokensFile.Tokens[token] = config
		}
	}

	c.tokenConfigs = tokensFile.Tokens
	return tokensFile.Tokens, nil
}

// Reload recarrega todas as configurações
func (c *ConfigLoader) Reload() error {
	_, err := c.LoadConfig()
	return err
}

// GetConfig retorna a configuração atual
func (c *ConfigLoader) GetConfig() *Config {
	return c.config
}

// GetTokenConfig retorna a configuração de um token específico
func (c *ConfigLoader) GetTokenConfig(token string) (domain.TokenConfig, bool) {
	config, exists := c.tokenConfigs[token]
	return config, exists
}

// loadFromEnv carrega configurações das variáveis de ambiente
func (c *ConfigLoader) loadFromEnv() (*Config, error) {
	config := &Config{
		// Redis defaults
		RedisHost:     getEnvWithDefault("REDIS_HOST", "localhost"),
		RedisPort:     getEnvWithDefault("REDIS_PORT", "6379"),
		RedisPassword: getEnvWithDefault("REDIS_PASSWORD", ""),
		
		// Server defaults
		ServerPort: getEnvWithDefault("SERVER_PORT", "8080"),
		GinMode:    getEnvWithDefault("GIN_MODE", "debug"),
		
		// Logging defaults
		LogLevel:  getEnvWithDefault("LOG_LEVEL", "info"),
		LogFormat: getEnvWithDefault("LOG_FORMAT", "json"),
		
		// Token config file
		TokenConfigFile: getEnvWithDefault("TOKEN_CONFIG_FILE", "internal/config/tokens.json"),
	}

	// Parse Redis DB
	redisDB, err := strconv.Atoi(getEnvWithDefault("REDIS_DB", "0"))
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_DB value: %w", err)
	}
	config.RedisDB = redisDB

	// Parse rate limiting configuration
	defaultIPLimit, err := strconv.Atoi(getEnvWithDefault("DEFAULT_IP_LIMIT", "10"))
	if err != nil {
		return nil, fmt.Errorf("invalid DEFAULT_IP_LIMIT value: %w", err)
	}
	config.DefaultIPLimit = defaultIPLimit

	defaultTokenLimit, err := strconv.Atoi(getEnvWithDefault("DEFAULT_TOKEN_LIMIT", "100"))
	if err != nil {
		return nil, fmt.Errorf("invalid DEFAULT_TOKEN_LIMIT value: %w", err)
	}
	config.DefaultTokenLimit = defaultTokenLimit

	rateWindow, err := strconv.Atoi(getEnvWithDefault("RATE_WINDOW", "60"))
	if err != nil {
		return nil, fmt.Errorf("invalid RATE_WINDOW value: %w", err)
	}
	config.RateWindow = rateWindow

	blockDuration, err := strconv.Atoi(getEnvWithDefault("BLOCK_DURATION", "180"))
	if err != nil {
		return nil, fmt.Errorf("invalid BLOCK_DURATION value: %w", err)
	}
	config.BlockDuration = blockDuration

	// Valida configurações obrigatórias
	if err := c.validateConfig(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

// validateConfig valida se as configurações são válidas
func (c *ConfigLoader) validateConfig(config *Config) error {
	if config.DefaultIPLimit <= 0 {
		return fmt.Errorf("DEFAULT_IP_LIMIT must be greater than 0")
	}
	
	if config.DefaultTokenLimit <= 0 {
		return fmt.Errorf("DEFAULT_TOKEN_LIMIT must be greater than 0")
	}
	
	if config.RateWindow <= 0 {
		return fmt.Errorf("RATE_WINDOW must be greater than 0")
	}
	
	if config.BlockDuration <= 0 {
		return fmt.Errorf("BLOCK_DURATION must be greater than 0")
	}

	if config.RedisDB < 0 || config.RedisDB > 15 {
		return fmt.Errorf("REDIS_DB must be between 0 and 15")
	}

	return nil
}

// getTokenConfigFile retorna o caminho do arquivo de configuração de tokens
func (c *ConfigLoader) getTokenConfigFile() string {
	if c.config != nil && c.config.TokenConfigFile != "" {
		return c.config.TokenConfigFile
	}
	return getEnvWithDefault("TOKEN_CONFIG_FILE", "internal/config/tokens.json")
}

// getEnvWithDefault retorna o valor da variável de ambiente ou um valor padrão
func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
} 