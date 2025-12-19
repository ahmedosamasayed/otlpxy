package config

import (
	"fmt"
	"log"

	"github.com/spf13/viper"
)

// Config holds all configuration values for the application
type Config struct {
	OtelCollectorTargetURL string `mapstructure:"otel_collector_target_url"`
	OtelCollectorAPIKey    string `mapstructure:"otel_collector_api_key"`
	ShutdownDrainSeconds   int    `mapstructure:"shutdown_drain_seconds"`
	ShutdownTimeoutSeconds int    `mapstructure:"shutdown_timeout_seconds"`
	ServerPort             int    `mapstructure:"server_port"`
	WorkerPoolSize         int    `mapstructure:"worker_pool_size"`
	JobQueueSize           int    `mapstructure:"job_queue_size"`
	AllowedOrigins         []string `mapstructure:"allowed_origins"` // CORS allowed origins
	MaxRequestSizeMB       int    `mapstructure:"max_request_size_mb"`      // Request body size limit in MB
    ForwardingMode         string `mapstructure:"forwarding_mode"`          // "pool" or "semaphore"
    SemaphoreMaxConcurrent int    `mapstructure:"semaphore_max_concurrent"` // Max concurrent requests in semaphore mode
    SyncLogsDebug          bool   `mapstructure:"sync_logs_debug"`          // If true, /v1/logs forwards synchronously
}

// Load reads configuration from config.toml file
// Returns error if configuration file is missing or required fields are not set
func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")

	// Set default values
	viper.SetDefault("shutdown_drain_seconds", 2)
	viper.SetDefault("shutdown_timeout_seconds", 10)
	viper.SetDefault("server_port", 8080)
	viper.SetDefault("worker_pool_size", 0)    // 0 = auto-detect 2×NumCPU in worker.NewPool()
	viper.SetDefault("job_queue_size", 10000)  // Default job queue buffer size
	viper.SetDefault("allowed_origins", []string{"*"}) // Default wildcard for development
	viper.SetDefault("max_request_size_mb", 1) // Default 1MB request size limit
    viper.SetDefault("forwarding_mode", "pool") // Default to existing pool behavior
    viper.SetDefault("semaphore_max_concurrent", 10000)
    viper.SetDefault("sync_logs_debug", false)

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate required configuration
	if config.OtelCollectorTargetURL == "" {
		return nil, fmt.Errorf("otel_collector_target_url is required in config file")
	}

	// Warn if API key is empty
	if config.OtelCollectorAPIKey == "" {
		log.Printf("WARN:  otel_collector_api_key is empty - forwarding will not include authentication")
	}


    // Normalize/validate forwarding mode
    switch config.ForwardingMode {
    case "pool", "semaphore", "hybrid":
        // ok
    case "":
        config.ForwardingMode = "pool"
    default:
        log.Printf("WARN:  unknown forwarding_mode=%q, defaulting to 'pool'", config.ForwardingMode)
        config.ForwardingMode = "pool"
    }

    if config.SemaphoreMaxConcurrent <= 0 {
        log.Printf("WARN:  semaphore_max_concurrent <= 0 (%d), defaulting to 10000", config.SemaphoreMaxConcurrent)
        config.SemaphoreMaxConcurrent = 10000
    }

	log.Printf("INFO:  Configuration loaded successfully from %s", viper.ConfigFileUsed())
	log.Printf("INFO:    otel_collector_target_url: %s", config.OtelCollectorTargetURL)
	log.Printf("INFO:    shutdown_drain_seconds: %d", config.ShutdownDrainSeconds)
	log.Printf("INFO:    shutdown_timeout_seconds: %d", config.ShutdownTimeoutSeconds)
	log.Printf("INFO:    server_port: %d", config.ServerPort)
	log.Printf("INFO:    worker_pool_size: %d (0 = auto-detect)", config.WorkerPoolSize)
	log.Printf("INFO:    job_queue_size: %d", config.JobQueueSize)
	log.Printf("INFO:    allowed_origins: %v", config.AllowedOrigins)
	log.Printf("INFO:    max_request_size_mb: %d", config.MaxRequestSizeMB)
    log.Printf("INFO:    forwarding_mode: %s", config.ForwardingMode)
    if config.ForwardingMode == "semaphore" || config.ForwardingMode == "hybrid" {
        log.Printf("INFO:    semaphore_max_concurrent: %d", config.SemaphoreMaxConcurrent)
    }
    log.Printf("INFO:    sync_logs_debug: %v", config.SyncLogsDebug)

	return &config, nil
}
