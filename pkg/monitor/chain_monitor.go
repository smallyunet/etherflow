package monitor

import (
	"context"
	"fmt"
	"sync"

	"github.com/username/etherflow/pkg/core"
	"github.com/username/etherflow/pkg/spi"
)

// SafeWindowSize defines how many blocks we keep in memory to handle reorgs
const SafeWindowSize = 128

// InMemoryChainMonitor implements core.ChainMonitor
type InMemoryChainMonitor struct {
	source spi.BlockSource
	store  spi.StateStore
	
	// blocks is a circular buffer or list of recent blocks
	// For simplicity, using a slice here, where index 0 is the oldest
	blocks []*core.Block
	mu     sync.RWMutex
}

func NewChainMonitor(source spi.BlockSource, store spi.StateStore) *InMemoryChainMonitor {
	return &InMemoryChainMonitor{
		source: source,
		store:  store,
		blocks: make([]*core.Block, 0, SafeWindowSize),
	}
}

func (m *InMemoryChainMonitor) AddBlock(block *core.Block) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.blocks) >= SafeWindowSize {
		// Remove oldest
		m.blocks = m.blocks[1:]
	}
	m.blocks = append(m.blocks, block)
}

func (m *InMemoryChainMonitor) CheckConsistency(newBlock *core.Block) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.blocks) == 0 {
		return true, nil // No history, assume consistent (bootstrapping)
	}

	lastBlock := m.blocks[len(m.blocks)-1]
	
	// Simple check: Does the new block's parent match our last seen block?
	if newBlock.Number == lastBlock.Number+1 {
		return newBlock.ParentHash == lastBlock.Hash, nil
	}
	
	// If we missed some blocks, we can't easily check consistency without fetching intermediates.
	// For this simplified implementation, we assume sequential processing.
	if newBlock.Number <= lastBlock.Number {
		// We are seeing a block number we've already processed or older.
		// This might be a re-emit or a deep reorg that we need to handle.
		return false, nil
	}

	return false, fmt.Errorf("gap detected: last %d, new %d", lastBlock.Number, newBlock.Number)
}

func (m *InMemoryChainMonitor) ResolveReorg(ctx context.Context, newHead *core.Block) (*core.Block, []*core.Block, []*core.Block, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Find the fork point by backtracking from newHead
	// This is a simplified logic. In production, you'd fetch parents of newHead until you find a match in m.blocks.
	
	var forkPoint *core.Block
	newChain := []*core.Block{newHead}
	current := newHead

	// Backtrack the new chain from the node
	for {
		// Check if current's parent is in our local history
		parent, found := m.findInHistory(current.ParentHash)
		if found {
			forkPoint = parent
			break
		}

		// Fetch parent from source
		var err error
		parentBlock, err := m.source.GetBlockByHash(ctx, current.ParentHash)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to fetch parent %s: %w", current.ParentHash, err)
		}
		
		// Prepend to newChain
		newChain = append([]*core.Block{parentBlock}, newChain...)
		current = parentBlock
		
		// Safety break
		if len(newChain) > SafeWindowSize {
			return nil, nil, nil, fmt.Errorf("reorg too deep, exceeded safety window")
		}
	}

	// 2. Identify old chain (blocks in history that are after forkPoint)
	oldChain := []*core.Block{}
	forkIndex := -1
	for i, b := range m.blocks {
		if b.Hash == forkPoint.Hash {
			forkIndex = i
			break
		}
	}

	if forkIndex != -1 && forkIndex+1 < len(m.blocks) {
		oldChain = m.blocks[forkIndex+1:]
		// Truncate history to fork point
		m.blocks = m.blocks[:forkIndex+1]
	}

	return forkPoint, oldChain, newChain, nil
}

func (m *InMemoryChainMonitor) findInHistory(hash core.Hash) (*core.Block, bool) {
	for i := len(m.blocks) - 1; i >= 0; i-- {
		if m.blocks[i].Hash == hash {
			return m.blocks[i], true
		}
	}
	return nil, false
}
