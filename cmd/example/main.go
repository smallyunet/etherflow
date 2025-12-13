package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/username/etherflow"
	"github.com/username/etherflow/pkg/core"
	"github.com/username/etherflow/pkg/spi"
)

// MockSource implements spi.BlockSource for demonstration
type MockSource struct {
	blocks []*core.Block
}

func (m *MockSource) LatestBlock(ctx context.Context) (uint64, core.Hash, error) {
	if len(m.blocks) == 0 {
		return 0, "", nil
	}
	last := m.blocks[len(m.blocks)-1]
	return last.Number, last.Hash, nil
}

func (m *MockSource) GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error) {
	for _, b := range m.blocks {
		if b.Number == number {
			return b, nil
		}
	}
	return nil, fmt.Errorf("block not found")
}

func (m *MockSource) GetBlockByHash(ctx context.Context, hash core.Hash) (*core.Block, error) {
	for _, b := range m.blocks {
		if b.Hash == hash {
			return b, nil
		}
	}
	return nil, fmt.Errorf("block not found")
}

// MockStore implements spi.StateStore
type MockStore struct {
	lastBlock *core.Block
}

func (m *MockStore) SaveBlock(ctx context.Context, block *core.Block) error {
	m.lastBlock = block
	fmt.Printf("[Store] Saved block %d (%s)\n", block.Number, block.Hash)
	return nil
}

func (m *MockStore) GetLastBlock(ctx context.Context) (*core.Block, error) {
	return m.lastBlock, nil
}

func (m *MockStore) GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error) {
	return nil, nil
}

func (m *MockStore) Rewind(ctx context.Context, height uint64) error {
	fmt.Printf("[Store] Rewinding to block %d\n", height)
	if m.lastBlock != nil && m.lastBlock.Number > height {
		// In a real store, we would delete records.
		// Here we just reset the pointer if it's ahead.
		// This is a simplification.
	}
	return nil
}

func main() {
	// 1. Setup Mocks
	source := &MockSource{
		blocks: []*core.Block{
			{Number: 1, Hash: "0x100", ParentHash: "0x099", Logs: []core.Log{{Index: 1}}},
			{Number: 2, Hash: "0x101", ParentHash: "0x100", Logs: []core.Log{{Index: 2}}},
			{Number: 3, Hash: "0x102", ParentHash: "0x101", Logs: []core.Log{{Index: 3}}},
		},
	}
	store := &MockStore{}

	// 2. Initialize EtherFlow
	indexer := etherflow.New(source, store)

	// 3. Register Event Handlers
	indexer.On("Transfer", func(ctx *core.EventContext) error {
		fmt.Printf("[Event] Transfer detected in block %d, Log Index: %d\n", ctx.Block.Number, ctx.Log.Index)
		return nil
	})

	// 4. Register Reorg Handler
	indexer.OnReorg(func(ctx context.Context, forkPoint *core.Block, oldChain []*core.Block, newChain []*core.Block) error {
		fmt.Printf("[Reorg] Fork detected at block %d. Dropping %d blocks, adding %d blocks.\n", 
			forkPoint.Number, len(oldChain), len(newChain))
		return nil
	})

	// 5. Run
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := indexer.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
