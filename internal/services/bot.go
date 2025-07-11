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

const (
	// Checkpoint names for different operations
	CheckpointBorrowEvents = "borrow_events"
	CheckpointLiquidations = "liquidations"

	// Default number of blocks to look back if no checkpoint exists
	DefaultLookbackBlocks = 10000
)

// LiquidationBot represents the main liquidation bot
type LiquidationBot struct {
	client              *ethclient.Client
	cauldronAddress     common.Address
	cauldronABI         abi.ABI
	privateKey          *ecdsa.PrivateKey
	auth                *bind.TransactOpts
	cauldron            *bind.BoundContract
	isRunning           bool
	checkInterval       time.Duration
	mutex               sync.RWMutex
	stopChan            chan struct{}
	db                  *gorm.DB
	checkpointRepo      *models.ProcessingCheckpointRepository
	contractDeployBlock uint64 // Block number when contract was deployed
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

	lb := &LiquidationBot{
		client:          client,
		cauldronAddress: cauldronAddr,
		cauldronABI:     cauldronABI,
		privateKey:      privateKey,
		auth:            auth,
		cauldron:        cauldron,
		checkInterval:   cfg.CheckInterval,
		stopChan:        make(chan struct{}),
		db:              db,
		checkpointRepo:  models.NewProcessingCheckpointRepository(db),
	}

	// Get contract deployment block automatically
	contractDeployBlock, err := lb.getContractDeploymentBlock()
	if err != nil {
		return nil, fmt.Errorf("failed to get contract deployment block: %v", err)
	}
	lb.contractDeployBlock = contractDeployBlock

	// Initialize checkpoints if they don't exist
	if err := lb.initializeCheckpoints(); err != nil {
		return nil, fmt.Errorf("failed to initialize checkpoints: %v", err)
	}

	return lb, nil
}

// getContractDeploymentBlock finds the block where the contract was deployed
func (lb *LiquidationBot) getContractDeploymentBlock() (uint64, error) {
	// Check if we have a cached deployment block in checkpoints
	checkpoint, err := lb.checkpointRepo.GetCheckpoint("contract_deployment")
	if err == nil && checkpoint != nil {
		lb.logOperation("info", fmt.Sprintf("Using cached contract deployment block: %d", checkpoint.BlockNumber))
		return checkpoint.BlockNumber, nil
	}

	lb.logOperation("info", "Detecting contract deployment block...")

	// Get contract code to verify it exists
	code, err := lb.client.CodeAt(context.Background(), lb.cauldronAddress, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get contract code: %v", err)
	}
	if len(code) == 0 {
		return 0, fmt.Errorf("no contract found at address %s", lb.cauldronAddress.Hex())
	}

	// Binary search to find the deployment block
	currentBlock, err := lb.client.BlockNumber(context.Background())
	if err != nil {
		return 0, fmt.Errorf("failed to get current block: %v", err)
	}

	deploymentBlock, err := lb.binarySearchDeploymentBlock(0, currentBlock)
	if err != nil {
		return 0, fmt.Errorf("failed to find deployment block: %v", err)
	}

	// Cache the deployment block for future use
	err = lb.checkpointRepo.UpdateCheckpoint("contract_deployment", deploymentBlock, "")
	if err != nil {
		lb.logOperation("error", fmt.Sprintf("Failed to cache deployment block: %v", err))
	}

	lb.logOperation("info", fmt.Sprintf("Contract deployment block detected: %d", deploymentBlock))
	return deploymentBlock, nil
}

// binarySearchDeploymentBlock uses binary search to find the deployment block
func (lb *LiquidationBot) binarySearchDeploymentBlock(low, high uint64) (uint64, error) {
	for low < high {
		mid := (low + high) / 2

		// Check if contract exists at this block
		code, err := lb.client.CodeAt(context.Background(), lb.cauldronAddress, big.NewInt(int64(mid)))
		if err != nil {
			return 0, fmt.Errorf("failed to check code at block %d: %v", mid, err)
		}

		if len(code) > 0 {
			// Contract exists, search in the lower half
			high = mid
		} else {
			// Contract doesn't exist, search in the upper half
			low = mid + 1
		}

		// Add a small delay to avoid overwhelming the RPC
		time.Sleep(100 * time.Millisecond)
	}

	// Verify the found block
	code, err := lb.client.CodeAt(context.Background(), lb.cauldronAddress, big.NewInt(int64(low)))
	if err != nil {
		return 0, fmt.Errorf("failed to verify deployment block %d: %v", low, err)
	}
	if len(code) == 0 {
		return 0, fmt.Errorf("contract not found at calculated deployment block %d", low)
	}

	return low, nil
}

