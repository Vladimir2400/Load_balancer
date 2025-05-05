package ratelimiter

import (
	"log"
	"sync"
	"time"
)

// Limiter является основным компонентом Rate Limiter.
// Он управляет хранилищем бакетов (BucketStore), проверяет лимиты для клиентов
// и запускает фоновую задачу для очистки неактивных бакетов.
type Limiter struct {
	store           *BucketStore
	stopChan        chan struct{}
	cleanupInterval time.Duration
	wg              sync.WaitGroup
}

// NewLimiter создает, инициализирует и запускает новый Limiter.
// Принимает BucketStore и интервал очистки.
// Запускает горутину для периодической очистки.
// Возвращает nil, если store равен nil.
func NewLimiter(store *BucketStore, cleanupInterval time.Duration) *Limiter {
	if store == nil {
		log.Println("ERROR: Cannot create Limiter with a nil BucketStore")
		return nil
	}
	if cleanupInterval <= 0 {
		log.Printf("WARN: Invalid cleanupInterval (%v) for Limiter, using default 5m", cleanupInterval)
		cleanupInterval = 5 * time.Minute
	}

	limiter := &Limiter{
		store:           store,
		stopChan:        make(chan struct{}),
		cleanupInterval: cleanupInterval,
	}

	limiter.wg.Add(1)
	go limiter.runCleanup()

	return limiter
}

// Allow проверяет, разрешен ли запрос для данного clientID.
// Получает или создает бакет для клиента из BucketStore и вызывает его метод Allow.
// Возвращает true, если запрос разрешен, иначе false.
func (l *Limiter) Allow(clientID string) bool {
	bucket := l.store.GetOrCreateBucket(clientID)
	if bucket == nil {
		log.Printf("ERROR: Could not get or create bucket for client %s in Limiter.Allow", clientID)
		return false
	}
	return bucket.Allow()
}

// runCleanup - это фоновая горутина, которая периодически удаляет старые/неактивные бакеты из хранилища.
// Это предотвращает утечку памяти при большом количестве уникальных клиентов.
func (l *Limiter) runCleanup() {
	defer l.wg.Done()
	ticker := time.NewTicker(l.cleanupInterval)
	defer ticker.Stop()

	inactivityThreshold := l.cleanupInterval * 2
	log.Printf("INFO: Limiter cleanup goroutine started (interval: %v, inactivity threshold: %v)", l.cleanupInterval, inactivityThreshold)

	for {
		select {
		case <-ticker.C:
			log.Println("DEBUG: Running limiter cleanup...")
			cleanedCount := 0

			l.store.mu.Lock()
			for id, bucket := range l.store.buckets {
				if bucket.IsInactive(inactivityThreshold) {
					delete(l.store.buckets, id)
					cleanedCount++
					log.Printf("DEBUG: Cleaned up inactive bucket for client %s", id)
				}
			}
			l.store.mu.Unlock()

			if cleanedCount > 0 {
				log.Printf("INFO: Limiter cleanup finished. Removed %d inactive buckets.", cleanedCount)
			}

		case <-l.stopChan:
			log.Println("INFO: Limiter cleanup goroutine stopping.")
			return
		}
	}
}

// Stop грациозно останавливает Limiter.
// Сигнализирует горутине очистки о необходимости завершения и ожидает ее остановки.
func (l *Limiter) Stop() {
	log.Println("INFO: Stopping Limiter...")
	close(l.stopChan)
	l.wg.Wait()
	log.Println("INFO: Limiter stopped gracefully.")
}
