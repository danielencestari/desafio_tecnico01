package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigLoader_LoadConfig(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		expectError bool
		expectedIP  int
		expectedToken int
		expectedWindow int
		expectedBlock int
	}{
		{
			name: "Default values",
			envVars: map[string]string{},
			expectError: false,
			expectedIP: 10,
			expectedToken: 100,
			expectedWindow: 60,
			expectedBlock: 180,
		},
		{
			name: "Custom values",
			envVars: map[string]string{
				"DEFAULT_IP_LIMIT": "5",
				"DEFAULT_TOKEN_LIMIT": "50",
				"RATE_WINDOW": "30",
				"BLOCK_DURATION": "300",
			},
			expectError: false,
			expectedIP: 5,
			expectedToken: 50,
			expectedWindow: 30,
			expectedBlock: 300,
		},
		{
			name: "Invalid IP limit",
			envVars: map[string]string{
				"DEFAULT_IP_LIMIT": "0",
			},
			expectError: true,
		},
		{
			name: "Invalid token limit",
			envVars: map[string]string{
				"DEFAULT_TOKEN_LIMIT": "-1",
			},
			expectError: true,
		},
		{
			name: "Invalid window",
			envVars: map[string]string{
				"RATE_WINDOW": "0",
			},
			expectError: true,
		},
		{
			name: "Invalid block duration",
			envVars: map[string]string{
				"BLOCK_DURATION": "-1",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			// Cleanup after test
			defer func() {
				for key := range tt.envVars {
					os.Unsetenv(key)
				}
			}()

			loader := NewConfigLoader()
			config, err := loader.LoadConfig()

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, config)
			} else {
				require.NoError(t, err)
				require.NotNil(t, config)
				
				assert.Equal(t, tt.expectedIP, config.DefaultIPLimit)
				assert.Equal(t, tt.expectedToken, config.DefaultTokenLimit)
				assert.Equal(t, tt.expectedWindow, config.Window)
				assert.Equal(t, tt.expectedBlock, config.BlockDuration)
			}
		})
	}
}

func TestConfigLoader_LoadTokenConfigs(t *testing.T) {
	// Create a temporary token config file
	tmpFile := "/tmp/test_tokens.json"
	tokenData := `{
		"tokens": {
			"test-token-1": {
				"limit": 100,
				"description": "Test token 1"
			},
			"test-token-2": {
				"limit": 200,
				"description": "Test token 2"
			}
		}
	}`

	err := os.WriteFile(tmpFile, []byte(tokenData), 0644)
	require.NoError(t, err)
	defer os.Remove(tmpFile)

	// Set environment variable to point to test file
	os.Setenv("TOKEN_CONFIG_FILE", tmpFile)
	defer os.Unsetenv("TOKEN_CONFIG_FILE")

	loader := NewConfigLoader()
	
	// Load basic config first
	_, err = loader.LoadConfig()
	require.NoError(t, err)

	// Test token config loading
	tokenConfigs, err := loader.LoadTokenConfigs()
	require.NoError(t, err)
	
	assert.Len(t, tokenConfigs, 2)
	
	token1, exists := tokenConfigs["test-token-1"]
	assert.True(t, exists)
	assert.Equal(t, 100, token1.Limit)
	assert.Equal(t, "Test token 1", token1.Description)
	assert.Equal(t, "test-token-1", token1.Token)

	token2, exists := tokenConfigs["test-token-2"]
	assert.True(t, exists)
	assert.Equal(t, 200, token2.Limit)
	assert.Equal(t, "Test token 2", token2.Description)
	assert.Equal(t, "test-token-2", token2.Token)
}

func TestConfigLoader_LoadTokenConfigs_FileNotFound(t *testing.T) {
	// Set non-existent file
	os.Setenv("TOKEN_CONFIG_FILE", "/tmp/non_existent_tokens.json")
	defer os.Unsetenv("TOKEN_CONFIG_FILE")

	loader := NewConfigLoader()
	
	// Should not error when file doesn't exist, just return empty map
	tokenConfigs, err := loader.LoadTokenConfigs()
	require.NoError(t, err)
	assert.Empty(t, tokenConfigs)
}

func TestConfigLoader_LoadTokenConfigs_InvalidJSON(t *testing.T) {
	// Create invalid JSON file
	tmpFile := "/tmp/invalid_tokens.json"
	invalidData := `{"tokens": {"test": invalid json}}`

	err := os.WriteFile(tmpFile, []byte(invalidData), 0644)
	require.NoError(t, err)
	defer os.Remove(tmpFile)

	os.Setenv("TOKEN_CONFIG_FILE", tmpFile)
	defer os.Unsetenv("TOKEN_CONFIG_FILE")

	loader := NewConfigLoader()
	
	// Should error on invalid JSON
	_, err = loader.LoadTokenConfigs()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse token config file")
}

func TestConfigLoader_ValidateConfig(t *testing.T) {
	loader := NewConfigLoader()

	tests := []struct {
		name        string
		config      *Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid config",
			config: &Config{
				DefaultIPLimit:    10,
				DefaultTokenLimit: 100,
				RateWindow:        60,
				BlockDuration:     180,
				RedisDB:          0,
			},
			expectError: false,
		},
		{
			name: "Invalid IP limit",
			config: &Config{
				DefaultIPLimit:    0,
				DefaultTokenLimit: 100,
				RateWindow:        60,
				BlockDuration:     180,
				RedisDB:          0,
			},
			expectError: true,
			errorMsg:    "DEFAULT_IP_LIMIT must be greater than 0",
		},
		{
			name: "Invalid Redis DB",
			config: &Config{
				DefaultIPLimit:    10,
				DefaultTokenLimit: 100,
				RateWindow:        60,
				BlockDuration:     180,
				RedisDB:          16,
			},
			expectError: true,
			errorMsg:    "REDIS_DB must be between 0 and 15",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := loader.validateConfig(tt.config)
			
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetEnvWithDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		expected     string
	}{
		{
			name:         "Environment variable exists",
			key:          "TEST_VAR",
			defaultValue: "default",
			envValue:     "custom",
			expected:     "custom",
		},
		{
			name:         "Environment variable does not exist",
			key:          "NON_EXISTENT_VAR",
			defaultValue: "default",
			envValue:     "",
			expected:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			result := getEnvWithDefault(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
} 