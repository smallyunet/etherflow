package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/username/etherflow"
	"github.com/username/etherflow/pkg/core"
)

// E2ESource implements a controllable source
type E2ESource struct {
	blocks []*core.Block
}

func (m *E2ESource) LatestBlock(ctx context.Context) (uint64, core.Hash, error) {
	if len(m.blocks) == 0 {
		return 0, "", nil
	}
	last := m.blocks[len(m.blocks)-1]
	return last.Number, last.Hash, nil
}

func (m *E2ESource) GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error) {
	for _, b := range m.blocks {
		if b.Number == number {
			return b, nil
		}
	}
	return nil, fmt.Errorf("block not found")
}

func (m *E2ESource) GetBlockByHash(ctx context.Context, hash core.Hash) (*core.Block, error) {
	for _, b := range m.blocks {
		if b.Hash == hash {
			return b, nil
		}
	}
	return nil, fmt.Errorf("block not found")
}

// E2EStore implements a simple in-memory store
type E2EStore struct {
	lastBlock *core.Block
}

func (m *E2EStore) SaveBlock(ctx context.Context, block *core.Block) error {
	m.lastBlock = block
	return nil
}

func (m *E2EStore) GetLastBlock(ctx context.Context) (*core.Block, error) {
	return m.lastBlock, nil
}

func (m *E2EStore) GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error) {
	return nil, nil
}

func (m *E2EStore) Rewind(ctx context.Context, height uint64) error {
	if m.lastBlock != nil && m.lastBlock.Number > height {
		// Simulate rewind by just setting lastBlock to nil or a dummy
		// In a real test we might want to look up the block at height
		// For this test, we assume the indexer handles the state reset logic via monitor
		// But the store needs to reflect the rollback.
		// Let's just set it to nil to force re-sync from that point if needed, 
		// or ideally we should keep history.
		// For simplicity:
		m.lastBlock = &core.Block{Number: height} 
	}
	return nil
}

func TestIndexer_ReorgFlow(t *testing.T) {
	// 1. Setup
	source := &E2ESource{
		blocks: []*core.Block{
			{Number: 1, Hash: "0x1", ParentHash: "0x0", Logs: []core.Log{{Index: 1}}},
			{Number: 2, Hash: "0x2", ParentHash: "0x1", Logs: []core.Log{{Index: 2}}},
		},
	}
	store := &E2EStore{}
	indexer := etherflow.New(source, store)
	indexer.SetStartBlock(1)
	indexer.SetPollingInterval(10 * time.Millisecond)

	// Track events
	eventCount := 0
	indexer.On("Transfer", func(ctx *core.EventContext) error {
		eventCount++
		return nil
	})

	reorgDetected := false
	indexer.OnReorg(func(ctx context.Context, forkPoint *core.Block, oldChain []*core.Block, newChain []*core.Block) error {
		reorgDetected = true
		return nil
	})

	// 2. Run in background
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := indexer.Run(ctx); err != nil {
			// t.Error(err) // Can't call t.Error from goroutine safely without helper
		}
	}()

	// 3. Wait for initial sync
	time.Sleep(100 * time.Millisecond)
	// Should have processed 2 blocks
	if eventCount != 2 {
		t.Errorf("Expected 2 events, got %d", eventCount)
	}

	// 4. Simulate Reorg
	// Replace block 2 with 2b, add 3b
	source.blocks = []*core.Block{
		{Number: 1, Hash: "0x1", ParentHash: "0x0"},
		{Number: 2, Hash: "0x2b", ParentHash: "0x1", Logs: []core.Log{{Index: 3}}}, // New branch
		{Number: 3, Hash: "0x3b", ParentHash: "0x2b", Logs: []core.Log{{Index: 4}}},
	}

	// Wait for poll
	time.Sleep(3 * time.Second) // Polling interval is 2s in default implementation

	if !reorgDetected {
		t.Error("Reorg was not detected")
	}
	
	// We expect events from 2b and 3b to be processed
	// Total events: 2 (initial) + 2 (new chain) = 4
	// Note: Depending on implementation, old events might be reverted or not. 
	// The current simple implementation doesn't "un-emit" events, it just calls ReorgHandler.
	// But it should process the new blocks.
	if eventCount < 4 {
		t.Errorf("Expected at least 4 events (2 initial + 2 new), got %d", eventCount)
	}

	cancel()
}
