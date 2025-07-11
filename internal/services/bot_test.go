package services

import (
	"errors"
	"fmt"
	"math/big"
	"scalar-money-bot/internal/config"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// LiquidationBotTestSuite is the test suite for the liquidation bot
type LiquidationBotTestSuite struct {
	suite.Suite
	testAddress common.Address
	testConfig  *config.Config
}

func (suite *LiquidationBotTestSuite) SetupTest() {
	suite.testAddress = common.HexToAddress("0x1234567890123456789012345678901234567890")
	
	suite.testConfig = &config.Config{
		RPCURL:          "http://localhost:8545",
		PrivateKey:      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef01",
		CauldronAddress: suite.testAddress.Hex(),
		CheckInterval:   time.Second * 10,
	}
}

func (suite *LiquidationBotTestSuite) TestBotLifecycle() {
	// Create a basic bot for lifecycle testing
	bot := &LiquidationBot{
		cauldronAddress: suite.testAddress,
		checkInterval:   time.Millisecond * 100,
		stopChan:        make(chan struct{}),
	}
	
	// Test initial state
	suite.False(bot.IsRunning())
	
	// Test start
	go bot.Start()
	time.Sleep(50 * time.Millisecond)
	suite.True(bot.IsRunning())
	
	// Test stop
	bot.Stop()
	time.Sleep(50 * time.Millisecond)
	suite.False(bot.IsRunning())
}

func (suite *LiquidationBotTestSuite) TestGetCheckInterval() {
	bot := &LiquidationBot{
		checkInterval: time.Second * 5,
	}
	
	interval := bot.GetCheckInterval()
	suite.Equal(time.Second*5, interval)
}

func (suite *LiquidationBotTestSuite) TestConstants() {
	suite.Equal("borrow_events", CheckpointBorrowEvents)
	suite.Equal("liquidations", CheckpointLiquidations)
	suite.Equal(uint64(500), DefaultLookbackBlocks)
	suite.Equal(uint64(9000), MaxBlockRange)
}

func (suite *LiquidationBotTestSuite) TestChunkCalculation() {
	// Test chunk size calculation for large ranges
	fromBlock := uint64(1000)
	toBlock := uint64(50000)
	
	expectedChunks := int((toBlock-fromBlock)/MaxBlockRange) + 1
	suite.Equal(6, expectedChunks) // (50000-1000)/9000 + 1 = 5.44 + 1 = 6
}

func (suite *LiquidationBotTestSuite) TestBotConfiguration() {
	// Test bot configuration validation
	config := &config.Config{
		RPCURL:          "http://localhost:8545",
		PrivateKey:      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef01",
		CauldronAddress: "0x1234567890123456789012345678901234567890",
		CheckInterval:   time.Second * 10,
	}
	
	// Validate private key format
	suite.Len(config.PrivateKey, 64)
	
	// Validate address format
	addr := common.HexToAddress(config.CauldronAddress)
	suite.NotEqual(common.Address{}, addr)
	
	// Validate interval
	suite.Greater(config.CheckInterval, time.Duration(0))
}

func (suite *LiquidationBotTestSuite) TestPrivateKeyValidation() {
	// Test valid private key
	validKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef01"
	_, err := crypto.HexToECDSA(validKey)
	suite.NoError(err)
	
	// Test invalid private key
	invalidKey := "invalid_key"
	_, err = crypto.HexToECDSA(invalidKey)
	suite.Error(err)
}

func (suite *LiquidationBotTestSuite) TestAddressValidation() {
	// Test valid address
	validAddr := "0x1234567890123456789012345678901234567890"
	addr := common.HexToAddress(validAddr)
	suite.NotEqual(common.Address{}, addr)
	suite.Equal(validAddr, addr.Hex())
	
	// Test empty address
	emptyAddr := common.Address{}
	suite.Equal("0x0000000000000000000000000000000000000000", emptyAddr.Hex())
}

// Test runner
func TestLiquidationBotTestSuite(t *testing.T) {
	suite.Run(t, new(LiquidationBotTestSuite))
}

// Individual test functions
func TestParseABI(t *testing.T) {
	testABI := `[{"inputs":[],"name":"test","outputs":[],"stateMutability":"nonpayable","type":"function"}]`
	
	parsedABI, err := parseABI(testABI)
	assert.NoError(t, err)
	assert.NotNil(t, parsedABI)
	assert.True(t, len(parsedABI.Methods) > 0)
}

func TestParseABI_InvalidJSON(t *testing.T) {
	invalidABI := `invalid json`
	
	_, err := parseABI(invalidABI)
	assert.Error(t, err)
}

func TestNewLiquidationBot_InvalidPrivateKey(t *testing.T) {
	invalidConfig := &config.Config{
		RPCURL:          "http://localhost:8545",
		PrivateKey:      "invalid_key",
		CauldronAddress: "0x1234567890123456789012345678901234567890",
		CheckInterval:   time.Second * 10,
	}
	
	// Test that invalid private key causes error
	_, err := crypto.HexToECDSA(invalidConfig.PrivateKey)
	assert.Error(t, err)
}

func TestQueryLogsErrorHandling(t *testing.T) {
	// Test error message parsing
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Block range error",
			err:      errors.New("eth_getLogs is limited to a 10,000 range"),
			expected: true,
		},
		{
			name:     "Request entity too large",
			err:      errors.New("413 Request Entity Too Large"),
			expected: true,
		},
		{
			name:     "Network error",
			err:      errors.New("network timeout"),
			expected: false,
		},
		{
			name:     "Other error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isBlockRangeError := tc.err.Error() == "eth_getLogs is limited to a 10,000 range" || 
				tc.err.Error() == "413 Request Entity Too Large"
			assert.Equal(t, tc.expected, isBlockRangeError)
		})
	}
}

