package storage

import (
	"context"
	"testing"
	"time"

	"rate-limiter/internal/domain"
	"rate-limiter/internal/logger"

	"github.com/stretchr/testify/assert"
)

func TestMemoryStorage_Get(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		setup    func(*MemoryStorage)
		expected *domain.RateLimitStatus
	}{
		{
			name: "Should return status when key exists",
			key:  "rate_limit:ip:192.168.1.1",
			setup: func(storage *MemoryStorage) {
				storage.data["rate_limit:ip:192.168.1.1"] = &domain.RateLimitStatus{
					Key:       "rate_limit:ip:192.168.1.1",
					Count:     5,
					Limit:     10,
					Window:    60,
					LastReset: time.Now(),
					IsBlocked: false,
				}
			},
			expected: &domain.RateLimitStatus{
				Key:       "rate_limit:ip:192.168.1.1",
				Count:     5,
				Limit:     10,
				Window:    60,
				IsBlocked: false,
			},
		},
		{
			name:     "Should return nil when key doesn't exist",
			key:      "rate_limit:ip:192.168.1.2",
			setup:    func(storage *MemoryStorage) {},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			testLogger := logger.NewLogger("debug", "text")
			storage := NewMemoryStorage(testLogger)
			tt.setup(storage)

			ctx := context.Background()

			// Act
			result, err := storage.Get(ctx, tt.key)

			// Assert
			assert.NoError(t, err)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.expected.Key, result.Key)
				assert.Equal(t, tt.expected.Count, result.Count)
				assert.Equal(t, tt.expected.Limit, result.Limit)
				assert.Equal(t, tt.expected.Window, result.Window)
				assert.Equal(t, tt.expected.IsBlocked, result.IsBlocked)
			}
		})
	}
}

func TestMemoryStorage_Set(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		status *domain.RateLimitStatus
		ttl    time.Duration
	}{
		{
			name: "Should set status successfully",
			key:  "rate_limit:ip:192.168.1.1",
			status: &domain.RateLimitStatus{
				Key:       "rate_limit:ip:192.168.1.1",
				Count:     5,
				Limit:     10,
				Window:    60,
				LastReset: time.Now(),
				IsBlocked: false,
			},
			ttl: time.Minute,
		},
		{
			name: "Should set status without TTL",
			key:  "rate_limit:ip:192.168.1.2",
			status: &domain.RateLimitStatus{
				Key:   "rate_limit:ip:192.168.1.2",
				Count: 1,
			},
			ttl: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			testLogger := logger.NewLogger("debug", "text")
			storage := NewMemoryStorage(testLogger)
			ctx := context.Background()

			// Act
			err := storage.Set(ctx, tt.key, tt.status, tt.ttl)

			// Assert
			assert.NoError(t, err)

			// Verify data was stored
			stored, exists := storage.data[tt.key]
			assert.True(t, exists)
			assert.Equal(t, tt.status.Key, stored.Key)
			assert.Equal(t, tt.status.Count, stored.Count)
		})
	}
}

func TestMemoryStorage_Set_WithTTL(t *testing.T) {
	// Arrange
	testLogger := logger.NewLogger("debug", "text")
	storage := NewMemoryStorage(testLogger)
	ctx := context.Background()

	key := "rate_limit:ip:192.168.1.1"
	status := &domain.RateLimitStatus{
		Key:   key,
		Count: 1,
	}
	ttl := 100 * time.Millisecond

	// Act
	err := storage.Set(ctx, key, status, ttl)

	// Assert
	assert.NoError(t, err)

	// Verify data exists initially
	_, exists := storage.data[key]
	assert.True(t, exists)

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Verify data was removed
	storage.mutex.RLock()
	_, exists = storage.data[key]
	storage.mutex.RUnlock()
	assert.False(t, exists)
}

