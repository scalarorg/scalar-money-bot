package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all configuration values
type Config struct {
	RpcUrl          string
	CauldronAddress string
	PrivateKey      string
	CheckInterval   time.Duration
	Port            string
	LogLevel        string
	DatabaseURL     string
}

// LoadConfig loads configuration from environment variables
func LoadConfig(filenames ...string) (*Config, error) {
	// Load .env file if it exists
	if err := godotenv.Load(filenames...); err != nil {
		fmt.Printf("No %v file found, using environment variables\n", filenames)
	}

	config := &Config{
		RpcUrl:          getEnv("RPC_URL", ""),
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
	if config.RpcUrl == "" {
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
