package monitor

import (
	"context"
	"fmt"
	"sync"

	"github.com/username/etherflow/pkg/core"
	"github.com/username/etherflow/pkg/spi"
)

// SafeWindowSize defines how many blocks we keep in memory to handle reorgs
// Deprecated: use Config.SafeWindowSize
const DefaultSafeWindowSize = 128

// InMemoryChainMonitor implements core.ChainMonitor
type InMemoryChainMonitor struct {
	source spi.BlockSource
	store  spi.StateStore

	// blocks is a circular buffer or list of recent blocks
	// For simplicity, using a slice here, where index 0 is the oldest
	blocks []*core.Block
	mu     sync.RWMutex

	safeWindowSize uint64
}

func NewChainMonitor(source spi.BlockSource, store spi.StateStore, safeWindowSize uint64) *InMemoryChainMonitor {
	if safeWindowSize == 0 {
		safeWindowSize = DefaultSafeWindowSize
	}
	return &InMemoryChainMonitor{
		source:         source,
		store:          store,
		blocks:         make([]*core.Block, 0, safeWindowSize),
		safeWindowSize: safeWindowSize,
	}
}

func (m *InMemoryChainMonitor) AddBlock(block *core.Block) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if uint64(len(m.blocks)) >= m.safeWindowSize {
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
	var forkPoint *core.Block
	newChain := []*core.Block{newHead}
	current := newHead

	// Backtrack the new chain from the node
	// We keep backtracking until we find a parent that matches what we have in history (memory or DB)
	for {
		// A. Check in Memory
		parent, found := m.findInHistory(current.ParentHash)
		if found {
			forkPoint = parent
			break
		}

		// B. Check in Store (for persistence across restarts)
		// We expect the parent block to be at current.Number - 1
		parentNum := current.Number - 1
		storedParent, err := m.store.GetBlockByNumber(ctx, parentNum)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to fetch parent from store at %d: %w", parentNum, err)
		}

		if storedParent != nil && storedParent.Hash == current.ParentHash {
			// Found the fork point in DB!
			forkPoint = storedParent
			break
		}

		// If storedParent is nil OR hash mismatch, it means we are still on a side chain compared
		// to what is in our DB/Memory. We need to fetch the *real* parent of 'current' from the Source.

		// Fetch parent from source
		parentBlock, err := m.source.GetBlockByHash(ctx, current.ParentHash)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to fetch parent %s: %w", current.ParentHash, err)
		}

		// Prepend to newChain
		newChain = append([]*core.Block{parentBlock}, newChain...)
		current = parentBlock

		// Safety break
		if uint64(len(newChain)) > m.safeWindowSize {
			return nil, nil, nil, fmt.Errorf("reorg too deep, exceeded safety window")
		}
	}

	// 2. Identify old chain (blocks in history that are after forkPoint)
	oldChain := []*core.Block{}

	// Try to find forkPoint in memory first
	forkIndex := -1
	for i, b := range m.blocks {
		if b.Hash == forkPoint.Hash {
			forkIndex = i
			break
		}
	}

	if forkIndex != -1 {
		// Case A: Fork point is in memory
		if forkIndex+1 < len(m.blocks) {
			oldChain = m.blocks[forkIndex+1:]
			// Truncate history to fork point
			m.blocks = m.blocks[:forkIndex+1]
		}
	} else {
		// Case B: Fork point is deep in DB, not in current memory window.
		// This happens if we restarted and confirmed a reorg deep in history (but within safety limit fetched from Source).
		// We need to fetch the "old chain" from DB to report it.
		// Blocks from ForkPoint.Number + 1 to LastStoredBlock

		lastStored, err := m.store.GetLastBlock(ctx)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get last stored block: %w", err)
		}

		if lastStored != nil && lastStored.Number > forkPoint.Number {
			// Fetch all blocks that are being reverted
			for i := forkPoint.Number + 1; i <= lastStored.Number; i++ {
				b, err := m.store.GetBlockByNumber(ctx, i)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("failed to fetch reverted block %d: %w", i, err)
				}
				if b != nil {
					oldChain = append(oldChain, b)
				}
			}
		}

		// Since fork point is not in memory, we assume our memory is either empty or contains wrong blocks
		// (though CheckConsistency usually prevents adding wrong blocks, on restart it might be empty).
		// We should clear memory to be safe as we are resetting state.
		m.blocks = []*core.Block{forkPoint}
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