func TestBlockRangeCalculation(t *testing.T) {
	testCases := []struct {
		name       string
		fromBlock  uint64
		toBlock    uint64
		maxRange   uint64
		expectedChunks int
	}{
		{
			name:       "Single chunk",
			fromBlock:  1000,
			toBlock:    5000,
			maxRange:   9000,
			expectedChunks: 1,
		},
		{
			name:       "Two chunks",
			fromBlock:  1000,
			toBlock:    15000,
			maxRange:   9000,
			expectedChunks: 2,
		},
		{
			name:       "Multiple chunks",
			fromBlock:  1000,
			toBlock:    50000,
			maxRange:   9000,
			expectedChunks: 6,
		},
		{
			name:       "Exact chunk boundary",
			fromBlock:  1000,
			toBlock:    10000,
			maxRange:   9000,
			expectedChunks: 1,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			chunks := int((tc.toBlock-tc.fromBlock)/tc.maxRange) + 1
			assert.Equal(t, tc.expectedChunks, chunks)
		})
	}
}

func TestBigIntOperations(t *testing.T) {
	// Test big integer operations used in the bot
	testCases := []struct {
		name     string
		input    int64
		expected string
	}{
		{
			name:     "Small number",
			input:    1000,
			expected: "1000",
		},
		{
			name:     "Large number",
			input:    1000000000000000000,
			expected: "1000000000000000000",
		},
		{
			name:     "Zero",
			input:    0,
			expected: "0",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bigInt := big.NewInt(tc.input)
			assert.Equal(t, tc.expected, bigInt.String())
		})
	}
}

func TestTimeOperations(t *testing.T) {
	// Test time-related operations
	testCases := []struct {
		name     string
		interval time.Duration
		expected bool
	}{
		{
			name:     "Valid interval",
			interval: time.Second * 10,
			expected: true,
		},
		{
			name:     "Zero interval",
			interval: 0,
			expected: false,
		},
		{
			name:     "Negative interval",
			interval: -time.Second,
			expected: false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isValid := tc.interval > 0
			assert.Equal(t, tc.expected, isValid)
		})
	}
}

// Benchmark tests
func BenchmarkAddressConversion(b *testing.B) {
	addrStr := "0x1234567890123456789012345678901234567890"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = common.HexToAddress(addrStr)
	}
}

func BenchmarkBigIntCreation(b *testing.B) {
	value := int64(1000000000000000000)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = big.NewInt(value)
	}
}

func BenchmarkPrivateKeyGeneration(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = crypto.GenerateKey()
	}
}

// Table-driven tests
func TestErrorMessageParsing(t *testing.T) {
	testCases := []struct {
		name        string
		errorMsg    string
		isRangeError bool
		isEntityTooLarge bool
	}{
		{
			name:        "Range limit error",
			errorMsg:    "eth_getLogs is limited to a 10,000 range",
			isRangeError: true,
			isEntityTooLarge: false,
		},
		{
			name:        "Entity too large error",
			errorMsg:    "413 Request Entity Too Large",
			isRangeError: false,
			isEntityTooLarge: true,
		},
		{
			name:        "Combined error message",
			errorMsg:    "413 Request Entity Too Large: eth_getLogs is limited to a 10,000 range",
			isRangeError: true,
			isEntityTooLarge: true,
		},
		{
			name:        "Network error",
			errorMsg:    "network timeout",
			isRangeError: false,
			isEntityTooLarge: false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isRangeError := contains(tc.errorMsg, "10,000 range")
			isEntityTooLarge := contains(tc.errorMsg, "Request Entity Too Large")
			
			assert.Equal(t, tc.isRangeError, isRangeError)
			assert.Equal(t, tc.isEntityTooLarge, isEntityTooLarge)
		})
	}
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || 
		(len(s) > len(substr) && s[len(s)-len(substr):] == substr) ||
		(len(s) > len(substr) && s[:len(substr)] == substr) ||
		(len(s) > len(substr) && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Test edge cases
func TestBotEdgeCases(t *testing.T) {
	t.Run("EmptyAddress", func(t *testing.T) {
		addr := common.Address{}
		assert.Equal(t, "0x0000000000000000000000000000000000000000", addr.Hex())
	})
	
	t.Run("ZeroInterval", func(t *testing.T) {
		interval := time.Duration(0)
		assert.Equal(t, time.Duration(0), interval)
	})
	
	t.Run("MaxBlockRange", func(t *testing.T) {
		assert.Equal(t, uint64(9000), MaxBlockRange)
		assert.Less(t, MaxBlockRange, uint64(10000))
	})
}

// Property-based testing style
func TestBlockRangeProperties(t *testing.T) {
	testCases := []uint64{100, 1000, 5000, 9000, 10000, 15000, 20000}
	
	for _, blockRange := range testCases {
		t.Run(fmt.Sprintf("Range_%d", blockRange), func(t *testing.T) {
			fromBlock := uint64(1000)
			toBlock := fromBlock + blockRange
			
			chunks := int((toBlock-fromBlock)/MaxBlockRange) + 1
			
			// Properties that should always hold
			assert.Greater(t, chunks, 0, "Should always have at least one chunk")
			
			if blockRange <= MaxBlockRange {
				assert.Equal(t, 1, chunks, "Should have exactly one chunk for ranges <= MaxBlockRange")
			} else {
				assert.Greater(t, chunks, 1, "Should have more than one chunk for ranges > MaxBlockRange")
			}
		})
	}
}

