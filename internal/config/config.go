package config

import (
	"fmt"
	"log"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// DBConfig содержит параметры подключения к базе данных для кастомных лимитов rate limiter.
type DBConfig struct {
	Driver string `yaml:"driver"`
	Path   string `yaml:"path"`
}

type RateLimiterConfig struct {
	Enabled            bool          `yaml:"enabled"`
	DefaultCapacity    int64         `yaml:"default_capacity"`
	DefaultRefillRate  float64       `yaml:"default_refill_rate"`
	CleanupIntervalStr string        `yaml:"cleanup_interval"`
	CleanupInterval    time.Duration `yaml:"-"`
	DB                 DBConfig      `yaml:"db"`
}

// Config представляет основную конфигурацию приложения балансировщика нагрузки.
// Загружается из YAML файла, может переопределяться переменными окружения.
type Config struct {
	Port                   string            `json:"port`
	Backends               []string          `json:"backends"`
	HealthCheckIntervalStr string            `yaml:"health_check_interval"`
	HealthCheckTimeoutStr  string            `yaml:"health_check_timeout"`
	HealthCheckInterval    time.Duration     `yaml:"-"`
	HealthCheckTimeout     time.Duration     `yaml:"-"`
	RateLimiter            RateLimiterConfig `yaml:"rate_limiter"`
}

// LoadConfig загружает конфигурацию из указанного файла YAML.
// Применяет значения по умолчанию, переопределяет их значениями из файла,
// а затем значениями из переменных окружения (если они установлены).
// Также выполняет парсинг строковых значений времени в time.Duration и валидацию.
// Возвращает загруженную конфигурацию или ошибку, если конфигурация невалидна.
func LoadConfig(configPath string) (*Config, error) {
	cfg := &Config{
		Port:                   ":8080",
		HealthCheckIntervalStr: "10s",
		HealthCheckTimeoutStr:  "2s",
		Backends:               []string{},
		RateLimiter: RateLimiterConfig{
			Enabled:            false,
			DefaultCapacity:    10,
			DefaultRefillRate:  1,
			CleanupIntervalStr: "5m",
			DB: DBConfig{
				Driver: "",
				Path:   "",
			},
		},
	}

	fileData, err := os.ReadFile(configPath)
	if err == nil {
		err := yaml.Unmarshal(fileData, cfg)
		if err != nil {
			log.Printf("WARN: Could not parse config file '%s' as YAML: %v. Using defaults/env/flags.", configPath, err)
		} else {
			log.Printf("INFO: Loaded configuration from %s", configPath)
		}
	} else if !os.IsNotExist(err) {
		log.Printf("WARN: Could not read config file '%s': %v. Using defaults/env/flags.", configPath, err)
	} else {
		log.Printf("INFO: Config file '%s' not found. Using defaults/env/flags.", configPath)
	}

	if addr := os.Getenv("LB_LISTEN_ADDR"); addr != "" {
		cfg.Port = addr
	}

	var parseErr error
	cfg.HealthCheckInterval, parseErr = time.ParseDuration(cfg.HealthCheckIntervalStr)
	if parseErr != nil {
		log.Printf("WARN: Invalid health_check_interval format '%s': %v. Using default 10s.", cfg.HealthCheckIntervalStr, parseErr)
		cfg.HealthCheckInterval = 10 * time.Second
	}

	cfg.HealthCheckTimeout, parseErr = time.ParseDuration(cfg.HealthCheckTimeoutStr)
	if parseErr != nil {
		log.Printf("WARN: Invalid health_check_timeout format '%s': %v. Using default 2s.", cfg.HealthCheckTimeoutStr, parseErr)
		cfg.HealthCheckTimeout = 2 * time.Second
	}

	if len(cfg.Backends) == 0 {
		log.Fatal("FATAL: No backend servers configured. Please provide backends in config file or via environment variables.")
	}

	if cfg.RateLimiter.Enabled {
		if cfg.RateLimiter.DefaultCapacity <= 0 {
			return nil, fmt.Errorf("rate_limiter.default_capacity must be positive")
		}
		if cfg.RateLimiter.DefaultRefillRate <= 0 {
			return nil, fmt.Errorf("rate_limiter.default_refill_rate must be positive")
		}
		if cfg.RateLimiter.DB.Driver != "" {
			if cfg.RateLimiter.DB.Driver != "sqlite" {
				return nil, fmt.Errorf("unsupported rate_limiter.db.driver: %s (only 'sqlite' is supported)", cfg.RateLimiter.DB.Driver)
			}
			if cfg.RateLimiter.DB.Path == "" {
				return nil, fmt.Errorf("rate_limiter.db.path must be specified when db.driver is set")
			}
		}
	}

	return cfg, nil
}
