package services

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"scalar-money-bot/constants"
	"scalar-money-bot/internal/config"
	"scalar-money-bot/internal/models"
	"scalar-money-bot/pkg/evm"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"gorm.io/gorm"
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
	db              *gorm.DB
}

// NewLiquidationBot creates a new liquidation bot instance
func NewLiquidationBot(cfg *config.Config, db *gorm.DB) (*LiquidationBot, error) {
	client, err := evm.NewClient(cfg.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to EVM client: %v", err)
	}

	fmt.Printf("config: %+v\n", cfg)

	privateKey, err := crypto.HexToECDSA(cfg.PrivateKey)
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

	cauldronAddr := common.HexToAddress(cfg.CauldronAddress)
	cauldron := bind.NewBoundContract(cauldronAddr, cauldronABI, client, client, client)

	return &LiquidationBot{
		client:          client,
		cauldronAddress: cauldronAddr,
		cauldronABI:     cauldronABI,
		privateKey:      privateKey,
		auth:            auth,
		cauldron:        cauldron,
		checkInterval:   cfg.CheckInterval,
		stopChan:        make(chan struct{}),
		db:              db,
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

	lb.logOperation("info", "Liquidation bot started")

	ticker := time.NewTicker(lb.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-lb.stopChan:
			lb.logOperation("info", "Liquidation bot stopped")
			return
		case <-ticker.C:
			if err := lb.checkForLiquidations(); err != nil {
				lb.logOperation("error", fmt.Sprintf("Error in liquidation check: %v", err))
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

// GetCheckInterval returns the check interval
func (lb *LiquidationBot) GetCheckInterval() time.Duration {
	return lb.checkInterval
}

// checkForLiquidations checks for liquidation opportunities
func (lb *LiquidationBot) checkForLiquidations() error {
	lb.logOperation("info", "Checking for liquidation opportunities")

	users, err := lb.getAllBorrowers()
	if err != nil {
		return fmt.Errorf("failed to get borrowers: %v", err)
	}

	var insolventUsers []common.Address
	for _, user := range users {
		isInsolvent, err := lb.isUserInsolvent(user)
		if err != nil {
			lb.logOperation("error", fmt.Sprintf("Error checking solvency for %s: %v", user.Hex(), err))
			continue
		}
		if isInsolvent {
			insolventUsers = append(insolventUsers, user)
		}
	}

	if len(insolventUsers) > 0 {
		lb.logOperation("info", fmt.Sprintf("Found %d insolvent users", len(insolventUsers)))
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
	lb.logOperation("info", fmt.Sprintf("Attempting to liquidate %d users", len(users)))

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

	lb.logOperation("info", fmt.Sprintf("Liquidation transaction sent: %s", tx.Hash().Hex()))
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
func (lb *LiquidationBot) GetPositionInfo(userAddress common.Address) (*models.UserPosition, error) {
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

	// Get total borrow
	err = lb.cauldron.Call(&bind.CallOpts{}, &totalBorrowElastic, "totalBorrow")
	if err != nil {
		return nil, fmt.Errorf("failed to get total borrow: %v", err)
	}

	// Get exchange rate
	err = lb.cauldron.Call(&bind.CallOpts{}, &exchangeRate, "exchangeRate")
	if err != nil {
		return nil, fmt.Errorf("failed to get exchange rate: %v", err)
	}

	// Calculate borrow amount
	borrowAmount := new(big.Int).Set(borrowPart[0].(*big.Int))

	// Check if user is insolvent
	isInsolvent, err := lb.isUserInsolvent(userAddress)
	if err != nil {
		isInsolvent = false // Default to false if we can't check
	}

	position := &models.UserPosition{
		User:            userAddress.Hex(),
		CollateralShare: collateralShare[0].(*big.Int).String(),
		BorrowPart:      borrowPart[0].(*big.Int).String(),
		BorrowAmount:    borrowAmount.String(),
		ExchangeRate:    exchangeRate[0].(*big.Int).String(),
		IsInsolvent:     isInsolvent,
	}

	// Save to database
	lb.db.Create(position)

	return position, nil
}

// logOperation logs bot operations to database
func (lb *LiquidationBot) logOperation(level, message string) {
	log.Printf("[%s] %s", level, message)

	logEntry := &models.BotLog{
		Level:   level,
		Message: message,
	}
	lb.db.Create(logEntry)
}

// parseABI parses the ABI from JSON string
func parseABI(abiJSON string) (abi.ABI, error) {
	return abi.JSON(strings.NewReader(abiJSON))
}
