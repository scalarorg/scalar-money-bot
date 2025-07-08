package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"scalar-money-bot/constants"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// LiquidationBot represents the main liquidation bot
type LiquidationBot struct {
	client          *ethclient.Client
	cauldronAddress common.Address
	cauldronABI     abi.ABI
	privateKey      *ecdsa.PrivateKey
	auth            *bind.TransactOpts
	cauldron        *bind.BoundContract
	isRunning       bool
	checkInterval   time.Duration
	mutex           sync.RWMutex
	stopChan        chan struct{}
}

// LiquidationMonitor handles monitoring and dashboard functionality
type LiquidationMonitor struct {
	client          *ethclient.Client
	cauldronAddress common.Address
	cauldronABI     abi.ABI
	cauldron        *bind.BoundContract
}

// UserPosition represents a user's position information
type UserPosition struct {
	User            string `json:"user"`
	CollateralShare string `json:"collateralShare"`
	BorrowPart      string `json:"borrowPart"`
	BorrowAmount    string `json:"borrowAmount"`
	ExchangeRate    string `json:"exchangeRate"`
}

// SystemHealth represents the overall system health
type SystemHealth struct {
	TotalBorrowElastic    string `json:"totalBorrowElastic"`
	TotalBorrowBase       string `json:"totalBorrowBase"`
	TotalCollateralShare  string `json:"totalCollateralShare"`
	ExchangeRate          string `json:"exchangeRate"`
	LiquidationMultiplier string `json:"liquidationMultiplier"`
	CollateralizationRate string `json:"collateralizationRate"`
}

// LiquidationEvent represents a liquidation event
type LiquidationEvent struct {
	TxHash          string `json:"txHash"`
	BlockNumber     uint64 `json:"blockNumber"`
	Liquidator      string `json:"liquidator"`
	User            string `json:"user"`
	To              string `json:"to"`
	CollateralShare string `json:"collateralShare"`
	BorrowAmount    string `json:"borrowAmount"`
	BorrowPart      string `json:"borrowPart"`
}

// BotStatus represents the bot's current status
type BotStatus struct {
	IsRunning     bool   `json:"isRunning"`
	CheckInterval string `json:"checkInterval"`
	LastCheck     string `json:"lastCheck"`
}

// Config holds all configuration values
type Config struct {
	RPCURL          string
	CauldronAddress string
	PrivateKey      string
	CheckInterval   time.Duration
	Port            string
	LogLevel        string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	config := &Config{
		RPCURL:          getEnv("RPC_URL", ""),
		CauldronAddress: getEnv("CAULDRON_ADDRESS", ""),
		PrivateKey:      getEnv("PRIVATE_KEY", ""),
		Port:            getEnv("PORT", "8080"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
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

// parseABI parses the ABI from JSON string
func parseABI(abiJSON string) (abi.ABI, error) {
	return abi.JSON(strings.NewReader(abiJSON))
}

// NewLiquidationBot creates a new liquidation bot instance
func NewLiquidationBot(config *Config) (*LiquidationBot, error) {
	client, err := ethclient.Dial(config.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ethereum client: %v", err)
	}

	privateKey, err := crypto.HexToECDSA(config.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %v", err)
	}

	cauldronABI, err := parseABI(constants.CauldronABI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cauldron ABI: %v", err)
	}

	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get network ID: %v", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create transactor: %v", err)
	}

	cauldronAddr := common.HexToAddress(config.CauldronAddress)
	cauldron := bind.NewBoundContract(cauldronAddr, cauldronABI, client, client, client)

	return &LiquidationBot{
		client:          client,
		cauldronAddress: cauldronAddr,
		cauldronABI:     cauldronABI,
		privateKey:      privateKey,
		auth:            auth,
		cauldron:        cauldron,
		checkInterval:   config.CheckInterval,
		stopChan:        make(chan struct{}),
	}, nil
}

// Start begins the liquidation bot
func (lb *LiquidationBot) Start() {
	lb.mutex.Lock()
	if lb.isRunning {
		lb.mutex.Unlock()
		return
	}
	lb.isRunning = true
	lb.mutex.Unlock()

	log.Println("Liquidation bot started...")

	ticker := time.NewTicker(lb.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-lb.stopChan:
			log.Println("Liquidation bot stopped.")
			return
		case <-ticker.C:
			if err := lb.checkForLiquidations(); err != nil {
				log.Printf("Error in liquidation check: %v", err)
			}
		}
	}
}

