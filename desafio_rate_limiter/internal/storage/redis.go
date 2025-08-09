package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"rate-limiter/internal/domain"

	"github.com/go-redis/redis/v8"
)

// RedisStorage implementa a interface domain.RateLimiterStorage usando Redis
type RedisStorage struct {
	client redis.Cmdable
	logger domain.Logger
}

// NewRedisStorage cria uma nova instância do RedisStorage
func NewRedisStorage(host, port, password string, db int, logger domain.Logger) (*RedisStorage, error) {
	// Configura cliente Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", host, port),
		Password: password,
		DB:       db,
		
		// Configurações de performance
		PoolSize:        20,
		MinIdleConns:    5,
		MaxRetries:      3,
		DialTimeout:     5 * time.Second,
		ReadTimeout:     3 * time.Second,
		WriteTimeout:    3 * time.Second,
		PoolTimeout:     4 * time.Second,
		IdleTimeout:     5 * time.Minute,
	})

	// Testa a conexão
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("Redis connection established", map[string]interface{}{
		"host": host,
		"port": port,
		"db":   db,
	})

	return &RedisStorage{
		client: rdb,
		logger: logger,
	}, nil
}

// Get recupera o status atual de rate limit para uma chave
func (r *RedisStorage) Get(ctx context.Context, key string) (*domain.RateLimitStatus, error) {
	start := time.Now()
	
	// Busca dados no Redis
	result, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			// Chave não existe, retorna status vazio
			r.logStorageOperation("GET", key, true, time.Since(start).Seconds()*1000, nil)
			return nil, nil
		}
		r.logStorageOperation("GET", key, false, time.Since(start).Seconds()*1000, err)
		return nil, fmt.Errorf("failed to get key %s: %w", key, err)
	}

	// Parse do JSON
	var status domain.RateLimitStatus
	if err := json.Unmarshal([]byte(result), &status); err != nil {
		r.logStorageOperation("GET", key, false, time.Since(start).Seconds()*1000, err)
		return nil, fmt.Errorf("failed to unmarshal status for key %s: %w", key, err)
	}

	r.logStorageOperation("GET", key, true, time.Since(start).Seconds()*1000, nil)
	return &status, nil
}

// Set define o status de rate limit para uma chave
func (r *RedisStorage) Set(ctx context.Context, key string, status *domain.RateLimitStatus, ttl time.Duration) error {
	start := time.Now()

	// Serializa para JSON
	data, err := json.Marshal(status)
	if err != nil {
		r.logStorageOperation("SET", key, false, time.Since(start).Seconds()*1000, err)
		return fmt.Errorf("failed to marshal status for key %s: %w", key, err)
	}

	// Define no Redis com TTL
	if err := r.client.Set(ctx, key, data, ttl).Err(); err != nil {
		r.logStorageOperation("SET", key, false, time.Since(start).Seconds()*1000, err)
		return fmt.Errorf("failed to set key %s: %w", key, err)
	}

	r.logStorageOperation("SET", key, true, time.Since(start).Seconds()*1000, nil)
	return nil
}

// Increment incrementa o contador para uma chave e retorna o novo valor
func (r *RedisStorage) Increment(ctx context.Context, key string, limit int, window time.Duration) (int, time.Time, error) {
	start := time.Now()

	// Script Lua para operação atômica
	script := `
		local key = KEYS[1]
		local limit = tonumber(ARGV[1])
		local window = tonumber(ARGV[2])
		local now = tonumber(ARGV[3])
		
		-- Busca valor atual
		local current = redis.call('GET', key)
		local data = {}
		
		if current then
			data = cjson.decode(current)
		else
			data = {
				key = key,
				type = '',
				count = 0,
				limit = limit,
				window = window,
				lastReset = now,
				isBlocked = false
			}
		end
		
		-- Verifica se precisa resetar a janela
		local timeSinceReset = now - data.lastReset
		if timeSinceReset >= window * 1000 then
			data.count = 0
			data.lastReset = now
			data.isBlocked = false
		end
		
		-- Incrementa contador
		data.count = data.count + 1
		
		-- Verifica se excedeu o limite
		if data.count > limit then
			data.isBlocked = true
			-- Define tempo de bloqueio (será usado externalmente)
		end
		
		-- Calcula TTL restante
		local ttl = window - (timeSinceReset / 1000)
		if ttl <= 0 then
			ttl = window
		end
		
		-- Salva no Redis
		local encoded = cjson.encode(data)
		redis.call('SET', key, encoded, 'EX', math.ceil(ttl))
		
		return {data.count, data.lastReset}
	`

	now := time.Now().UnixMilli()
	windowMs := int64(window.Seconds())

	result, err := r.client.Eval(ctx, script, []string{key}, limit, windowMs, now).Result()
	if err != nil {
		r.logStorageOperation("INCREMENT", key, false, time.Since(start).Seconds()*1000, err)
		return 0, time.Time{}, fmt.Errorf("failed to increment key %s: %w", key, err)
	}

	// Parse do resultado
	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) != 2 {
		r.logStorageOperation("INCREMENT", key, false, time.Since(start).Seconds()*1000, fmt.Errorf("invalid result format"))
		return 0, time.Time{}, fmt.Errorf("invalid increment result for key %s", key)
	}

	count, err := strconv.Atoi(fmt.Sprint(resultSlice[0]))
	if err != nil {
		r.logStorageOperation("INCREMENT", key, false, time.Since(start).Seconds()*1000, err)
		return 0, time.Time{}, fmt.Errorf("invalid count in result for key %s: %w", key, err)
	}

	lastResetMs, err := strconv.ParseInt(fmt.Sprint(resultSlice[1]), 10, 64)
	if err != nil {
		r.logStorageOperation("INCREMENT", key, false, time.Since(start).Seconds()*1000, err)
		return 0, time.Time{}, fmt.Errorf("invalid lastReset in result for key %s: %w", key, err)
	}

	lastReset := time.UnixMilli(lastResetMs)

	r.logStorageOperation("INCREMENT", key, true, time.Since(start).Seconds()*1000, nil)
	return count, lastReset, nil
}

