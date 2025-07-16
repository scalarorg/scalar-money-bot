package database

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository methods for common queries
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, obj interface{}) *gorm.DB {
	return r.db.WithContext(ctx).Create(obj)
}

func (r *Repository) FirstOrCreate(ctx context.Context, dest interface{}, conds ...interface{}) *gorm.DB {
	return r.db.WithContext(ctx).FirstOrCreate(dest, conds)
}

func (r *Repository) Raw() *gorm.DB {
	return r.db
}

// FindInsolventPositions finds all insolvent positions
func (r *Repository) FindInsolventPositions() ([]UserPosition, error) {
	var positions []UserPosition
	err := r.db.Where("is_insolvent = ?", true).
		Order("created_at DESC").
		Find(&positions).Error
	return positions, err
}

// FindPositionsByUser finds all positions for a specific user
func (r *Repository) FindPositionsByUser(userAddress string) ([]UserPosition, error) {
	var positions []UserPosition
	err := r.db.Where("user_address = ?", userAddress).
		Order("created_at DESC").
		Find(&positions).Error
	return positions, err
}

// UpsertPosition creates or updates a user position
func (r *Repository) UpsertPosition(position *UserPosition) error {
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

// CreateEvent creates a new liquidation event (with duplicate check)
func (r *Repository) CreateEvent(event *LiquidationEvent) error {
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
func (r *Repository) FindEventsByUser(userAddress string) ([]LiquidationEvent, error) {
	var events []LiquidationEvent
	err := r.db.Where("user_address = ?", userAddress).
		Order("block_number DESC").
		Find(&events).Error
	return events, err
}

// CreateLog creates a new bot log entry
func (r *Repository) CreateLog(level, message, data string) error {
	log := &BotLog{
		Level:   level,
		Message: message,
		Data:    data,
	}
	return r.db.Create(log).Error
}

// FindLogsByLevel finds logs by level with pagination
func (r *Repository) FindLogsByLevel(level string, limit, offset int) ([]BotLog, error) {
	var logs []BotLog
	err := r.db.Where("level = ?", level).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&logs).Error
	return logs, err
}

// CleanupOldLogs removes logs older than specified duration
func (r *Repository) CleanupOldLogs(olderThan time.Duration) error {
	cutoffTime := time.Now().Add(-olderThan)
	return r.db.Where("created_at < ?", cutoffTime).Delete(&BotLog{}).Error
}

// GetCheckpoint retrieves the last processed block for a specific operation
func (r *Repository) GetCheckpoint(name string) (*ProcessingCheckpoint, error) {
	var checkpoint ProcessingCheckpoint
	err := r.db.Where("name = ?", name).First(&checkpoint).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil // No checkpoint found
	}
	return &checkpoint, err
}

// UpdateCheckpoint updates the checkpoint for a specific operation
func (r *Repository) UpdateCheckpoint(name string, blockNumber uint64, txHash string) error {
	checkpoint := &ProcessingCheckpoint{
		Name:        name,
		BlockNumber: blockNumber,
		TxHash:      txHash,
	}

	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"block_number", "tx_hash", "updated_at"}),
	}).Create(checkpoint).Error
}

// SetCheckpointIfNotExists creates a checkpoint only if it doesn't exist
func (r *Repository) SetCheckpointIfNotExists(name string, blockNumber uint64) error {
	var count int64
	err := r.db.Model(&ProcessingCheckpoint{}).Where("name = ?", name).Count(&count).Error
	if err != nil {
		return err
	}

	if count == 0 {
		checkpoint := &ProcessingCheckpoint{
			Name:        name,
			BlockNumber: blockNumber,
		}
		return r.db.Create(checkpoint).Error
	}

	return nil // Checkpoint already exists
}

// DeleteCheckpoint removes a checkpoint
func (r *Repository) DeleteCheckpoint(name string) error {
	return r.db.Where("name = ?", name).Delete(&ProcessingCheckpoint{}).Error
}

// GetAllCheckpoints retrieves all checkpoints
func (r *Repository) GetAllCheckpoints() ([]ProcessingCheckpoint, error) {
	var checkpoints []ProcessingCheckpoint
	err := r.db.Order("name").Find(&checkpoints).Error
	return checkpoints, err
}
