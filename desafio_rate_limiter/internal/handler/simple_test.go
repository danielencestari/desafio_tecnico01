package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestHandlers_Basic testa funcionalidades básicas dos handlers
func TestHandlers_Basic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Health endpoint should return healthy status", func(t *testing.T) {
		// Arrange
		handlers := NewHandlers(nil, nil)
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
		assert.Equal(t, "1.0.0", response["version"])
	})

	t.Run("Admin status should validate parameters", func(t *testing.T) {
		// Arrange
		handlers := NewHandlers(nil, nil)
		router := gin.New()
		router.GET("/admin/status", handlers.AdminStatusHandler)

		testCases := []struct {
			name           string
			query          string
			expectedStatus int
			expectedError  string
		}{
			{
				name:           "Missing key parameter",
				query:          "?type=ip",
				expectedStatus: http.StatusBadRequest,
				expectedError:  "key parameter is required",
			},
			{
				name:           "Missing type parameter",
				query:          "?key=test",
				expectedStatus: http.StatusBadRequest,
				expectedError:  "type parameter is required",
			},
			{
				name:           "Invalid type parameter",
				query:          "?key=test&type=invalid",
				expectedStatus: http.StatusBadRequest,
				expectedError:  "type must be 'ip' or 'token'",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Act
				req := httptest.NewRequest("GET", "/admin/status"+tc.query, nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Assert
				assert.Equal(t, tc.expectedStatus, w.Code)
				
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response["message"], tc.expectedError)
			})
		}
	})

	t.Run("Admin reset should validate JSON body", func(t *testing.T) {
		// Arrange
		handlers := NewHandlers(nil, nil)
		router := gin.New()
		router.POST("/admin/reset", handlers.AdminResetHandler)

		testCases := []struct {
			name           string
			body           string
			expectedStatus int
		}{
			{
				name:           "Invalid JSON",
				body:           `{"invalid": json}`,
				expectedStatus: http.StatusBadRequest,
			},
			{
				name:           "Missing key field",
				body:           `{"type": "ip"}`,
				expectedStatus: http.StatusBadRequest,
			},
			{
				name:           "Missing type field",
				body:           `{"key": "test"}`,
				expectedStatus: http.StatusBadRequest,
			},
			{
				name:           "Invalid type value",
				body:           `{"key": "test", "type": "invalid"}`,
				expectedStatus: http.StatusBadRequest,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Act
				req := httptest.NewRequest("POST", "/admin/reset", bytes.NewBufferString(tc.body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Assert
				assert.Equal(t, tc.expectedStatus, w.Code)
			})
		}
	})
}

// TestMaskToken testa a função de mascaramento de tokens
func TestMaskToken(t *testing.T) {
	handlers := &Handlers{}

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Empty token",
			input:    "",
			expected: "",
		},
		{
			name:     "Short token",
			input:    "abc",
			expected: "abc***",
		},
		{
			name:     "Long token",
			input:    "abcd1234567890token",
			expected: "abcd1234***",
		},
		{
			name:     "Exactly 8 chars",
			input:    "12345678",
			expected: "12345678***",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := handlers.maskToken(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestFormatBytes testa a função de formatação de bytes
func TestFormatBytes(t *testing.T) {
	testCases := []struct {
		name     string
		input    uint64
		expected string
	}{
		{
			name:     "Bytes",
			input:    512,
			expected: "512 B",
		},
		{
			name:     "Kilobytes",
			input:    1536, // 1.5 KB
			expected: "1.5 KB",
		},
		{
			name:     "Megabytes",
			input:    1572864, // 1.5 MB
			expected: "1.5 MB",
		},
		{
			name:     "Zero bytes",
			input:    0,
			expected: "0 B",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := formatBytes(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
} 