// IsBlocked verifica se uma chave está bloqueada
func (r *RedisStorage) IsBlocked(ctx context.Context, key string) (bool, *time.Time, error) {
	start := time.Now()

	// Busca status
	status, err := r.Get(ctx, key)
	if err != nil {
		return false, nil, err
	}

	if status == nil {
		r.logStorageOperation("IS_BLOCKED", key, true, time.Since(start).Seconds()*1000, nil)
		return false, nil, nil
	}

	r.logStorageOperation("IS_BLOCKED", key, true, time.Since(start).Seconds()*1000, nil)
	return status.IsBlocked, status.BlockedUntil, nil
}

// Block bloqueia uma chave por um período específico
func (r *RedisStorage) Block(ctx context.Context, key string, duration time.Duration) error {
	start := time.Now()

	// Busca status atual
	status, err := r.Get(ctx, key)
	if err != nil {
		return err
	}

	if status == nil {
		// Cria novo status bloqueado
		blockedUntil := time.Now().Add(duration)
		status = &domain.RateLimitStatus{
			Key:          key,
			IsBlocked:    true,
			BlockedUntil: &blockedUntil,
			LastReset:    time.Now(),
		}
	} else {
		// Atualiza status existente
		blockedUntil := time.Now().Add(duration)
		status.IsBlocked = true
		status.BlockedUntil = &blockedUntil
	}

	// Salva status atualizado
	if err := r.Set(ctx, key, status, duration+time.Minute); err != nil {
		r.logStorageOperation("BLOCK", key, false, time.Since(start).Seconds()*1000, err)
		return err
	}

	r.logStorageOperation("BLOCK", key, true, time.Since(start).Seconds()*1000, nil)
	return nil
}

// Reset limpa os dados de uma chave
func (r *RedisStorage) Reset(ctx context.Context, key string) error {
	start := time.Now()

	if err := r.client.Del(ctx, key).Err(); err != nil {
		r.logStorageOperation("RESET", key, false, time.Since(start).Seconds()*1000, err)
		return fmt.Errorf("failed to reset key %s: %w", key, err)
	}

	r.logStorageOperation("RESET", key, true, time.Since(start).Seconds()*1000, nil)
	return nil
}

// Health verifica se o storage está saudável
func (r *RedisStorage) Health(ctx context.Context) error {
	start := time.Now()

	if err := r.client.Ping(ctx).Err(); err != nil {
		r.logStorageOperation("HEALTH", "ping", false, time.Since(start).Seconds()*1000, err)
		return fmt.Errorf("Redis health check failed: %w", err)
	}

	r.logStorageOperation("HEALTH", "ping", true, time.Since(start).Seconds()*1000, nil)
	return nil
}

// Close fecha a conexão com o storage
func (r *RedisStorage) Close() error {
	if client, ok := r.client.(*redis.Client); ok {
		if err := client.Close(); err != nil {
			r.logger.Error("Failed to close Redis connection", err, nil)
			return err
		}
		r.logger.Info("Redis connection closed", nil)
	}
	return nil
}

// logStorageOperation registra operações de storage
func (r *RedisStorage) logStorageOperation(operation, key string, success bool, latency float64, err error) {
	if r.logger != nil {
		if success {
			r.logger.Debug("Storage operation completed", map[string]interface{}{
				"operation": operation,
				"key":       key,
				"latency":   latency,
			})
		} else {
			r.logger.Error("Storage operation failed", err, map[string]interface{}{
				"operation": operation,
				"key":       key,
				"latency":   latency,
			})
		}
	}
}

// BuildKey constrói chaves padronizadas para Redis
func BuildKey(limiterType domain.LimiterType, identifier string) string {
	switch limiterType {
	case domain.IPLimiter:
		return fmt.Sprintf("rate_limit:ip:%s", identifier)
	case domain.TokenLimiter:
		return fmt.Sprintf("rate_limit:token:%s", identifier)
	default:
		return fmt.Sprintf("rate_limit:unknown:%s", identifier)
	}
} 