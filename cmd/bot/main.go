package main

import (
	"fmt"
	"log"
	"os"
	"scalar-money-bot/internal/config"
	"scalar-money-bot/internal/database"
	"scalar-money-bot/internal/handlers"
	"scalar-money-bot/internal/services"
	"time"

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
		cfg.RpcUrl, cfg.CauldronAddress, cfg.Port)

	// Initialize database
	db, err := database.Initialize(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	repo := database.NewRepository(db)

	// Initialize services
	bot, err := services.NewLiquidationBot(cfg, repo)
	if err != nil {
		log.Fatal("Failed to create liquidation bot:", err)
	}

	monitor, err := services.NewLiquidationMonitor(cfg, repo)
	if err != nil {
		log.Fatal("Failed to create liquidation monitor:", err)
	}

	// Initialize handlers
	h := handlers.NewHandlers(bot, monitor)

	// Setup Echo
	e := echo.New()

	// Configure custom logger
	setupLogging(e, cfg.LogLevel)

	// Setup middleware
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Custom middleware for API logging
	e.Use(apiLoggingMiddleware())

	// Custom middleware for request ID
	e.Use(requestIDMiddleware())

	// Setup routes
	h.SetupRoutes(e)

	// Log all registered routes
	logRegisteredRoutes(e)

	// Start server
	log.Printf("Starting server on :%s", cfg.Port)
	if err := e.Start(":" + cfg.Port); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}

// setupLogging configures Echo's logging based on log level
func setupLogging(e *echo.Echo, logLevel string) {
	if logLevel == "debug" {
		// Detailed logging for debug mode
		e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
			Format:           `${time_custom} ${id} ${remote_ip} ${method} ${uri} ${status} ${error} ${latency_human}` + "\n",
			CustomTimeFormat: "2006-01-02 15:04:05",
			Output:           os.Stdout,
		}))
	} else {
		// Standard logging for production
		e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
			Format:           `${time_custom} ${method} ${uri} ${status} ${latency_human}` + "\n",
			CustomTimeFormat: "2006-01-02 15:04:05",
			Output:           os.Stdout,
		}))
	}
}

// requestIDMiddleware adds a unique request ID to each request
func requestIDMiddleware() echo.MiddlewareFunc {
	return middleware.RequestIDWithConfig(middleware.RequestIDConfig{
		Generator: func() string {
			return fmt.Sprintf("%d", time.Now().UnixNano())
		},
	})
}

// apiLoggingMiddleware provides detailed API request/response logging
func apiLoggingMiddleware() echo.MiddlewareFunc {
	return middleware.BodyDumpWithConfig(middleware.BodyDumpConfig{
		Skipper: func(c echo.Context) bool {
			// Skip logging for health checks and static files
			path := c.Request().URL.Path
			return path == "/api/health" ||
				path == "/favicon.ico" ||
				path == "/" ||
				(len(path) > 7 && path[:8] == "/static/")
		},
		Handler: func(c echo.Context, reqBody, resBody []byte) {
			req := c.Request()
			res := c.Response()

			// Get request ID
			requestID := c.Response().Header().Get(echo.HeaderXRequestID)
			if requestID == "" {
				requestID = "unknown"
			}

			// Log request details
			log.Printf("[API-REQ] ID:%s %s %s - Headers: %v",
				requestID,
				req.Method,
				req.URL.Path,
				getRelevantHeaders(req.Header),
			)

			// Log request body for POST/PUT/PATCH
			if len(reqBody) > 0 && (req.Method == "POST" || req.Method == "PUT" || req.Method == "PATCH") {
				log.Printf("[API-REQ-BODY] ID:%s %s", requestID, string(reqBody))
			}

			// Log response details
			log.Printf("[API-RES] ID:%s Status:%d Size:%d",
				requestID,
				res.Status,
				len(resBody),
			)

			// Log response body for errors or debug mode
			if res.Status >= 400 || os.Getenv("LOG_LEVEL") == "debug" {
				if len(resBody) > 0 {
					log.Printf("[API-RES-BODY] ID:%s %s", requestID, string(resBody))
				}
			}
		},
	})
}

// getRelevantHeaders filters headers to only include relevant ones for logging
func getRelevantHeaders(headers map[string][]string) map[string]string {
	relevant := map[string]string{}
	importantHeaders := []string{
		"Content-Type",
		"Authorization",
		"User-Agent",
		"Accept",
		"X-Real-IP",
		"X-Forwarded-For",
	}

	for _, header := range importantHeaders {
		if values, exists := headers[header]; exists && len(values) > 0 {
			relevant[header] = values[0]
		}
	}

	return relevant
}

// logRegisteredRoutes logs all registered routes at startup
func logRegisteredRoutes(e *echo.Echo) {
	log.Println("=== Registered API Routes ===")

	routes := e.Routes()
	for _, route := range routes {
		// Simple colored output without using gommon/color
		methodStr := getMethodString(route.Method)
		log.Printf("%-8s %s", methodStr, route.Path)
	}

	log.Printf("Total routes registered: %d", len(routes))
	log.Println("=============================")
}

// getMethodString returns formatted method strings
func getMethodString(method string) string {
	switch method {
	case "GET":
		return "[GET]"
	case "POST":
		return "[POST]"
	case "PUT":
		return "[PUT]"
	case "DELETE":
		return "[DELETE]"
	case "PATCH":
		return "[PATCH]"
	default:
		return fmt.Sprintf("[%s]", method)
	}
}
