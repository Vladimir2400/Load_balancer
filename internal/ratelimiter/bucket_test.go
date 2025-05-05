package ratelimiter

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestBucket_AllowBasic проверяет базовую логику потребления токенов.
func TestBucket_AllowBasic(t *testing.T) {
	capacity := int64(5)
	rate := 1.0
	bucket := NewBucket(capacity, rate)
	if bucket == nil {
		t.Fatal("NewBucket returned nil")
	}

	for i := int64(0); i < capacity; i++ {
		if !bucket.Allow() {
			t.Errorf("Allow() failed on token %d, expected true", i+1)
		}
	}

	if bucket.Allow() {
		t.Errorf("Allow() succeeded after consuming all tokens, expected false")
	}
}

// TestBucket_Refill проверяет логику пополнения токенов со временем.
func TestBucket_Refill(t *testing.T) {
	capacity := int64(2)
	rate := 1.0
	bucket := NewBucket(capacity, rate)
	if bucket == nil {
		t.Fatal("NewBucket returned nil")
	}

	if !bucket.Allow() {
		t.Error("Allow failed on 1st token")
	}
	if !bucket.Allow() {
		t.Error("Allow failed on 2nd token")
	}
	if bucket.Allow() {
		t.Error("Allow succeeded after consuming all tokens")
	}

	time.Sleep(1100 * time.Millisecond)

	if !bucket.Allow() {
		t.Errorf("Allow() failed after 1.1 second wait, expected 1 token to be refilled")
	}

	if bucket.Allow() {
		t.Errorf("Allow() succeeded again immediately, expected no more tokens")
	}

	time.Sleep(2100 * time.Millisecond)

	if !bucket.Allow() {
		t.Error("Allow failed on 1st token after long wait")
	}
	if !bucket.Allow() {
		t.Error("Allow failed on 2nd token after long wait")
	}

	if bucket.Allow() {
		t.Errorf("Allow() succeeded after consuming capacity tokens, expected no more tokens")
	}
}

// TestBucket_AllowConcurrent проверяет потокобезопасность метода Allow
func TestBucket_AllowConcurrent(t *testing.T) {
	capacity := int64(100)
	rate := 10.0 // Довольно быстрая скорость пополнения
	bucket := NewBucket(capacity, rate)
	if bucket == nil {
		t.Fatal("NewBucket returned nil")
	}

	numGoroutines := 50
	numRequestsPerG := 10
	successfulRequests := int64(0)
	var wg sync.WaitGroup

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			localSuccess := 0
			for j := 0; j < numRequestsPerG; j++ {
				if bucket.Allow() {
					localSuccess++
				}
				time.Sleep(10 * time.Millisecond)
			}
			atomic.AddInt64(&successfulRequests, int64(localSuccess))
		}()
	}

	wg.Wait()

	totalRequests := int64(numGoroutines * numRequestsPerG)
	if successfulRequests <= 0 {
		t.Errorf("Expected some successful requests in concurrent test, got %d", successfulRequests)
	}
	if successfulRequests > totalRequests {
		t.Errorf("Successful requests (%d) cannot exceed total requests (%d)", successfulRequests, totalRequests)
	}
	t.Logf("Concurrent Allow test finished. Successful requests: %d / %d", successfulRequests, totalRequests)
}