// Stop stops the liquidation bot
func (lb *LiquidationBot) Stop() {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	if !lb.isRunning {
		return
	}

	lb.isRunning = false
	close(lb.stopChan)
	lb.stopChan = make(chan struct{})
}

// IsRunning returns the current running status
func (lb *LiquidationBot) IsRunning() bool {
	lb.mutex.RLock()
	defer lb.mutex.RUnlock()
	return lb.isRunning
}

// checkForLiquidations checks for liquidation opportunities
func (lb *LiquidationBot) checkForLiquidations() error {
	log.Println("Checking for liquidation opportunities...")

	users, err := lb.getAllBorrowers()
	if err != nil {
		return fmt.Errorf("failed to get borrowers: %v", err)
	}

	var insolventUsers []common.Address
	for _, user := range users {
		isInsolvent, err := lb.isUserInsolvent(user)
		if err != nil {
			log.Printf("Error checking solvency for %s: %v", user.Hex(), err)
			continue
		}
		if isInsolvent {
			insolventUsers = append(insolventUsers, user)
		}
	}

	if len(insolventUsers) > 0 {
		log.Printf("Found %d insolvent users", len(insolventUsers))
		return lb.liquidateUsers(insolventUsers)
	}

	return nil
}

// isUserInsolvent checks if a user is insolvent
func (lb *LiquidationBot) isUserInsolvent(userAddress common.Address) (bool, error) {
	// Update exchange rate first
	_, err := lb.cauldron.Transact(lb.auth, "updateExchangeRate")
	if err != nil {
		return false, fmt.Errorf("failed to update exchange rate: %v", err)
	}

	// Check if user is solvent
	var result []interface{}
	err = lb.cauldron.Call(&bind.CallOpts{}, &result, "isSolvent", userAddress)
	if err != nil {
		return false, fmt.Errorf("failed to check solvency: %v", err)
	}

	if len(result) == 0 {
		return false, fmt.Errorf("no result from isSolvent call")
	}

	isSolvent, ok := result[0].(bool)
	if !ok {
		return false, fmt.Errorf("unexpected result type from isSolvent")
	}

	return !isSolvent, nil
}

// liquidateUsers performs liquidation on insolvent users
func (lb *LiquidationBot) liquidateUsers(users []common.Address) error {
	log.Printf("Attempting to liquidate %d users", len(users))

	// Get maximum borrow parts for each user
	maxBorrowParts := make([]*big.Int, len(users))
	for i, user := range users {
		var result []interface{}
		err := lb.cauldron.Call(&bind.CallOpts{}, &result, "userBorrowPart", user)
		if err != nil {
			return fmt.Errorf("failed to get borrow part for user %s: %v", user.Hex(), err)
		}

		if len(result) == 0 {
			return fmt.Errorf("no result from userBorrowPart call")
		}

		borrowPart, ok := result[0].(*big.Int)
		if !ok {
			return fmt.Errorf("unexpected result type from userBorrowPart")
		}

		maxBorrowParts[i] = borrowPart
	}

	// Execute liquidation
	tx, err := lb.cauldron.Transact(lb.auth, "liquidate",
		users,
		maxBorrowParts,
		lb.auth.From,
		common.Address{}, // no swapper
		[]byte{})         // no swapper data

	if err != nil {
		return fmt.Errorf("liquidation transaction failed: %v", err)
	}

	log.Printf("Liquidation transaction sent: %s", tx.Hash().Hex())
	return nil
}

// getAllBorrowers retrieves all borrowers from events
func (lb *LiquidationBot) getAllBorrowers() ([]common.Address, error) {
	// Get the latest block
	latestBlock, err := lb.client.BlockNumber(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get latest block: %v", err)
	}

	// Query LogBorrow events from the last 10k blocks
	fromBlock := uint64(0)
	if latestBlock >= 10000 {
		fromBlock = latestBlock - 10000
	}

	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(fromBlock)),
		ToBlock:   big.NewInt(int64(latestBlock)),
		Addresses: []common.Address{lb.cauldronAddress},
		Topics:    [][]common.Hash{{crypto.Keccak256Hash([]byte("LogBorrow(address,address,uint256,uint256)"))}},
	}

	logs, err := lb.client.FilterLogs(context.Background(), query)
	if err != nil {
		return nil, fmt.Errorf("failed to filter logs: %v", err)
	}

	userSet := make(map[common.Address]bool)
	for _, vLog := range logs {
		if len(vLog.Topics) >= 2 {
			userSet[common.BytesToAddress(vLog.Topics[1].Bytes())] = true
		}
	}

	var users []common.Address
	for user := range userSet {
		users = append(users, user)
	}

	return users, nil
}

