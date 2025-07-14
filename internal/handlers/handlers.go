package handlers

import (
	"net/http"
	"scalar-money-bot/constants"
	"scalar-money-bot/internal/services"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/labstack/echo/v4"
)

type MessageResponse struct {
	Message string `json:"message"`
}

type StatusResponse struct {
	Status string `json:"status"`
	Time   string `json:"time"`
}

// Handlers holds all HTTP handlers
type Handlers struct {
	bot     *services.LiquidationBot
	monitor *services.LiquidationMonitor
}

// NewHandlers creates a new handlers instance
func NewHandlers(bot *services.LiquidationBot, monitor *services.LiquidationMonitor) *Handlers {
	return &Handlers{
		bot:     bot,
		monitor: monitor,
	}
}

// SetupRoutes sets up all HTTP routes
func (h *Handlers) SetupRoutes(e *echo.Echo) {
	// API Routes
	api := e.Group("/api/v1")

	// Bot control endpoints
	api.POST("/bot/start", h.handleStart)
	api.POST("/bot/stop", h.handleStop)
	api.GET("/bot/status", h.handleStatus)

	// Position and liquidation endpoints
	api.GET("/position/:address", h.handlePosition)
	api.GET("/system/health", h.handleSystemHealth)
	api.GET("/liquidations/recent", h.handleRecentLiquidations)
	api.GET("/liquidations/history", h.handleHistoricalLiquidations)

	// Health check endpoint
	e.GET("/health", h.handleHealthCheck)
}

func (h *Handlers) handleStart(c echo.Context) error {
	if h.bot.IsRunning() {
		return c.JSON(http.StatusConflict, constants.NewErrorResponse("Bot is already running"))
	}

	go h.bot.Start()
	return c.String(http.StatusOK, "Bot started successfully")
}

func (h *Handlers) handleStop(c echo.Context) error {
	if !h.bot.IsRunning() {
		return c.JSON(http.StatusConflict, constants.NewErrorResponse("Bot is not running"))
	}

	h.bot.Stop()
	return c.String(http.StatusOK, "Bot stopped successfully")
}

func (h *Handlers) handleStatus(c echo.Context) error {
	status := h.bot.GetStatus()
	return c.JSON(http.StatusOK, status)
}

func (h *Handlers) handlePosition(c echo.Context) error {
	userAddress := c.Param("address")
	if !common.IsHexAddress(userAddress) {
		return c.JSON(http.StatusBadRequest, constants.NewErrorResponse("Invalid address format"))
	}

	position, err := h.bot.GetPositionInfo(common.HexToAddress(userAddress))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, constants.NewErrorResponse(err.Error()))
	}

	return c.JSON(http.StatusOK, position)
}

func (h *Handlers) handleSystemHealth(c echo.Context) error {
	health, err := h.monitor.GetSystemHealth()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, constants.NewErrorResponse(err.Error()))
	}

	return c.JSON(http.StatusOK, health)
}

func (h *Handlers) handleRecentLiquidations(c echo.Context) error {
	blocksParam := c.QueryParam("blocks")
	blocks := uint64(1000) // default

	if blocksParam != "" {
		if b, err := strconv.ParseUint(blocksParam, 10, 64); err == nil {
			blocks = b
		}
	}

	events, err := h.monitor.GetRecentLiquidations(blocks)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, constants.NewErrorResponse(err.Error()))
	}

	return c.JSON(http.StatusOK, events)
}

func (h *Handlers) handleHistoricalLiquidations(c echo.Context) error {
	limitParam := c.QueryParam("limit")
	limit := 100 // default

	if limitParam != "" {
		if l, err := strconv.Atoi(limitParam); err == nil {
			limit = l
		}
	}

	events, err := h.monitor.GetHistoricalLiquidations(limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, constants.NewErrorResponse(err.Error()))
	}

	return c.JSON(http.StatusOK, events)
}

func (h *Handlers) handleHealthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, StatusResponse{
		Status: "healthy",
		Time:   time.Now().Format(time.RFC3339),
	})
}
