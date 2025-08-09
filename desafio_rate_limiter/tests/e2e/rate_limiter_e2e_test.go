package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rate-limiter/internal/config"
	"rate-limiter/internal/handler"
	"rate-limiter/internal/logger"
	"rate-limiter/internal/service"
	"rate-limiter/internal/storage"
)

// E2ETestSuite contém os componentes necessários para os testes E2E
type E2ETestSuite struct {
	router     *gin.Engine
	server     *httptest.Server
	configFile string
	tokenFile  string
}

// setupE2ETest configura um ambiente completo para testes E2E
func setupE2ETest(t *testing.T) *E2ETestSuite {
	// Configurar Gin para teste
	gin.SetMode(gin.TestMode)

	// Carregar configurações
	configLoader := config.NewConfigLoader()
	cfg, err := configLoader.LoadConfig()
	require.NoError(t, err)

	// Inicializar logger
	appLogger := logger.NewLogger("debug", "json")

	// Usar MemoryStorage para testes (isolamento)
	rateLimiterStorage := storage.NewMemoryStorage(appLogger)

	// Inicializar service
	rateLimiterService := service.NewRateLimiterService(rateLimiterStorage, cfg, appLogger)

	// Inicializar handlers
	handlers := handler.NewHandlers(rateLimiterService, appLogger)

	// Criar router
	router := gin.New()
	router.Use(gin.Recovery())
	handlers.SetupRoutes(router)

	// Criar servidor de teste
	server := httptest.NewServer(router)

	return &E2ETestSuite{
		router: router,
		server: server,
	}
}

// teardownE2ETest limpa os recursos do teste E2E
func (suite *E2ETestSuite) teardownE2ETest() {
	if suite.server != nil {
		suite.server.Close()
	}
}

// TestE2E_RateLimiter_BasicFunctionality testa a funcionalidade básica do rate limiter
func TestE2E_RateLimiter_BasicFunctionality(t *testing.T) {
	suite := setupE2ETest(t)
	defer suite.teardownE2ETest()

	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("Health endpoint should be accessible", func(t *testing.T) {
		resp, err := client.Get(suite.server.URL + "/health")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, "healthy", response["status"])
		assert.Equal(t, "Rate Limiter API", response["service"])
	})

	t.Run("Metrics endpoint should return system info", func(t *testing.T) {
		resp, err := client.Get(suite.server.URL + "/metrics")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, "Rate Limiter API", response["service"])
		assert.Contains(t, response, "uptime")
		assert.Contains(t, response, "system")
	})
}

// TestE2E_RateLimiter_IPLimiting testa limitação por IP
func TestE2E_RateLimiter_IPLimiting(t *testing.T) {
	suite := setupE2ETest(t)
	defer suite.teardownE2ETest()

	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("Should allow requests within IP limit", func(t *testing.T) {
		// Fazer algumas requisições normais (dentro do limite)
		for i := 0; i < 5; i++ {
			req, err := http.NewRequest("GET", suite.server.URL+"/", nil)
			require.NoError(t, err)
			
			// Simular IP específico
			req.Header.Set("X-Forwarded-For", "192.168.1.100")
			
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			
			// Verificar headers de rate limiting
			assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Limit"))
			assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Remaining"))
		}
	})

	t.Run("Should block requests when IP exceeds limit", func(t *testing.T) {
		// Fazer muitas requisições para exceder o limite (padrão é 10)
		var lastResp *http.Response
		for i := 0; i < 15; i++ {
			req, err := http.NewRequest("GET", suite.server.URL+"/", nil)
			require.NoError(t, err)
			
			// Simular IP específico diferente do teste anterior
			req.Header.Set("X-Forwarded-For", "192.168.1.101")
			
			resp, err := client.Do(req)
			require.NoError(t, err)
			
			if i < 10 {
				// Primeiras requisições devem passar
				assert.Equal(t, http.StatusOK, resp.StatusCode, "Request %d should be allowed", i+1)
			}
			
			if resp.StatusCode == http.StatusTooManyRequests {
				// Primeira requisição bloqueada
				assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
				
				// Verificar headers de rate limiting
				assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Limit"))
				assert.Equal(t, "0", resp.Header.Get("X-RateLimit-Remaining"))
				assert.NotEmpty(t, resp.Header.Get("Retry-After"))
				
				// Verificar mensagem específica
				var response map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&response)
				require.NoError(t, err)
				
				expectedMessage := "you have reached the maximum number of requests or actions allowed within a certain time frame"
				assert.Equal(t, expectedMessage, response["message"])
				
				lastResp = resp
				break
			}
			
			resp.Body.Close()
		}
		
		// Deve ter encontrado uma resposta 429
		require.NotNil(t, lastResp, "Should have received a 429 response")
		lastResp.Body.Close()
	})
}

