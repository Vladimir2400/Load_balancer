package ratelimiter

import (
	"sync"
	"time"
)

type Bucket struct {
	capacity   int64
	tokens     int64
	refillRate float64
	lastRefill time.Time
	lastAccess time.Time
	mu         sync.Mutex
}

// NewBucket создает новый экземпляр Bucket с заданными параметрами.
// Бакет инициализируется полным количеством токенов.
// Возвращает nil, если capacity или rate не положительные.
func NewBucket(capacity int64, rate float64) *Bucket {
	if capacity <= 0 || rate <= 0 {
		return nil
	}
	now := time.Now()
	return &Bucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: rate,
		lastRefill: now,
		lastAccess: now,
	}
}

// refill вычисляет и добавляет токены в бакет, прошедшие с момента lastRefill.
// Количество токенов не превышает capacity.
func (b *Bucket) refill() {
	now := time.Now()
	duration := now.Sub(b.lastRefill)
	if duration <= 0 {
		return
	}
	tokensToAdd := duration.Seconds() * b.refillRate
	b.tokens += int64(tokensToAdd)
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	b.lastRefill = now
}

// Allow проверяет, доступен ли хотя бы один токен в бакете.
// Если да, то уменьшает количество токенов на 1, обновляет lastAccess и возвращает true.
// Если нет, возвращает false.
func (b *Bucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.refill()

	if b.tokens >= 1 {
		b.tokens--
		b.lastAccess = time.Now()
		return true
	}

	return false
}

// IsInactive проверяет, был ли бакет неактивен (не было вызовов Allow) дольше заданного времени.
// Используется для определения бакетов, которые можно удалить при очистке.
func (b *Bucket) IsInactive(threshold time.Duration) bool {
	b.mu.Lock()
	lastAccessTime := b.lastAccess
	b.mu.Unlock()

	return time.Since(lastAccessTime) > threshold
}
