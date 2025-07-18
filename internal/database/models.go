package database

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

type ProcessingCheckpoint struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"uniqueIndex:idx_processing_checkpoints_name;size:50;not null" json:"name"`
	BlockNumber uint64    `gorm:"not null" json:"blockNumber"`
	TxHash      string    `gorm:"size:66" json:"txHash,omitempty"`
	CreatedAt   time.Time `gorm:"index:idx_processing_checkpoints_created_at" json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// TableName returns the table name for ProcessingCheckpoint
func (ProcessingCheckpoint) TableName() string {
	return "processing_checkpoints"
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

// BeforeCreate hook for ProcessingCheckpoint to set CreatedAt
func (p *ProcessingCheckpoint) BeforeCreate(tx *gorm.DB) error {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	return nil
}
