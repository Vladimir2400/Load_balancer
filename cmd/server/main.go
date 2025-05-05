// Пакет main является точкой входа для приложения балансировщика нагрузки.
// Он инициализирует конфигурацию, пул бэкендов, rate limiter (если включен),
// настраивает HTTP сервер и middleware, а также обрабатывает graceful shutdown.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	admin_api "cloud/load_balancer/internal/adminapi"
	balancer_pkg "cloud/load_balancer/internal/balancer"
	cfg_pkg "cloud/load_balancer/internal/config"
	httputil_pkg "cloud/load_balancer/internal/httputil"
	mw_pkg "cloud/load_balancer/internal/middleware"
	rl_pkg "cloud/load_balancer/internal/ratelimiter"

	sqlite_store "cloud/load_balancer/storage/sqlite"
)

func main() {
	// 1. Обработка флагов командной строки
	// Определяем флаг -config для указания пути к файлу конфигурации.
	configPath := flag.String("config", "config.yaml", "Path to the configuration file (e.g., config.yaml)")
	flag.Parse()

	// 2. Загрузка и логирование конфигурации
	log.Println("INFO: Loading configuration...")
	cfg, err := cfg_pkg.LoadConfig(*configPath)
	if err != nil {
		// Критическая ошибка при загрузке или валидации конфигурации.
		log.Fatalf("FATAL: Failed to load configuration: %v", err)
	}

	// Логируем загруженную конфигурацию для информации.
	log.Println("--- Configuration Loaded ---")
	log.Printf("INFO: Listening on port: %s", cfg.Port)
	log.Printf("INFO: Backend servers: %s", strings.Join(cfg.Backends, ", "))
	log.Printf("INFO: Health check interval: %v", cfg.HealthCheckInterval)
	log.Printf("INFO: Health check timeout: %v", cfg.HealthCheckTimeout)
	log.Printf("INFO: Rate Limiter Enabled: %t", cfg.RateLimiter.Enabled)
	if cfg.RateLimiter.Enabled {
		log.Printf("INFO:   Default Capacity: %d", cfg.RateLimiter.DefaultCapacity)
		log.Printf("INFO:   Default Refill Rate: %.2f/s", cfg.RateLimiter.DefaultRefillRate)
		log.Printf("INFO:   Cleanup Interval: %v", cfg.RateLimiter.CleanupInterval)
		if cfg.RateLimiter.DB.Driver == "sqlite" && cfg.RateLimiter.DB.Path != "" {
			log.Printf("INFO:   Custom Limits DB: %s (driver: %s)", cfg.RateLimiter.DB.Path, cfg.RateLimiter.DB.Driver)
		} else if cfg.RateLimiter.DB.Driver != "" {
			log.Printf("WARN:   DB driver '%s' specified but might be unsupported or path is missing.", cfg.RateLimiter.DB.Driver)
		} else {
			log.Println("INFO:   Custom limits DB not configured. Using defaults only.")
		}
	}
	log.Println("--------------------------")

	// 3. Инициализация Хранилища Лимитов
	var limitProvider rl_pkg.LimitProvider                          // Провайдер для чтения лимитов
	var limitManager rl_pkg.LimitManager                            // Менеджер для CRUD операций (может быть тем же объектом)
	var limitStoreCloser func() error = func() error { return nil } // Функция закрытия хранилища

	if cfg.RateLimiter.Enabled && cfg.RateLimiter.DB.Driver == "sqlite" && cfg.RateLimiter.DB.Path != "" {
		sqliteStore, err := sqlite_store.New(cfg.RateLimiter.DB.Path)
		if err != nil {
			log.Printf("ERROR: Failed to initialize SQLite limit store: %v. Proceeding without custom limits management.", err)
		} else {
			limitProvider = sqliteStore
			limitManager = sqliteStore
			limitStoreCloser = sqliteStore.Closer
			log.Println("INFO: SQLite Limit Provider & Manager initialized.")
			defer func() {
				log.Println("INFO: Closing Limit Store...")
				if err := limitStoreCloser(); err != nil {
					log.Printf("ERROR: Failed to close limit store: %v", err)
				}
			}()
		}
	} else {
		log.Println("INFO: Custom limit database is not configured. Admin API will not be available.")
		// limitProvider и limitManager остаются nil
	}

	// 4. Инициализация Rate Limiter
	var limiter *rl_pkg.Limiter
	if cfg.RateLimiter.Enabled {
		bucketStore := rl_pkg.NewBucketStore(
			cfg.RateLimiter.DefaultCapacity,
			cfg.RateLimiter.DefaultRefillRate,
			limitProvider,
		)
		if bucketStore == nil {
			log.Fatal("FATAL: Failed to create bucket store (invalid default config?)")
		}
		limiter = rl_pkg.NewLimiter(bucketStore, cfg.RateLimiter.CleanupInterval)
		if limiter == nil {
			log.Fatal("FATAL: Failed to create rate limiter")
		}
		log.Println("INFO: Rate Limiter initialized and running background cleanup task.")
		defer func() {
			log.Println("INFO: Stopping Rate Limiter...")
			limiter.Stop()
		}()
	} else {
		log.Println("INFO: Rate Limiter is disabled by configuration.")
	}

	// 5. Инициализация Пула Бэкендов
	log.Println("INFO: Initializing backend server pool...")
	serverPool := balancer_pkg.NewServerPool(cfg.Backends, cfg.HealthCheckInterval, cfg.HealthCheckTimeout)
	if len(serverPool.GetBackends()) == 0 {
		log.Fatal("FATAL: No valid backend servers were initialized. Check config file and logs for errors.")
	}
	go serverPool.HealthCheck()

	// 6. Настройка HTTP Роутера и Middleware
	router := http.NewServeMux()

	// Настраиваем обработчик балансировщика
	loadBalancerHandler := balancer_pkg.NewLoadBalancerHandler(serverPool)
	var finalBalancerHandler http.Handler = loadBalancerHandler
	if limiter != nil {
		// Применяем Rate Limiter middleware ТОЛЬКО к балансировщику
		finalBalancerHandler = mw_pkg.RateLimit(limiter)(finalBalancerHandler)
		log.Println("INFO: Rate Limiter Middleware enabled for the load balancer.")
	}
	// Регистрируем обработчик балансировщика для корневого пути "/"
	router.Handle("/", finalBalancerHandler)

	// Настраиваем и регистрируем обработчик Admin API, если менеджер лимитов доступен
	if limitManager != nil {
		adminHandler := admin_api.NewAdminHandler(limitManager)
		// Регистрируем для пути /admin/limits/ (слеш в конце важен для ServeMux)
		router.Handle("/admin/limits/", http.StripPrefix("/admin/limits", adminHandler))
		log.Println("INFO: Admin API for limits enabled at /admin/limits/")
	} else {
		// Регистрируем заглушку, если Admin API не доступен
		router.HandleFunc("/admin/limits/", func(w http.ResponseWriter, r *http.Request) {
			httputil_pkg.RespondWithError(w, http.StatusNotImplemented, "Admin API is disabled (database not configured)")
		})
		log.Println("INFO: Admin API is disabled (database not configured). Endpoint /admin/limits/ will return 501.")
	}

	//7. Настройка и Запуск HTTP Сервера
	log.Println("INFO: Configuring HTTP server...")
	server := &http.Server{
		Addr:         cfg.Port,
		Handler:      router, // Используем созданный роутер
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	// 8. Настройка Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Запускаем сервер в отдельной горутине, чтобы не блокировать основной поток.
	go func() {
		log.Printf("INFO: Starting server on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Критическая ошибка при запуске сервера (кроме штатного закрытия).
			log.Fatalf("FATAL: Could not start server on %s: %v", server.Addr, err)
		}
	}()
	log.Println("INFO: Server started. Press Ctrl+C to shut down.")

	// 9. Ожидание сигнала завершения и Graceful Shutdown
	// Блокируем основной поток, ожидая сигнала в канал quit.
	<-quit
	log.Println("INFO: Received shutdown signal. Starting graceful shutdown...")

	// Создаем контекст с таймаутом для graceful shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Пытаемся грациозно завершить работу сервера.
	if err := server.Shutdown(ctx); err != nil {
		// Ошибка при graceful shutdown (например, истек таймаут).
		log.Fatalf("FATAL: Server forced to shutdown: %v", err)
	}

	log.Println("INFO: Server shut down gracefully. Exiting.")
}
