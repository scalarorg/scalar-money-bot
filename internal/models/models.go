package models

import (
	"time"

	"gorm.io/gorm"
)

// UserPosition represents a user's position information
type UserPosition struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	User            string    `gorm:"index:idx_user_positions_user;size:42;not null" json:"user"`
	CollateralShare string    `gorm:"type:text;not null" json:"collateralShare"`
	BorrowPart      string    `gorm:"type:text;not null" json:"borrowPart"`
	BorrowAmount    string    `gorm:"type:text;not null" json:"borrowAmount"`
	ExchangeRate    string    `gorm:"type:text;not null" json:"exchangeRate"`
	IsInsolvent     bool      `gorm:"default:false" json:"isInsolvent"`
	CreatedAt       time.Time `gorm:"index:idx_user_positions_created_at" json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// TableName returns the table name for UserPosition
func (UserPosition) TableName() string {
	return "user_positions"
}

// LiquidationEvent represents a liquidation event
type LiquidationEvent struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	TxHash          string    `gorm:"uniqueIndex:idx_liquidation_events_tx_hash;size:66;not null" json:"txHash"`
	BlockNumber     uint64    `gorm:"index:idx_liquidation_events_block_number;not null" json:"blockNumber"`
	Liquidator      string    `gorm:"index:idx_liquidation_events_liquidator;size:42;not null" json:"liquidator"`
	User            string    `gorm:"index:idx_liquidation_events_user;size:42;not null" json:"user"`
	To              string    `gorm:"size:42;not null" json:"to"`
	CollateralShare string    `gorm:"type:text" json:"collateralShare"`
	BorrowAmount    string    `gorm:"type:text" json:"borrowAmount"`
	BorrowPart      string    `gorm:"type:text" json:"borrowPart"`
	CreatedAt       time.Time `gorm:"index:idx_liquidation_events_created_at" json:"createdAt"`
}

// TableName returns the table name for LiquidationEvent
func (LiquidationEvent) TableName() string {
	return "liquidation_events"
}

// SystemHealth represents the overall system health
type SystemHealth struct {
	ID                    uint      `gorm:"primaryKey" json:"id"`
	TotalBorrowElastic    string    `gorm:"type:text;not null" json:"totalBorrowElastic"`
	TotalBorrowBase       string    `gorm:"type:text;not null" json:"totalBorrowBase"`
	TotalCollateralShare  string    `gorm:"type:text;not null" json:"totalCollateralShare"`
	ExchangeRate          string    `gorm:"type:text;not null" json:"exchangeRate"`
	LiquidationMultiplier string    `gorm:"type:text;not null" json:"liquidationMultiplier"`
	CollateralizationRate string    `gorm:"type:text;not null" json:"collateralizationRate"`
	CreatedAt             time.Time `gorm:"index:idx_system_healths_created_at" json:"createdAt"`
}

// TableName returns the table name for SystemHealth
func (SystemHealth) TableName() string {
	return "system_healths"
}

// BotStatus represents the bot's current status (this might be better as a config table)
type BotStatus struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	IsRunning     bool      `gorm:"default:false" json:"isRunning"`
	CheckInterval string    `gorm:"size:50;default:'30s'" json:"checkInterval"`
	LastCheck     time.Time `json:"lastCheck"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// TableName returns the table name for BotStatus
func (BotStatus) TableName() string {
	return "bot_status"
}

// BotLog represents bot operation logs
type BotLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Level     string    `gorm:"index:idx_bot_logs_level;size:10;not null" json:"level"`
	Message   string    `gorm:"type:text;not null" json:"message"`
	Data      string    `gorm:"type:text" json:"data,omitempty"`
	CreatedAt time.Time `gorm:"index:idx_bot_logs_created_at" json:"createdAt"`
}

// TableName returns the table name for BotLog
func (BotLog) TableName() string {
	return "bot_logs"
}

// BeforeCreate hook for BotLog to set CreatedAt
func (b *BotLog) BeforeCreate(tx *gorm.DB) error {
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now()
	}
	return nil
}

// BeforeCreate hook for LiquidationEvent to set CreatedAt
func (l *LiquidationEvent) BeforeCreate(tx *gorm.DB) error {
	if l.CreatedAt.IsZero() {
		l.CreatedAt = time.Now()
	}
	return nil
}

// BeforeCreate hook for SystemHealth to set CreatedAt
func (s *SystemHealth) BeforeCreate(tx *gorm.DB) error {
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now()
	}
	return nil
}

// AutoMigrate runs database migrations
func AutoMigrate(db *gorm.DB) error {
	// Create extensions first
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\"").Error; err != nil {
		return err
	}

	// Run auto migrations
	if err := db.AutoMigrate(
		&UserPosition{},
		&LiquidationEvent{},
		&SystemHealth{},
		&BotLog{},
		&BotStatus{},
	); err != nil {
		return err
	}

	// Create composite indexes that GORM can't handle via tags
	if err := createCompositeIndexes(db); err != nil {
		return err
	}

	return nil
}