// GetPositionInfo retrieves position information for a user
func (lb *LiquidationBot) GetPositionInfo(userAddress common.Address) (*UserPosition, error) {
	var collateralShare, borrowPart, totalBorrowElastic, exchangeRate []interface{}

	// Get user collateral share
	err := lb.cauldron.Call(&bind.CallOpts{}, &collateralShare, "userCollateralShare", userAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get collateral share: %v", err)
	}

	// Get user borrow part
	err = lb.cauldron.Call(&bind.CallOpts{}, &borrowPart, "userBorrowPart", userAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get borrow part: %v", err)
	}

	// Get total borrow (assuming it returns a struct with elastic and base)
	err = lb.cauldron.Call(&bind.CallOpts{}, &totalBorrowElastic, "totalBorrow")
	if err != nil {
		return nil, fmt.Errorf("failed to get total borrow: %v", err)
	}

	// Get exchange rate
	err = lb.cauldron.Call(&bind.CallOpts{}, &exchangeRate, "exchangeRate")
	if err != nil {
		return nil, fmt.Errorf("failed to get exchange rate: %v", err)
	}

	// Calculate borrow amount (simplified calculation)
	borrowAmount := new(big.Int).Set(borrowPart[0].(*big.Int))

	return &UserPosition{
		User:            userAddress.Hex(),
		CollateralShare: collateralShare[0].(*big.Int).String(),
		BorrowPart:      borrowPart[0].(*big.Int).String(),
		BorrowAmount:    borrowAmount.String(),
		ExchangeRate:    exchangeRate[0].(*big.Int).String(),
	}, nil
}

// NewLiquidationMonitor creates a new liquidation monitor
func NewLiquidationMonitor(config *Config) (*LiquidationMonitor, error) {
	client, err := ethclient.Dial(config.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ethereum client: %v", err)
	}

	cauldronABI, err := parseABI(constants.CauldronABI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cauldron ABI: %v", err)
	}

	cauldronAddr := common.HexToAddress(config.CauldronAddress)
	cauldron := bind.NewBoundContract(cauldronAddr, cauldronABI, client, client, client)

	return &LiquidationMonitor{
		client:          client,
		cauldronAddress: cauldronAddr,
		cauldronABI:     cauldronABI,
		cauldron:        cauldron,
	}, nil
}

// GetSystemHealth retrieves system health information
func (lm *LiquidationMonitor) GetSystemHealth() (*SystemHealth, error) {
	var (
		totalBorrow           []interface{}
		totalCollateralShare  []interface{}
		exchangeRate          []interface{}
		liquidationMultiplier []interface{}
		collateralizationRate []interface{}
	)

	// Get total borrow
	err := lm.cauldron.Call(&bind.CallOpts{}, &totalBorrow, "totalBorrow")
	if err != nil {
		return nil, fmt.Errorf("failed to get total borrow: %v", err)
	}

	// Get total collateral share
	err = lm.cauldron.Call(&bind.CallOpts{}, &totalCollateralShare, "totalCollateralShare")
	if err != nil {
		return nil, fmt.Errorf("failed to get total collateral share: %v", err)
	}

	// Get exchange rate
	err = lm.cauldron.Call(&bind.CallOpts{}, &exchangeRate, "exchangeRate")
	if err != nil {
		return nil, fmt.Errorf("failed to get exchange rate: %v", err)
	}

	// Get liquidation multiplier
	err = lm.cauldron.Call(&bind.CallOpts{}, &liquidationMultiplier, "LIQUIDATION_MULTIPLIER")
	if err != nil {
		return nil, fmt.Errorf("failed to get liquidation multiplier: %v", err)
	}

	// Get collateralization rate
	err = lm.cauldron.Call(&bind.CallOpts{}, &collateralizationRate, "COLLATERIZATION_RATE")
	if err != nil {
		return nil, fmt.Errorf("failed to get collateralization rate: %v", err)
	}

	return &SystemHealth{
		TotalBorrowElastic:    totalBorrow[0].(*big.Int).String(),
		TotalBorrowBase:       totalBorrow[1].(*big.Int).String(),
		TotalCollateralShare:  totalCollateralShare[0].(*big.Int).String(),
		ExchangeRate:          exchangeRate[0].(*big.Int).String(),
		LiquidationMultiplier: liquidationMultiplier[0].(*big.Int).String(),
		CollateralizationRate: collateralizationRate[0].(*big.Int).String(),
	}, nil
}

