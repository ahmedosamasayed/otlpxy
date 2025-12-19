package main

import (
	"zep-logger/internal/app"
	"zep-logger/internal/config"
	"zep-logger/pkg/logger"
)

func main() {
	// Load configuration from config.toml
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load configuration: %v", err)
	}

	// Create and run application
	application := app.NewApp(cfg)

	logger.Info("Zep Logger starting...")

	if err := application.Run(); err != nil {
		logger.Fatal("Server error: %v", err)
	}
}
