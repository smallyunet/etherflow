package pg

import (
	"context"
	"errors"
	"time"

	"github.com/username/etherflow/pkg/core"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// BlockModel represents the block state in the database
// Duplicated from sqlite package to keep store implementations decoupled
type BlockModel struct {
	Number      uint64 `gorm:"primaryKey;autoIncrement:false"`
	Hash        string `gorm:"size:66;not null"`
	ParentHash  string `gorm:"size:66;not null"`
	Timestamp   uint64
	ProcessedAt time.Time
}

// Store implements spi.StateStore using PostgreSQL
type Store struct {
	db *gorm.DB
}

// NewStore creates a new PostgreSQL store
func NewStore(dsn string) (*Store, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn), // Warn level for production
	})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&BlockModel{}); err != nil {
		return nil, err
	}

	return &Store{db: db}, nil
}

// SaveBlock saves a processed block hash to the store
func (s *Store) SaveBlock(ctx context.Context, block *core.Block) error {
	model := BlockModel{
		Number:      block.Number,
		Hash:        string(block.Hash),
		ParentHash:  string(block.ParentHash),
		Timestamp:   block.Timestamp,
		ProcessedAt: time.Now(),
	}
	// Use Save to upsert (primary key is Number)
	return s.db.WithContext(ctx).Save(&model).Error
}

// GetLastBlock returns the last successfully indexed block
func (s *Store) GetLastBlock(ctx context.Context) (*core.Block, error) {
	var model BlockModel
	result := s.db.WithContext(ctx).Order("number desc").First(&model)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil // No blocks yet
		}
		return nil, result.Error
	}
	return s.toCoreBlock(&model), nil
}

// GetBlockByNumber returns a block hash for a given height from local storage
func (s *Store) GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error) {
	var model BlockModel
	result := s.db.WithContext(ctx).Where("number = ?", number).First(&model)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return s.toCoreBlock(&model), nil
}

// Rewind deletes all blocks with number > height
func (s *Store) Rewind(ctx context.Context, height uint64) error {
	return s.db.WithContext(ctx).Where("number > ?", height).Delete(&BlockModel{}).Error
}

func (s *Store) toCoreBlock(m *BlockModel) *core.Block {
	return &core.Block{
		Number:     m.Number,
		Hash:       core.Hash(m.Hash),
		ParentHash: core.Hash(m.ParentHash),
		Timestamp:  m.Timestamp,
		Logs:       nil,
	}
}