func TestMemoryStorage_Increment(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		limit         int
		window        time.Duration
		setup         func(*MemoryStorage)
		expectedCount int
		expectBlocked bool
	}{
		{
			name:          "Should increment new key",
			key:           "rate_limit:ip:192.168.1.1",
			limit:         10,
			window:        time.Minute,
			setup:         func(storage *MemoryStorage) {},
			expectedCount: 1,
			expectBlocked: false,
		},
		{
			name:   "Should increment existing key",
			key:    "rate_limit:ip:192.168.1.2",
			limit:  10,
			window: time.Minute,
			setup: func(storage *MemoryStorage) {
				storage.data["rate_limit:ip:192.168.1.2"] = &domain.RateLimitStatus{
					Key:       "rate_limit:ip:192.168.1.2",
					Count:     5,
					Limit:     10,
					Window:    60,
					LastReset: time.Now(),
					IsBlocked: false,
				}
			},
			expectedCount: 6,
			expectBlocked: false,
		},
		{
			name:   "Should exceed limit and block",
			key:    "rate_limit:ip:192.168.1.3",
			limit:  5,
			window: time.Minute,
			setup: func(storage *MemoryStorage) {
				storage.data["rate_limit:ip:192.168.1.3"] = &domain.RateLimitStatus{
					Key:       "rate_limit:ip:192.168.1.3",
					Count:     5,
					Limit:     5,
					Window:    60,
					LastReset: time.Now(),
					IsBlocked: false,
				}
			},
			expectedCount: 6,
			expectBlocked: true,
		},
		{
			name:   "Should reset counter after window expires",
			key:    "rate_limit:ip:192.168.1.4",
			limit:  10,
			window: 100 * time.Millisecond,
			setup: func(storage *MemoryStorage) {
				storage.data["rate_limit:ip:192.168.1.4"] = &domain.RateLimitStatus{
					Key:       "rate_limit:ip:192.168.1.4",
					Count:     5,
					Limit:     10,
					Window:    1,
					LastReset: time.Now().Add(-2 * time.Second), // Expired
					IsBlocked: false,
				}
			},
			expectedCount: 1,
			expectBlocked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			testLogger := logger.NewLogger("debug", "text")
			storage := NewMemoryStorage(testLogger)
			tt.setup(storage)

			ctx := context.Background()

			// Act
			count, lastReset, err := storage.Increment(ctx, tt.key, tt.limit, tt.window)

			// Assert
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedCount, count)
			assert.NotZero(t, lastReset)

			// Verify storage state
			stored := storage.data[tt.key]
			assert.Equal(t, tt.expectedCount, stored.Count)
			assert.Equal(t, tt.expectBlocked, stored.IsBlocked)
		})
	}
}

func TestMemoryStorage_IsBlocked(t *testing.T) {
	tests := []struct {
		name           string
		key            string
		setup          func(*MemoryStorage)
		expectedBlocked bool
		expectedTime   bool
	}{
		{
			name:           "Should return false for non-existent key",
			key:            "rate_limit:ip:192.168.1.1",
			setup:          func(storage *MemoryStorage) {},
			expectedBlocked: false,
			expectedTime:   false,
		},
		{
			name: "Should return false for non-blocked key",
			key:  "rate_limit:ip:192.168.1.2",
			setup: func(storage *MemoryStorage) {
				storage.data["rate_limit:ip:192.168.1.2"] = &domain.RateLimitStatus{
					Key:       "rate_limit:ip:192.168.1.2",
					IsBlocked: false,
				}
			},
			expectedBlocked: false,
			expectedTime:   false,
		},
		{
			name: "Should return true for blocked key",
			key:  "rate_limit:ip:192.168.1.3",
			setup: func(storage *MemoryStorage) {
				storage.data["rate_limit:ip:192.168.1.3"] = &domain.RateLimitStatus{
					Key:       "rate_limit:ip:192.168.1.3",
					IsBlocked: true,
				}
			},
			expectedBlocked: true,
			expectedTime:   false,
		},
		{
			name: "Should return true for specifically blocked key",
			key:  "rate_limit:ip:192.168.1.4",
			setup: func(storage *MemoryStorage) {
				futureTime := time.Now().Add(5 * time.Minute)
				storage.blocks["rate_limit:ip:192.168.1.4"] = futureTime
			},
			expectedBlocked: true,
			expectedTime:   true,
		},
		{
			name: "Should return false for expired block",
			key:  "rate_limit:ip:192.168.1.5",
			setup: func(storage *MemoryStorage) {
				pastTime := time.Now().Add(-5 * time.Minute)
				storage.blocks["rate_limit:ip:192.168.1.5"] = pastTime
			},
			expectedBlocked: false,
			expectedTime:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			testLogger := logger.NewLogger("debug", "text")
			storage := NewMemoryStorage(testLogger)
			tt.setup(storage)

			ctx := context.Background()

			// Act
			blocked, blockedUntil, err := storage.IsBlocked(ctx, tt.key)

			// Assert
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedBlocked, blocked)
			if tt.expectedTime {
				assert.NotNil(t, blockedUntil)
			} else {
				// blockedUntil pode ser nil ou uma data passada
				if blockedUntil != nil {
					assert.True(t, time.Now().After(*blockedUntil))
				}
			}
		})
	}
}

func TestMemoryStorage_Block(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		duration time.Duration
		setup    func(*MemoryStorage)
	}{
		{
			name:     "Should block new key",
			key:      "rate_limit:ip:192.168.1.1",
			duration: 3 * time.Minute,
			setup:    func(storage *MemoryStorage) {},
		},
		{
			name:     "Should block existing key",
			key:      "rate_limit:ip:192.168.1.2",
			duration: 3 * time.Minute,
			setup: func(storage *MemoryStorage) {
				storage.data["rate_limit:ip:192.168.1.2"] = &domain.RateLimitStatus{
					Key:       "rate_limit:ip:192.168.1.2",
					Count:     5,
					IsBlocked: false,
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			testLogger := logger.NewLogger("debug", "text")
			storage := NewMemoryStorage(testLogger)
			tt.setup(storage)

			ctx := context.Background()

			// Act
			err := storage.Block(ctx, tt.key, tt.duration)

			// Assert
			assert.NoError(t, err)

			// Verify block was set
			blockedUntil, exists := storage.blocks[tt.key]
			assert.True(t, exists)
			assert.True(t, blockedUntil.After(time.Now()))

			// Verify status was updated
			status := storage.data[tt.key]
			assert.NotNil(t, status)
			assert.True(t, status.IsBlocked)
			assert.NotNil(t, status.BlockedUntil)
		})
	}
}

