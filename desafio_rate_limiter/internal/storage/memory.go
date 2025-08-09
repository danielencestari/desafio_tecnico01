package storage

import (
	"context"
	"sync"
	"time"

	"rate-limiter/internal/domain"
)

// MemoryStorage implementa a interface domain.RateLimiterStorage usando memória
type MemoryStorage struct {
	data   map[string]*domain.RateLimitStatus
	blocks map[string]time.Time // chave -> bloqueado até
	mutex  sync.RWMutex
	logger domain.Logger
}

// NewMemoryStorage cria uma nova instância do MemoryStorage
func NewMemoryStorage(logger domain.Logger) *MemoryStorage {
	storage := &MemoryStorage{
		data:   make(map[string]*domain.RateLimitStatus),
		blocks: make(map[string]time.Time),
		logger: logger,
	}

	// Inicia goroutine de limpeza
	go storage.cleanup()

	if logger != nil {
		logger.Info("Memory storage initialized", nil)
	}

	return storage
}

// Get recupera o status atual de rate limit para uma chave
func (m *MemoryStorage) Get(ctx context.Context, key string) (*domain.RateLimitStatus, error) {
	start := time.Now()
	
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Verifica se a chave existe
	status, exists := m.data[key]
	if !exists {
		m.logStorageOperation("GET", key, true, time.Since(start).Seconds()*1000, nil)
		return nil, nil
	}

	// Cria cópia para evitar modificações concorrentes
	result := *status

	m.logStorageOperation("GET", key, true, time.Since(start).Seconds()*1000, nil)
	return &result, nil
}

// Set define o status de rate limit para uma chave
func (m *MemoryStorage) Set(ctx context.Context, key string, status *domain.RateLimitStatus, ttl time.Duration) error {
	start := time.Now()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Cria cópia para evitar modificações externas
	statusCopy := *status
	m.data[key] = &statusCopy

	// Se TTL for especificado, agenda remoção
	if ttl > 0 {
		go func() {
			time.Sleep(ttl)
			m.mutex.Lock()
			delete(m.data, key)
			delete(m.blocks, key)
			m.mutex.Unlock()
		}()
	}

	m.logStorageOperation("SET", key, true, time.Since(start).Seconds()*1000, nil)
	return nil
}

// Increment incrementa o contador para uma chave e retorna o novo valor
func (m *MemoryStorage) Increment(ctx context.Context, key string, limit int, window time.Duration) (int, time.Time, error) {
	start := time.Now()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	
	// Busca ou cria status
	status, exists := m.data[key]
	if !exists {
		status = &domain.RateLimitStatus{
			Key:       key,
			Count:     0,
			Limit:     limit,
			Window:    int(window.Seconds()),
			LastReset: now,
			IsBlocked: false,
		}
		m.data[key] = status
	}

	// Verifica se precisa resetar a janela
	timeSinceReset := now.Sub(status.LastReset)
	if timeSinceReset >= window {
		status.Count = 0
		status.LastReset = now
		status.IsBlocked = false
		// Remove bloqueio se existir
		delete(m.blocks, key)
	}

	// Incrementa contador
	status.Count++

	// Verifica se excedeu o limite
	if status.Count > limit {
		status.IsBlocked = true
	}

	m.logStorageOperation("INCREMENT", key, true, time.Since(start).Seconds()*1000, nil)
	return status.Count, status.LastReset, nil
}

// IsBlocked verifica se uma chave está bloqueada
func (m *MemoryStorage) IsBlocked(ctx context.Context, key string) (bool, *time.Time, error) {
	start := time.Now()

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Verifica bloqueio específico
	if blockedUntil, exists := m.blocks[key]; exists {
		if time.Now().Before(blockedUntil) {
			m.logStorageOperation("IS_BLOCKED", key, true, time.Since(start).Seconds()*1000, nil)
			return true, &blockedUntil, nil
		} else {
			// Bloqueio expirou, remove
			delete(m.blocks, key)
		}
	}

	// Verifica status geral
	status, exists := m.data[key]
	if !exists {
		m.logStorageOperation("IS_BLOCKED", key, true, time.Since(start).Seconds()*1000, nil)
		return false, nil, nil
	}

	m.logStorageOperation("IS_BLOCKED", key, true, time.Since(start).Seconds()*1000, nil)
	return status.IsBlocked, status.BlockedUntil, nil
}

