package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all configuration values
type Config struct {
	RPCURL          string
	CauldronAddress string
	PrivateKey      string
	CheckInterval   time.Duration
	Port            string
	LogLevel        string
	DatabaseURL     string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		fmt.Println("No .env file found, using environment variables")
	}

	config := &Config{
		RPCURL:          getEnv("RPC_URL", ""),
		CauldronAddress: getEnv("CAULDRON_ADDRESS", ""),
		PrivateKey:      getEnv("PRIVATE_KEY", ""),
		Port:            getEnv("PORT", "8080"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://user:password@localhost/liquidation_bot?sslmode=disable"),
	}

	// Parse check interval
	checkIntervalStr := getEnv("CHECK_INTERVAL", "10s")
	interval, err := time.ParseDuration(checkIntervalStr)
	if err != nil {
		return nil, fmt.Errorf("invalid CHECK_INTERVAL format: %v", err)
	}
	config.CheckInterval = interval

	// Validate required fields
	if config.RPCURL == "" {
		return nil, fmt.Errorf("RPC_URL is required")
	}
	if config.CauldronAddress == "" {
		return nil, fmt.Errorf("CAULDRON_ADDRESS is required")
	}
	if config.PrivateKey == "" {
		return nil, fmt.Errorf("PRIVATE_KEY is required")
	}

	return config, nil
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
