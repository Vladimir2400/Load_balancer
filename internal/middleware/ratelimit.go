package middleware

import (
	"log"
	"net/http"
	"strings"

	httputil_pkg "cloud/load_balancer/internal/httputil"
	rl "cloud/load_balancer/internal/ratelimiter"
)

// RateLimit является middleware-функцией, которая применяет rate limiting
// к входящим запросам на основе IP-адреса клиента.
func RateLimit(limiter *rl.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if colonPos := strings.LastIndex(ip, ":"); colonPos != -1 {
				ip = ip[:colonPos]
			}

			if strings.HasPrefix(ip, "[") && strings.HasSuffix(ip, "]") {
				ip = ip[1 : len(ip)-1]
			}

			if !limiter.Allow(ip) {
				log.Printf("WARN: Rate limit exceeded for client %s on %s", ip, r.URL.Path)
				httputil_pkg.RespondWithError(w, http.StatusTooManyRequests, "Rate limit exceeded")
				return
			}

			log.Printf("DEBUG: Request allowed for client %s on %s", ip, r.URL.Path)
			next.ServeHTTP(w, r)
		})
	}
}
