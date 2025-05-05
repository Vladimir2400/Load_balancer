package balancer

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestBackend создает мок Backend для тестов.
func newTestBackend(rawURL string, alive bool) *Backend {
	u, _ := url.Parse(rawURL)
	return &Backend{
		URL:   u,
		Alive: alive,
	}
}

// TestServerPool_GetNextPeer_RoundRobin проверяет базовую логику Round Robin.
func TestServerPool_GetNextPeer_RoundRobin(t *testing.T) {
	pool := &ServerPool{
		backends: []*Backend{
			newTestBackend("http://backend1:8081", true),
			newTestBackend("http://backend2:8082", true),
			newTestBackend("http://backend3:8083", true),
		},
	}

	results := make(map[string]int)
	for i := 0; i < 6; i++ {
		peer := pool.GetNextPeer()
		require.NotNil(t, peer, "GetNextPeer should not return nil when backends are alive")
		results[peer.URL.String()]++
	}

	assert.Equal(t, 2, results["http://backend1:8081"], "Backend 1 count")
	assert.Equal(t, 2, results["http://backend2:8082"], "Backend 2 count")
	assert.Equal(t, 2, results["http://backend3:8083"], "Backend 3 count")
}

// TestServerPool_GetNextPeer_SkipDead проверяет, что мертвые бэкенды пропускаются.
func TestServerPool_GetNextPeer_SkipDead(t *testing.T) {
	pool := &ServerPool{
		backends: []*Backend{
			newTestBackend("http://backend1:8081", true),
			newTestBackend("http://backend2:8082", false), // Этот мертв
			newTestBackend("http://backend3:8083", true),
		},
	}

	results := make(map[string]int)
	for i := 0; i < 6; i++ {
		peer := pool.GetNextPeer()
		require.NotNil(t, peer, "GetNextPeer should not return nil when some backends are alive")
		results[peer.URL.String()]++
	}

	// Бэкенды 1 и 3 должны быть выбраны по 3 раза, бэкенд 2 - 0 раз.
	assert.Equal(t, 3, results["http://backend1:8081"], "Backend 1 count")
	assert.Equal(t, 0, results["http://backend2:8082"], "Backend 2 count")
	assert.Equal(t, 3, results["http://backend3:8083"], "Backend 3 count")
}

// TestServerPool_GetNextPeer_AllDead проверяет, что возвращается nil, если все бэкенды мертвы.
func TestServerPool_GetNextPeer_AllDead(t *testing.T) {
	pool := &ServerPool{
		backends: []*Backend{
			newTestBackend("http://backend1:8081", false),
			newTestBackend("http://backend2:8082", false),
			newTestBackend("http://backend3:8083", false),
		},
	}

	peer := pool.GetNextPeer()
	assert.Nil(t, peer, "GetNextPeer should return nil when all backends are dead")
}

// TestServerPool_GetNextPeer_Empty проверяет, что возвращается nil, если пул пуст.
func TestServerPool_GetNextPeer_Empty(t *testing.T) {
	pool := &ServerPool{
		backends: []*Backend{},
	}

	peer := pool.GetNextPeer()
	assert.Nil(t, peer, "GetNextPeer should return nil for an empty pool")
}

// TestServerPool_NewServerPool_ErrorHandler проверяет настройку ErrorHandler.
// (Простой тест, просто проверяем, что ErrorHandler не nil)
func TestServerPool_NewServerPool_ErrorHandler(t *testing.T) {
	urls := []string{"http://localhost:9999"}
	pool := NewServerPool(urls, 1*time.Second, 1*time.Second)
	require.Len(t, pool.backends, 1, "Should have one backend")
	assert.NotNil(t, pool.backends[0].ReverseProxy.ErrorHandler, "ErrorHandler should be set")
}