// GetRecentLiquidations retrieves recent liquidation events
func (lm *LiquidationMonitor) GetRecentLiquidations(blocks uint64) ([]*LiquidationEvent, error) {
	latestBlock, err := lm.client.BlockNumber(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get latest block: %v", err)
	}

	fromBlock := uint64(0)
	if latestBlock > blocks {
		fromBlock = latestBlock - blocks
	}

	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(fromBlock)),
		ToBlock:   big.NewInt(int64(latestBlock)),
		Addresses: []common.Address{lm.cauldronAddress},
		Topics:    [][]common.Hash{{crypto.Keccak256Hash([]byte("LogLiquidation(address,address,address,uint256,uint256,uint256)"))}},
	}

	logs, err := lm.client.FilterLogs(context.Background(), query)
	if err != nil {
		return nil, fmt.Errorf("failed to filter logs: %v", err)
	}

	var events []*LiquidationEvent
	for _, vLog := range logs {
		if len(vLog.Topics) >= 4 {
			event := &LiquidationEvent{
				TxHash:      vLog.TxHash.Hex(),
				BlockNumber: vLog.BlockNumber,
				Liquidator:  common.BytesToAddress(vLog.Topics[1].Bytes()).Hex(),
				User:        common.BytesToAddress(vLog.Topics[2].Bytes()).Hex(),
				To:          common.BytesToAddress(vLog.Topics[3].Bytes()).Hex(),
				// Data field would contain the uint256 values
			}
			events = append(events, event)
		}
	}

	return events, nil
}

// HTTP Handlers
func (lb *LiquidationBot) handleStart(c echo.Context) error {
	if lb.IsRunning() {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Bot is already running",
		})
	}

	go lb.Start()
	return c.JSON(http.StatusOK, map[string]string{
		"message": "Bot started successfully",
	})
}

func (lb *LiquidationBot) handleStop(c echo.Context) error {
	if !lb.IsRunning() {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Bot is not running",
		})
	}

	lb.Stop()
	return c.JSON(http.StatusOK, map[string]string{
		"message": "Bot stopped successfully",
	})
}

func (lb *LiquidationBot) handleStatus(c echo.Context) error {
	status := &BotStatus{
		IsRunning:     lb.IsRunning(),
		CheckInterval: lb.checkInterval.String(),
		LastCheck:     time.Now().Format(time.RFC3339),
	}
	return c.JSON(http.StatusOK, status)
}

func (lb *LiquidationBot) handlePosition(c echo.Context) error {
	userAddress := c.Param("address")
	if !common.IsHexAddress(userAddress) {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid address format",
		})
	}

	position, err := lb.GetPositionInfo(common.HexToAddress(userAddress))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, position)
}

func (lm *LiquidationMonitor) handleSystemHealth(c echo.Context) error {
	health, err := lm.GetSystemHealth()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, health)
}

func (lm *LiquidationMonitor) handleRecentLiquidations(c echo.Context) error {
	blocksParam := c.QueryParam("blocks")
	blocks := uint64(1000) // default

	if blocksParam != "" {
		if b, err := strconv.ParseUint(blocksParam, 10, 64); err == nil {
			blocks = b
		}
	}

	events, err := lm.GetRecentLiquidations(blocks)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, events)
}

func main() {
	// Load configuration
	config, err := LoadConfig()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// Set log level
	if config.LogLevel == "debug" {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	log.Printf("Starting liquidation bot with config: RPC=%s, Cauldron=%s, Port=%s",
		config.RPCURL, config.CauldronAddress, config.Port)

	// Initialize bot and monitor
	bot, err := NewLiquidationBot(config)
	if err != nil {
		log.Fatal("Failed to create liquidation bot:", err)
	}

	monitor, err := NewLiquidationMonitor(config)
	if err != nil {
		log.Fatal("Failed to create liquidation monitor:", err)
	}

	// Setup Echo
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// API Routes
	api := e.Group("/api/v1")

	// Bot control endpoints
	api.POST("/bot/start", bot.handleStart)
	api.POST("/bot/stop", bot.handleStop)
	api.GET("/bot/status", bot.handleStatus)

	// Position and liquidation endpoints
	api.GET("/position/:address", bot.handlePosition)
	api.GET("/system/health", monitor.handleSystemHealth)
	api.GET("/liquidations/recent", monitor.handleRecentLiquidations)

	// Health check endpoint
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"status": "healthy",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	// Start server
	log.Printf("Starting server on :%s", config.Port)
	if err := e.Start(":" + config.Port); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