func TestMemoryStorage_Reset(t *testing.T) {
	// Arrange
	testLogger := logger.NewLogger("debug", "text")
	storage := NewMemoryStorage(testLogger)

	key := "rate_limit:ip:192.168.1.1"
	
	// Setup data and block
	storage.data[key] = &domain.RateLimitStatus{
		Key:   key,
		Count: 5,
	}
	storage.blocks[key] = time.Now().Add(5 * time.Minute)

	ctx := context.Background()

	// Act
	err := storage.Reset(ctx, key)

	// Assert
	assert.NoError(t, err)

	// Verify data was removed
	_, dataExists := storage.data[key]
	assert.False(t, dataExists)

	_, blockExists := storage.blocks[key]
	assert.False(t, blockExists)
}

func TestMemoryStorage_Health(t *testing.T) {
	// Arrange
	testLogger := logger.NewLogger("debug", "text")
	storage := NewMemoryStorage(testLogger)
	ctx := context.Background()

	// Act
	err := storage.Health(ctx)

	// Assert
	assert.NoError(t, err)
}

func TestMemoryStorage_Close(t *testing.T) {
	// Arrange
	testLogger := logger.NewLogger("debug", "text")
	storage := NewMemoryStorage(testLogger)

	// Add some data
	storage.data["test"] = &domain.RateLimitStatus{Key: "test"}
	storage.blocks["test"] = time.Now()

	// Act
	err := storage.Close()

	// Assert
	assert.NoError(t, err)
	assert.Empty(t, storage.data)
	assert.Empty(t, storage.blocks)
}

func TestMemoryStorage_GetStats(t *testing.T) {
	// Arrange
	testLogger := logger.NewLogger("debug", "text")
	storage := NewMemoryStorage(testLogger)

	// Add some data
	storage.data["test1"] = &domain.RateLimitStatus{Key: "test1"}
	storage.data["test2"] = &domain.RateLimitStatus{Key: "test2"}
	storage.blocks["block1"] = time.Now()

	// Act
	stats := storage.GetStats()

	// Assert
	assert.Equal(t, 2, stats["data_entries"])
	assert.Equal(t, 1, stats["blocks_entries"])
	assert.Equal(t, "memory", stats["type"])
}

func TestMemoryStorage_CleanupExpiredEntries(t *testing.T) {
	// Arrange
	testLogger := logger.NewLogger("debug", "text")
	storage := NewMemoryStorage(testLogger)

	now := time.Now()
	
	// Add expired block
	storage.blocks["expired_block"] = now.Add(-5 * time.Minute)
	// Add valid block
	storage.blocks["valid_block"] = now.Add(5 * time.Minute)
	
	// Add expired data
	storage.data["expired_data"] = &domain.RateLimitStatus{
		Key:       "expired_data",
		Window:    60,
		LastReset: now.Add(-3 * time.Minute), // Expired (> 2 * window)
	}
	// Add valid data
	storage.data["valid_data"] = &domain.RateLimitStatus{
		Key:       "valid_data",
		Window:    60,
		LastReset: now.Add(-30 * time.Second), // Valid
	}

	// Act
	storage.cleanupExpiredEntries()

	// Assert
	// Expired entries should be removed
	_, expiredBlockExists := storage.blocks["expired_block"]
	assert.False(t, expiredBlockExists)

	_, expiredDataExists := storage.data["expired_data"]
	assert.False(t, expiredDataExists)

	// Valid entries should remain
	_, validBlockExists := storage.blocks["valid_block"]
	assert.True(t, validBlockExists)

	_, validDataExists := storage.data["valid_data"]
	assert.True(t, validDataExists)
}

func TestMemoryStorage_ConcurrentAccess(t *testing.T) {
	// Arrange
	testLogger := logger.NewLogger("debug", "text")
	storage := NewMemoryStorage(testLogger)
	ctx := context.Background()

	key := "rate_limit:ip:192.168.1.1"
	numGoroutines := 10
	done := make(chan bool, numGoroutines)

	// Act - Concurrent increments
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer func() { done <- true }()
			_, _, err := storage.Increment(ctx, key, 100, time.Minute)
			assert.NoError(t, err)
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Assert
	status, err := storage.Get(ctx, key)
	assert.NoError(t, err)
	assert.Equal(t, numGoroutines, status.Count)
} 