func (lb *LiquidationBot) initializeCheckpoints() error {
	// Get current block number
	currentBlock, err := lb.client.BlockNumber(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get current block: %v", err)
	}

	// Determine starting block
	var startBlock uint64
	if lb.contractDeployBlock > 0 {
		startBlock = lb.contractDeployBlock
	} else {
		// Default to current block minus default lookback
		if currentBlock >= DefaultLookbackBlocks {
			startBlock = currentBlock - DefaultLookbackBlocks
		} else {
			startBlock = 0
		}
	}

	// Initialize borrow events checkpoint
	err = lb.checkpointRepo.SetCheckpointIfNotExists(CheckpointBorrowEvents, startBlock)
	if err != nil {
		return fmt.Errorf("failed to set borrow events checkpoint: %v", err)
	}

	// Initialize liquidations checkpoint
	err = lb.checkpointRepo.SetCheckpointIfNotExists(CheckpointLiquidations, startBlock)
	if err != nil {
		return fmt.Errorf("failed to set liquidations checkpoint: %v", err)
	}

	lb.logOperation("info", fmt.Sprintf("Initialized checkpoints with start block: %d", startBlock))
	return nil
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

// getAllBorrowers retrieves all borrowers from events using checkpoints
func (lb *LiquidationBot) getAllBorrowers() ([]common.Address, error) {
	// Get the latest block
	latestBlock, err := lb.client.BlockNumber(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get latest block: %v", err)
	}

	// Get last processed block from checkpoint
	checkpoint, err := lb.checkpointRepo.GetCheckpoint(CheckpointBorrowEvents)
	if err != nil {
		return nil, fmt.Errorf("failed to get borrow events checkpoint: %v", err)
	}

	var fromBlock uint64
	if checkpoint != nil {
		fromBlock = checkpoint.BlockNumber + 1 // Start from next block after last processed
	} else {
		// This shouldn't happen if initialization worked correctly
		if latestBlock >= DefaultLookbackBlocks {
			fromBlock = latestBlock - DefaultLookbackBlocks
		} else {
			fromBlock = 0
		}
	}

	// Don't query if we're already up to date
	if fromBlock > latestBlock {
		lb.logOperation("info", "No new blocks to process for borrow events")
		return lb.getCachedBorrowers(), nil
	}

	lb.logOperation("info", fmt.Sprintf("Processing borrow events from block %d to %d", fromBlock, latestBlock))

	// Query LogBorrow events
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

	// Update checkpoint
	err = lb.checkpointRepo.UpdateCheckpoint(CheckpointBorrowEvents, latestBlock, "")
	if err != nil {
		lb.logOperation("error", fmt.Sprintf("Failed to update borrow events checkpoint: %v", err))
	}

	// Get all unique borrowers (including previously cached ones)
	allBorrowers := lb.getCachedBorrowers()
	for user := range userSet {
		found := false
		for _, existing := range allBorrowers {
			if existing == user {
				found = true
				break
			}
		}
		if !found {
			allBorrowers = append(allBorrowers, user)
		}
	}

	lb.logOperation("info", fmt.Sprintf("Found %d total borrowers (%d new from recent events)", len(allBorrowers), len(userSet)))
	return allBorrowers, nil
}

// getCachedBorrowers gets borrowers from historical data (you might want to cache this)
func (lb *LiquidationBot) getCachedBorrowers() []common.Address {
	// For now, return empty slice. You might want to implement caching
	// or query from a separate table that stores all known borrowers
	return []common.Address{}
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

// GetCheckpointStatus returns the current checkpoint status
func (lb *LiquidationBot) GetCheckpointStatus() (map[string]*models.ProcessingCheckpoint, error) {
	checkpoints, err := lb.checkpointRepo.GetAllCheckpoints()
	if err != nil {
		return nil, fmt.Errorf("failed to get checkpoints: %v", err)
	}

	result := make(map[string]*models.ProcessingCheckpoint)
	for i := range checkpoints {
		result[checkpoints[i].Name] = &checkpoints[i]
	}

	return result, nil
}

// ResetCheckpoint resets a specific checkpoint to a given block number
func (lb *LiquidationBot) ResetCheckpoint(name string, blockNumber uint64) error {
	if lb.isRunning {
		return fmt.Errorf("cannot reset checkpoint while bot is running")
	}

	err := lb.checkpointRepo.UpdateCheckpoint(name, blockNumber, "")
	if err != nil {
		return fmt.Errorf("failed to reset checkpoint %s: %v", name, err)
	}

	lb.logOperation("info", fmt.Sprintf("Reset checkpoint %s to block %d", name, blockNumber))
	return nil
}

// GetContractDeploymentBlock returns the cached deployment block
func (lb *LiquidationBot) GetContractDeploymentBlock() (uint64, error) {
	return lb.getContractDeploymentBlock()
}

// LogOperation exposes the internal logOperation method for handlers
func (lb *LiquidationBot) LogOperation(level, message string) {
	lb.logOperation(level, message)
}
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