// Block bloqueia uma chave por um período específico
func (m *MemoryStorage) Block(ctx context.Context, key string, duration time.Duration) error {
	start := time.Now()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	blockedUntil := time.Now().Add(duration)

	// Define bloqueio específico
	m.blocks[key] = blockedUntil

	// Atualiza status se existir
	if status, exists := m.data[key]; exists {
		status.IsBlocked = true
		status.BlockedUntil = &blockedUntil
	} else {
		// Cria novo status bloqueado
		m.data[key] = &domain.RateLimitStatus{
			Key:          key,
			IsBlocked:    true,
			BlockedUntil: &blockedUntil,
			LastReset:    time.Now(),
		}
	}

	m.logStorageOperation("BLOCK", key, true, time.Since(start).Seconds()*1000, nil)
	return nil
}

// Reset limpa os dados de uma chave
func (m *MemoryStorage) Reset(ctx context.Context, key string) error {
	start := time.Now()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.data, key)
	delete(m.blocks, key)

	m.logStorageOperation("RESET", key, true, time.Since(start).Seconds()*1000, nil)
	return nil
}

// Health verifica se o storage está saudável
func (m *MemoryStorage) Health(ctx context.Context) error {
	start := time.Now()

	m.mutex.RLock()
	dataSize := len(m.data)
	blocksSize := len(m.blocks)
	m.mutex.RUnlock()

	if m.logger != nil {
		m.logger.Debug("Memory storage health check", map[string]interface{}{
			"data_entries":   dataSize,
			"blocks_entries": blocksSize,
		})
	}

	m.logStorageOperation("HEALTH", "check", true, time.Since(start).Seconds()*1000, nil)
	return nil
}

// Close fecha a conexão com o storage (no-op para memory)
func (m *MemoryStorage) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Limpa todos os dados
	m.data = make(map[string]*domain.RateLimitStatus)
	m.blocks = make(map[string]time.Time)

	if m.logger != nil {
		m.logger.Info("Memory storage closed", nil)
	}
	return nil
}

// cleanup remove entradas expiradas periodicamente
func (m *MemoryStorage) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.cleanupExpiredEntries()
	}
}

// cleanupExpiredEntries remove entradas expiradas
func (m *MemoryStorage) cleanupExpiredEntries() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	removedBlocks := 0
	removedData := 0

	// Remove bloqueios expirados
	for key, blockedUntil := range m.blocks {
		if now.After(blockedUntil) {
			delete(m.blocks, key)
			removedBlocks++
		}
	}

	// Remove dados com janela expirada (assumindo TTL baseado em LastReset + Window)
	for key, status := range m.data {
		if status.Window > 0 {
			windowDuration := time.Duration(status.Window) * time.Second
			if now.Sub(status.LastReset) > windowDuration*2 { // Grace period
				delete(m.data, key)
				removedData++
			}
		}
	}

	if (removedBlocks > 0 || removedData > 0) && m.logger != nil {
		m.logger.Debug("Memory storage cleanup completed", map[string]interface{}{
			"removed_blocks": removedBlocks,
			"removed_data":   removedData,
		})
	}
}

// GetStats retorna estatísticas do storage em memória
func (m *MemoryStorage) GetStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return map[string]interface{}{
		"data_entries":   len(m.data),
		"blocks_entries": len(m.blocks),
		"type":           "memory",
	}
}

// logStorageOperation registra operações de storage
func (m *MemoryStorage) logStorageOperation(operation, key string, success bool, latency float64, err error) {
	if m.logger == nil {
		return
	}
	
	if success {
		m.logger.Debug("Storage operation completed", map[string]interface{}{
			"operation": operation,
			"key":       key,
			"latency":   latency,
		})
	} else {
		m.logger.Error("Storage operation failed", err, map[string]interface{}{
			"operation": operation,
			"key":       key,
			"latency":   latency,
		})
	}
} 