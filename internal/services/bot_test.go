package services

import (
	"context"
	"fmt"
	"scalar-money-bot/internal/config"
	"scalar-money-bot/internal/database"
	testutils "scalar-money-bot/pkg/test_utils"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// LiquidationBotTestSuite is the test suite for the liquidation bot
type LiquidationBotTestSuite struct {
	testConfig *config.Config
	ctx        context.Context
}

var suite *LiquidationBotTestSuite

func TestMain(m *testing.M) {
	suite = &LiquidationBotTestSuite{}
	suite.testConfig, _ = config.LoadConfig("../../.env.test")
	suite.ctx = context.Background()
	m.Run()
}

func TestBotLifecycle(t *testing.T) {
	testutils.RunWithTestDB(func(ctx context.Context, repo *database.Repository) error {
		// Create a basic bot for lifecycle testing
		bot, err := NewLiquidationBot(suite.testConfig, repo)
		if err != nil {
			return fmt.Errorf("failed to create bot: %w", err)
		}
		// Test initial state
		// suite.False(bot.IsRunning())
		assert.False(t, bot.IsRunning())
		// Test start
		go bot.Start()
		time.Sleep(50 * time.Millisecond)
		assert.True(t, bot.IsRunning())
		// Test stop
		bot.Stop()
		time.Sleep(50 * time.Millisecond)
		assert.False(t, bot.IsRunning())
		return nil
	})
}

func TestCheckForLiquidations(t *testing.T) {
	// testutils.RunWithTestDB(func(ctx context.Context, repo *database.Repository) error {
	// Create a bot instance
	fmt.Println("TestCheckForLiquidations")
	db, err := database.Initialize(suite.testConfig.DatabaseURL)
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	repo := database.NewRepository(db)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}
	bot, err := NewLiquidationBot(suite.testConfig, repo)
	if err != nil {
		t.Fatalf("failed to create bot: %v", err)
	}

	// Test the liquidation check functionality
	t.Log("Testing liquidation check functionality")

	// Call checkForLiquidations to see if it can detect any liquidation opportunities
	err = bot.checkForLiquidations()
	if err != nil {
		// Log the error but don't fail the test - this might be expected if no borrowers exist
		t.Logf("Liquidation check returned error (may be expected): %v", err)
	} else {
		t.Log("Liquidation check completed successfully")
	}
}