// TestE2E_RateLimiter_TokenLimiting testa limitação por token
func TestE2E_RateLimiter_TokenLimiting(t *testing.T) {
	suite := setupE2ETest(t)
	defer suite.teardownE2ETest()

	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("Should allow requests with valid token", func(t *testing.T) {
		// Fazer requisições com token
		for i := 0; i < 5; i++ {
			req, err := http.NewRequest("GET", suite.server.URL+"/", nil)
			require.NoError(t, err)
			
			// Simular token válido
			req.Header.Set("X-Api-Token", "premium_token_123")
			req.Header.Set("X-Forwarded-For", "192.168.2.100") // IP diferente
			
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			
			// Verificar headers de rate limiting
			assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Limit"))
			assert.Equal(t, "token", resp.Header.Get("X-RateLimit-Type"))
		}
	})

	t.Run("Token should have priority over IP limiting", func(t *testing.T) {
		// Primeiro, esgotar limite de IP
		for i := 0; i < 12; i++ {
			req, err := http.NewRequest("GET", suite.server.URL+"/", nil)
			require.NoError(t, err)
			req.Header.Set("X-Forwarded-For", "192.168.2.200")
			
			resp, err := client.Do(req)
			require.NoError(t, err)
			resp.Body.Close()
		}

		// Agora fazer requisição com mesmo IP mas com token
		req, err := http.NewRequest("GET", suite.server.URL+"/", nil)
		require.NoError(t, err)
		req.Header.Set("X-Forwarded-For", "192.168.2.200") // Mesmo IP limitado
		req.Header.Set("X-Api-Token", "premium_token_456")  // Com token

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Deve passar porque token tem prioridade
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "token", resp.Header.Get("X-RateLimit-Type"))
	})
}

// TestE2E_RateLimiter_AdminEndpoints testa endpoints administrativos
func TestE2E_RateLimiter_AdminEndpoints(t *testing.T) {
	suite := setupE2ETest(t)
	defer suite.teardownE2ETest()

	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("Admin status should work for IP", func(t *testing.T) {
		// Primeiro fazer algumas requisições para criar dados
		for i := 0; i < 3; i++ {
			req, err := http.NewRequest("GET", suite.server.URL+"/", nil)
			require.NoError(t, err)
			req.Header.Set("X-Forwarded-For", "192.168.3.100")
			
			resp, err := client.Do(req)
			require.NoError(t, err)
			resp.Body.Close()
		}

		// Verificar status
		resp, err := client.Get(suite.server.URL + "/admin/status?key=192.168.3.100&type=ip")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		assert.Contains(t, response, "limit")
		assert.Contains(t, response, "current")
		assert.Contains(t, response, "remaining")
		assert.Equal(t, "ip", response["limiter_type"])
	})

	t.Run("Admin reset should work", func(t *testing.T) {
		// Primeiro exceder limite
		for i := 0; i < 12; i++ {
			req, err := http.NewRequest("GET", suite.server.URL+"/", nil)
			require.NoError(t, err)
			req.Header.Set("X-Forwarded-For", "192.168.3.200")
			
			resp, err := client.Do(req)
			require.NoError(t, err)
			resp.Body.Close()
		}

		// Resetar
		resetBody := map[string]string{
			"key":  "192.168.3.200",
			"type": "ip",
		}
		bodyBytes, _ := json.Marshal(resetBody)
		
		resp, err := client.Post(suite.server.URL+"/admin/reset", "application/json", bytes.NewBuffer(bodyBytes))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "success", response["status"])

		// Verificar se funciona novamente
		req, err := http.NewRequest("GET", suite.server.URL+"/", nil)
		require.NoError(t, err)
		req.Header.Set("X-Forwarded-For", "192.168.3.200")
		
		resp2, err := client.Do(req)
		require.NoError(t, err)
		defer resp2.Body.Close()

		// Deve funcionar após reset
		assert.Equal(t, http.StatusOK, resp2.StatusCode)
	})
}

// TestE2E_RateLimiter_Concurrency testa comportamento sob carga
func TestE2E_RateLimiter_Concurrency(t *testing.T) {
	suite := setupE2ETest(t)
	defer suite.teardownE2ETest()

	t.Run("Should handle concurrent requests correctly", func(t *testing.T) {
		const numGoroutines = 20
		const requestsPerGoroutine = 3
		
		resultsChan := make(chan int, numGoroutines*requestsPerGoroutine)
		
		// Simular carga concorrente
		for g := 0; g < numGoroutines; g++ {
			go func(goroutineID int) {
				client := &http.Client{Timeout: 5 * time.Second}
				
				for r := 0; r < requestsPerGoroutine; r++ {
					req, err := http.NewRequest("GET", suite.server.URL+"/", nil)
					if err != nil {
						resultsChan <- http.StatusInternalServerError
						continue
					}
					
					// Usar IP único por goroutine
					req.Header.Set("X-Forwarded-For", fmt.Sprintf("192.168.10.%d", goroutineID+1))
					
					resp, err := client.Do(req)
					if err != nil {
						resultsChan <- http.StatusInternalServerError
						continue
					}
					
					resultsChan <- resp.StatusCode
					resp.Body.Close()
				}
			}(g)
		}

		// Coletar resultados
		var statusOK, status429, statusOther int
		for i := 0; i < numGoroutines*requestsPerGoroutine; i++ {
			status := <-resultsChan
			switch status {
			case http.StatusOK:
				statusOK++
			case http.StatusTooManyRequests:
				status429++
			default:
				statusOther++
			}
		}

		// A maioria deve ser OK (dentro dos limites de IP)
		assert.Greater(t, statusOK, 0, "Should have some successful requests")
		assert.Equal(t, 0, statusOther, "Should not have unexpected status codes")
		
		// Total deve ser correto
		total := statusOK + status429 + statusOther
		assert.Equal(t, numGoroutines*requestsPerGoroutine, total)
	})
} 