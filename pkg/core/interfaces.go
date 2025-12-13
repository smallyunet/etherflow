package core

import "context"

// BlockIter iterates over blocks from a source
type BlockIter interface {
	// Next returns the next block. It blocks until a new block is available or context is cancelled.
	Next(ctx context.Context) (*Block, error)
	// Close cleans up resources
	Close() error
}

// ChainMonitor is responsible for maintaining chain consistency and detecting reorgs
type ChainMonitor interface {
	// CheckConsistency verifies if the new block connects to our local history
	// Returns true if consistent (ParentHash == LocalHead.Hash)
	CheckConsistency(newBlock *Block) (bool, error)
	
	// ResolveReorg finds the common ancestor and returns the path to switch to the new chain
	ResolveReorg(ctx context.Context, newHead *Block) (forkPoint *Block, oldChain []*Block, newChain []*Block, err error)
	
	// AddBlock adds a verified block to the monitor's internal safety window
	AddBlock(block *Block)
}
