package spi

import (
	"context"
	"github.com/username/etherflow/pkg/core"
)

// BlockSource defines how to fetch data from a blockchain node
type BlockSource interface {
	// LatestBlock returns the latest block number and hash from the node
	LatestBlock(ctx context.Context) (uint64, core.Hash, error)
	
	// GetBlockByNumber fetches a block by its number
	GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error)
	
	// GetBlockByHash fetches a block by its hash
	GetBlockByHash(ctx context.Context, hash core.Hash) (*core.Block, error)
}

// StateStore defines how to persist indexing progress and handle reorgs
type StateStore interface {
	// SaveBlock saves a processed block hash to the store
	SaveBlock(ctx context.Context, block *core.Block) error
	
	// GetLastBlock returns the last successfully indexed block
	GetLastBlock(ctx context.Context) (*core.Block, error)
	
	// GetBlockByNumber returns a block hash for a given height from local storage
	// Used to verify consistency with the node
	GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error)
	
	// Rewind deletes all blocks with number > height
	Rewind(ctx context.Context, height uint64) error
}
