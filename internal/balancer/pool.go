package balancer

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"time"
)

type ctxKey int

const Retry ctxKey = iota

// ServerPool управляет списком доступных бэкендов и выбором следующего бэкенда для обработки запроса.
type ServerPool struct {
	backends            []*Backend
	current             atomic.Uint64
	healthCheckInterval time.Duration
	healthCheckTimeout  time.Duration
}

// NewServerPool создает новый ServerPool с заданными URL бэкендов и параметрами проверки состояния.
// Он парсит URL, создает ReverseProxy для каждого бэкенда и настраивает обработчик ошибок прокси.
func NewServerPool(backendUrls []string, checkInterval, checkTimeout time.Duration) *ServerPool {
	pool := &ServerPool{
		backends:            make([]*Backend, 0),
		healthCheckInterval: checkInterval,
		healthCheckTimeout:  checkTimeout,
	}

	for _, backendURLStr := range backendUrls {
		backendURL, err := url.Parse(backendURLStr)
		if err != nil {
			log.Printf("ERROR: Invalid backend URL '%s': %v. Skipping.", backendURLStr, err)
			continue
		}

		proxy := httputil.NewSingleHostReverseProxy(backendURL)

		backend := &Backend{
			URL:          backendURL,
			Alive:        false,
			ReverseProxy: proxy,
		}

		proxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, e error) {
			log.Printf("ERROR: Proxy error connecting to backend %s: %v", backend.URL, e)

			retries := GetRetryFromContext(request)
			if retries < 1 {
				log.Printf("WARN: Marking backend %s as down due to connection error: %v", backend.URL, e)
				backend.SetAlive(false)
			} else {
				log.Printf("WARN: Backend %s connection error on retry %d: %v", backend.URL, retries, e)
			}

			http.Error(writer, "Bad Gateway: Error connecting to backend", http.StatusBadGateway)
		}

		pool.backends = append(pool.backends, backend)
		log.Printf("INFO: Added backend: %s", backendURLStr)
	}

	if len(pool.backends) == 0 {
		log.Printf("WARN: ServerPool initialized, but contains no valid backends.")
	}

	return pool
}

// GetNextPeer выбирает следующий доступный (Alive) бэкенд с использованием Round Robin.
// Если доступных бэкендов нет, возвращает nil.
func (s *ServerPool) GetNextPeer() *Backend {
	numBackends := uint64(len(s.backends))
	if numBackends == 0 {
		return nil
	}

	currentIdx := s.current.Load()

	for i := uint64(0); i < numBackends; i++ {
		nextIdx := (currentIdx + 1 + i) % numBackends

		if s.backends[nextIdx].IsAlive() {
			s.current.Store(nextIdx)
			return s.backends[nextIdx]
		}
	}

	return nil
}

func (s *ServerPool) GetBackends() []*Backend {
	return s.backends
}

// GetRetryFromContext извлекает количество попыток перенаправления из контекста запроса.
// Возвращает 0, если значение не найдено.
func GetRetryFromContext(r *http.Request) int {
	if retry, ok := r.Context().Value(Retry).(int); ok {
		return retry
	}
	return 0
}
