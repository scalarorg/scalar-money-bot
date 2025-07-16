package services

import (
	"context"
	"fmt"
	"math/big"
	"scalar-money-bot/constants"
	"scalar-money-bot/internal/config"
	"scalar-money-bot/internal/database"
	"scalar-money-bot/pkg/evm"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// LiquidationMonitor handles monitoring and dashboard functionality
type LiquidationMonitor struct {
	client          *ethclient.Client
	cauldronAddress common.Address
	cauldronABI     abi.ABI
	cauldron        *bind.BoundContract
	repo            *database.Repository
}

// NewLiquidationMonitor creates a new liquidation monitor
func NewLiquidationMonitor(cfg *config.Config, repo *database.Repository) (*LiquidationMonitor, error) {
	client, err := evm.NewClient(cfg.RpcUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ethereum client: %v", err)
	}

	cauldronABI, err := abi.JSON(strings.NewReader(constants.CauldronABI))
	if err != nil {
		return nil, fmt.Errorf("failed to parse cauldron ABI: %v", err)
	}

	cauldronAddr := common.HexToAddress(cfg.CauldronAddress)
	cauldron := bind.NewBoundContract(cauldronAddr, cauldronABI, client, client, client)

	return &LiquidationMonitor{
		client:          client,
		cauldronAddress: cauldronAddr,
		cauldronABI:     cauldronABI,
		cauldron:        cauldron,
		repo:            repo,
	}, nil
}

// GetSystemHealth retrieves system health information
func (lm *LiquidationMonitor) GetSystemHealth() (*database.SystemHealth, error) {
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

	health := &database.SystemHealth{
		TotalBorrowElastic:    totalBorrow[0].(*big.Int).String(),
		TotalBorrowBase:       totalBorrow[1].(*big.Int).String(),
		TotalCollateralShare:  totalCollateralShare[0].(*big.Int).String(),
		ExchangeRate:          exchangeRate[0].(*big.Int).String(),
		LiquidationMultiplier: liquidationMultiplier[0].(*big.Int).String(),
		CollateralizationRate: collateralizationRate[0].(*big.Int).String(),
	}

	// Save to database
	lm.repo.Create(context.Background(), health)

	return health, nil
}

// GetRecentLiquidations retrieves recent liquidation events
func (lm *LiquidationMonitor) GetRecentLiquidations(blocks uint64) ([]*database.LiquidationEvent, error) {
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

	var events []*database.LiquidationEvent
	for _, vLog := range logs {
		if len(vLog.Topics) >= 4 {
			event := &database.LiquidationEvent{
				TxHash:      vLog.TxHash.Hex(),
				BlockNumber: vLog.BlockNumber,
				Liquidator:  common.BytesToAddress(vLog.Topics[1].Bytes()).Hex(),
				User:        common.BytesToAddress(vLog.Topics[2].Bytes()).Hex(),
				To:          common.BytesToAddress(vLog.Topics[3].Bytes()).Hex(),
			}

			// Save to database
			lm.repo.FirstOrCreate(context.Background(), event, database.LiquidationEvent{TxHash: event.TxHash})
			events = append(events, event)
		}
	}

	return events, nil
}

// GetHistoricalLiquidations retrieves liquidation events from database
func (lm *LiquidationMonitor) GetHistoricalLiquidations(limit int) ([]*database.LiquidationEvent, error) {
	var events []*database.LiquidationEvent

	result := lm.repo.Raw().Order("created_at desc").Limit(limit).Find(&events)
	if result.Error != nil {
		return nil, result.Error
	}

	return events, nil
}
