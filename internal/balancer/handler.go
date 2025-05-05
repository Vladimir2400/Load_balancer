package balancer

import (
	"context"
	"log"
	"net/http"
	"time"

	httputil_pkg "cloud/load_balancer/internal/httputil"
)

// NewLoadBalancerHandler создает новый http.Handler, который распределяет входящие запросы
// между доступными бэкендами из предоставленного ServerPool.
// Если пул не настроен или не содержит бэкендов, возвращает обработчик, отвечающий ошибкой 500.
func NewLoadBalancerHandler(pool *ServerPool) http.Handler {
	if pool == nil || len(pool.GetBackends()) == 0 {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Printf("ERROR: Load balancer is not configured or has no valid backends. Request [%s %s]", r.Method, r.URL.Path)
			httputil_pkg.RespondWithError(w, http.StatusInternalServerError, "Load Balancer Configuration Error")
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("INFO: Received request: %s %s %s from %s", r.Method, r.Host, r.URL.Path, r.RemoteAddr)

		attempts := 0
		maxAttempts := len(pool.GetBackends())
		var peer *Backend

		for attempts < maxAttempts {
			peer = pool.GetNextPeer()
			if peer != nil {
				break
			}
			log.Printf("WARN: Attempt %d: No alive peer found for request [%s %s]. Retrying...", attempts+1, r.Method, r.URL.Path)
			attempts++
			time.Sleep(10 * time.Millisecond)
		}

		if peer == nil {
			log.Printf("ERROR: No available backends after %d attempts for request [%s %s]", maxAttempts, r.Method, r.URL.Path)
			httputil_pkg.RespondWithError(w, http.StatusServiceUnavailable, "Service Unavailable: No backend servers available")
			return
		}

		log.Printf("INFO: Forwarding request [%s %s] to backend %s", r.Method, r.URL.Path, peer.URL)

		ctx := context.WithValue(r.Context(), Retry, attempts)

		peer.ReverseProxy.ServeHTTP(w, r.WithContext(ctx))
	})
}