// createCompositeIndexes creates composite indexes that require raw SQL
func createCompositeIndexes(db *gorm.DB) error {
	indexes := []string{
		// Composite index for user positions by user and created_at
		`CREATE INDEX IF NOT EXISTS idx_user_positions_user_created_at ON user_positions(user_address, created_at DESC)`,

		// Composite index for liquidation events by user and block number
		`CREATE INDEX IF NOT EXISTS idx_liquidation_events_user_block ON liquidation_events(user_address, block_number DESC)`,

		// Composite index for bot logs by level and created_at
		`CREATE INDEX IF NOT EXISTS idx_bot_logs_level_created_at ON bot_logs(level, created_at DESC)`,

		// Partial index for insolvent positions only
		`CREATE INDEX IF NOT EXISTS idx_user_positions_insolvent ON user_positions(user_address, created_at DESC) WHERE is_insolvent = true`,
	}

	for _, indexSQL := range indexes {
		if err := db.Exec(indexSQL).Error; err != nil {
			return err
		}
	}

	return nil
}

// Repository methods for common queries
type UserPositionRepository struct {
	db *gorm.DB
}

func NewUserPositionRepository(db *gorm.DB) *UserPositionRepository {
	return &UserPositionRepository{db: db}
}

// FindInsolventPositions finds all insolvent positions
func (r *UserPositionRepository) FindInsolventPositions() ([]UserPosition, error) {
	var positions []UserPosition
	err := r.db.Where("is_insolvent = ?", true).
		Order("created_at DESC").
		Find(&positions).Error
	return positions, err
}

// FindPositionsByUser finds all positions for a specific user
func (r *UserPositionRepository) FindPositionsByUser(userAddress string) ([]UserPosition, error) {
	var positions []UserPosition
	err := r.db.Where("user_address = ?", userAddress).
		Order("created_at DESC").
		Find(&positions).Error
	return positions, err
}

// UpsertPosition creates or updates a user position
func (r *UserPositionRepository) UpsertPosition(position *UserPosition) error {
	// Try to find existing position first
	var existing UserPosition
	err := r.db.Where("user_address = ?", position.User).First(&existing).Error

	if err == gorm.ErrRecordNotFound {
		// Create new position
		return r.db.Create(position).Error
	} else if err != nil {
		return err
	}

	// Update existing position
	position.ID = existing.ID
	position.CreatedAt = existing.CreatedAt // Preserve original created_at
	return r.db.Save(position).Error
}

// LiquidationEventRepository handles liquidation event operations
type LiquidationEventRepository struct {
	db *gorm.DB
}

func NewLiquidationEventRepository(db *gorm.DB) *LiquidationEventRepository {
	return &LiquidationEventRepository{db: db}
}

// CreateEvent creates a new liquidation event (with duplicate check)
func (r *LiquidationEventRepository) CreateEvent(event *LiquidationEvent) error {
	// Check if event already exists
	var existing LiquidationEvent
	err := r.db.Where("tx_hash = ?", event.TxHash).First(&existing).Error

	if err == nil {
		// Event already exists, return nil (no error)
		return nil
	} else if err != gorm.ErrRecordNotFound {
		return err
	}

	// Create new event
	return r.db.Create(event).Error
}

// FindEventsByUser finds liquidation events for a specific user
func (r *LiquidationEventRepository) FindEventsByUser(userAddress string) ([]LiquidationEvent, error) {
	var events []LiquidationEvent
	err := r.db.Where("user_address = ?", userAddress).
		Order("block_number DESC").
		Find(&events).Error
	return events, err
}

// BotLogRepository handles bot log operations
type BotLogRepository struct {
	db *gorm.DB
}

func NewBotLogRepository(db *gorm.DB) *BotLogRepository {
	return &BotLogRepository{db: db}
}

// CreateLog creates a new bot log entry
func (r *BotLogRepository) CreateLog(level, message, data string) error {
	log := &BotLog{
		Level:   level,
		Message: message,
		Data:    data,
	}
	return r.db.Create(log).Error
}

// FindLogsByLevel finds logs by level with pagination
func (r *BotLogRepository) FindLogsByLevel(level string, limit, offset int) ([]BotLog, error) {
	var logs []BotLog
	err := r.db.Where("level = ?", level).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&logs).Error
	return logs, err
}

// CleanupOldLogs removes logs older than specified duration
func (r *BotLogRepository) CleanupOldLogs(olderThan time.Duration) error {
	cutoffTime := time.Now().Add(-olderThan)
	return r.db.Where("created_at < ?", cutoffTime).Delete(&BotLog{}).Error
}
