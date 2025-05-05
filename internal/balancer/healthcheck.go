package balancer

import (
	"log"
	"net"
	"net/url"
	"sync"
	"time"
)

// HealthCheck запускает периодическую проверку состояния всех бэкендов в пуле.
// Сначала выполняется немедленная проверка, затем проверки повторяются с интервалом s.healthCheckInterval.
func (s *ServerPool) HealthCheck() {
	log.Println("INFO: Starting initial health check...")
	s.runHealthCheckCycle()
	log.Println("INFO: Initial health check completed.")

	ticker := time.NewTicker(s.healthCheckInterval)
	defer ticker.Stop()

	for {
		<-ticker.C
		s.runHealthCheckCycle()
	}
}

// runHealthCheckCycle выполняет один цикл проверки состояния для всех бэкендов в пуле.
// Проверки выполняются параллельно для ускорения.
func (s *ServerPool) runHealthCheckCycle() {
	log.Println("INFO: Starting health check cycle...")
	wg := sync.WaitGroup{}
	backends := s.GetBackends()

	for _, b := range backends {
		wg.Add(1)
		go func(backend *Backend) {
			defer wg.Done()
			status := "up"
			alive := isBackendAlive(backend.URL, s.healthCheckTimeout)
			backend.SetAlive(alive)
			if !alive {
				status = "down"
			}
			log.Printf("INFO: Health Check: Backend %s is %s", backend.URL, status)
		}(b)
	}
	wg.Wait()
	log.Println("INFO: Health check cycle completed.")
}

// isBackendAlive проверяет доступность одного бэкенда путем попытки установить TCP-соединение.
// Возвращает true, если соединение успешно установлено в течение заданного таймаута, иначе false.
func isBackendAlive(u *url.URL, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", u.Host, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
