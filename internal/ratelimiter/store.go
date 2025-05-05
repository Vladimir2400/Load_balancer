package ratelimiter

import (
	"log"
	"sync"
)

// LimitProvider определяет интерфейс для получения кастомных лимитов (емкость и скорость)
// для конкретного clientID. Это позволяет использовать разные источники данных
// (например, базу данных, файл конфигурации) для задания индивидуальных лимитов.
type LimitProvider interface {
	// GetLimit запрашивает лимиты для заданного clientID.
	// Возвращает емкость (capacity), скорость пополнения (rate) и флаг found (true, если лимит найден).
	GetLimit(clientID string) (capacity int64, rate float64, found bool)
	// Closer освобождает ресурсы, связанные с провайдером (например, закрывает соединение с БД).
	// Должен быть вызван при завершении работы приложения.
	Closer() error
}

// BucketStore управляет коллекцией бакетов токенов для разных клиентов.
// Он отвечает за создание новых бакетов (с параметрами по умолчанию или кастомными из LimitProvider)
// и предоставление доступа к существующим бакетам. Доступ к map бакетов защищен мьютексом.
type BucketStore struct {
	buckets           map[string]*Bucket // Map для хранения бакетов, ключ - clientID.
	mu                sync.RWMutex       // Мьютекс для потокобезопасного доступа к map бакетов.
	defaultCapacity   int64              // Емкость бакета по умолчанию.
	defaultRefillRate float64            // Скорость пополнения по умолчанию (токенов в секунду).
	limitProvider     LimitProvider      // Необязательный провайдер для получения кастомных лимитов.
}

// NewBucketStore создает новое, пустое хранилище BucketStore.
// Принимает параметры по умолчанию (capacity, rate) и необязательный LimitProvider.
// Возвращает nil, если параметры по умолчанию невалидны.
func NewBucketStore(defaultCapacity int64, defaultRefillRate float64, provider LimitProvider) *BucketStore {
	if defaultCapacity <= 0 || defaultRefillRate <= 0 {
		log.Printf("ERROR: Invalid default parameters for NewBucketStore: capacity=%d, rate=%.2f", defaultCapacity, defaultRefillRate)
		return nil
	}
	store := &BucketStore{
		buckets:           make(map[string]*Bucket),
		defaultCapacity:   defaultCapacity,
		defaultRefillRate: defaultRefillRate,
		limitProvider:     provider,
	}
	if provider != nil {
		log.Println("INFO: BucketStore initialized with a custom LimitProvider.")
	} else {
		log.Println("INFO: BucketStore initialized without a custom LimitProvider (using defaults only).")
	}
	return store
}

// GetOrCreateBucket возвращает существующий Bucket для данного clientID или создает новый,
// если он еще не существует. При создании нового бакета сначала пытается получить
// кастомные лимиты через limitProvider. Если они не найдены или невалидны,
// используются лимиты по умолчанию. Метод потокобезопасен.
func (s *BucketStore) GetOrCreateBucket(clientID string) *Bucket {
	s.mu.RLock()
	bucket, exists := s.buckets[clientID]
	s.mu.RUnlock()

	if exists {
		return bucket
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	bucket, exists = s.buckets[clientID]
	if exists {
		return bucket
	}

	capacity := s.defaultCapacity
	rate := s.defaultRefillRate
	isCustom := false

	if s.limitProvider != nil {
		customCapacity, customRate, found := s.limitProvider.GetLimit(clientID)
		if found {
			if customCapacity > 0 && customRate > 0 {
				capacity = customCapacity
				rate = customRate
				isCustom = true
				log.Printf("INFO: Using custom rate limit for client %s: capacity=%d, rate=%.2f/s", clientID, capacity, rate)
			} else {
				log.Printf("WARN: Found invalid custom limit for client %s (capacity=%d, rate=%.2f). Using defaults.", clientID, customCapacity, customRate)
			}
		}
	}

	newBucket := NewBucket(capacity, rate)
	if newBucket == nil {
		log.Printf("ERROR: Failed to create new bucket for client %s with capacity %d, rate %.2f", clientID, capacity, rate)
		return nil
	}

	s.buckets[clientID] = newBucket
	if !isCustom {
		log.Printf("INFO: Created new bucket for client %s (Default Capacity: %d, Default Rate: %.2f/s)", clientID, capacity, rate)
	}
	return newBucket
}
