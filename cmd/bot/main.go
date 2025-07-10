package main

import (
	"log"
	"scalar-money-bot/internal/config"
	"scalar-money-bot/internal/db"
	"scalar-money-bot/internal/handlers"
	"scalar-money-bot/internal/services"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// Set log level
	if cfg.LogLevel == "debug" {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	log.Printf("Starting liquidation bot with config: RPC=%s, Cauldron=%s, Port=%s",
		cfg.RPCURL, cfg.CauldronAddress, cfg.Port)

	// Initialize database
	db, err := database.Initialize(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	// Initialize services
	bot, err := services.NewLiquidationBot(cfg, db)
	if err != nil {
		log.Fatal("Failed to create liquidation bot:", err)
	}

	monitor, err := services.NewLiquidationMonitor(cfg, db)
	if err != nil {
		log.Fatal("Failed to create liquidation monitor:", err)
	}

	// Initialize handlers
	h := handlers.NewHandlers(bot, monitor)

	// Setup Echo
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Setup routes
	h.SetupRoutes(e)

	// Start server
	log.Printf("Starting server on :%s", cfg.Port)
	if err := e.Start(":" + cfg.Port); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}