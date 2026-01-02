package etherflow

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/username/etherflow/pkg/config"
	"github.com/username/etherflow/pkg/core"
)

// Mock components (duplicates of other tests, but keeping self-contained for this package test)
type MockSource struct{}

func (m *MockSource) LatestBlock(ctx context.Context) (uint64, core.Hash, error) { return 0, "", nil }
func (m *MockSource) GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error) {
	return nil, nil
}
func (m *MockSource) GetBlockByHash(ctx context.Context, hash core.Hash) (*core.Block, error) {
	return nil, nil
}

type MockStore struct{}

func (m *MockStore) SaveBlock(ctx context.Context, block *core.Block) error { return nil }
func (m *MockStore) GetLastBlock(ctx context.Context) (*core.Block, error)  { return nil, nil }
func (m *MockStore) GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error) {
	return nil, nil
}
func (m *MockStore) Rewind(ctx context.Context, height uint64) error { return nil }

func TestParallelProcessing(t *testing.T) {
	// Setup
	block := &core.Block{
		Number: 1,
		Logs: []core.Log{
			{Index: 0},
			{Index: 1},
			{Index: 2},
			{Index: 3},
			{Index: 4},
		},
	}

	// Case 1: Sequential (Default)
	t.Run("Sequential", func(t *testing.T) {
		cfg := &config.Config{ParallelProcessing: false}
		// Basic setup
		indexer := &Indexer{
			cfg:      cfg,
			handlers: make(map[string]core.Handler),
			// Minimal valid state to avoid nil panics if processBlockEvents needs them (it doesn't use others)
		}

		var processedOrder []uint
		var mu sync.Mutex

		indexer.On("Test", func(ctx *core.EventContext) error {
			mu.Lock()
			processedOrder = append(processedOrder, ctx.Log.Index)
			mu.Unlock()
			return nil
		})

		if err := indexer.processBlockEvents(context.Background(), block, false); err != nil {
			t.Fatalf("Processing failed: %v", err)
		}

		// Verify order
		if len(processedOrder) != 5 {
			t.Errorf("Expected 5 processed logs, got %d", len(processedOrder))
		}
		for i, idx := range processedOrder {
			if idx != uint(i) {
				t.Errorf("Sequential order violation at pos %d: expected %d, got %d", i, i, idx)
			}
		}
	})

	// Case 2: Parallel
	t.Run("Parallel", func(t *testing.T) {
		cfg := &config.Config{ParallelProcessing: true}
		indexer := &Indexer{
			cfg:      cfg,
			handlers: make(map[string]core.Handler),
		}

		var processedOrder []uint
		var mu sync.Mutex

		indexer.On("Test", func(ctx *core.EventContext) error {
			// Simulate work to encourage reordering
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			processedOrder = append(processedOrder, ctx.Log.Index)
			mu.Unlock()
			return nil
		})

		// We cannot guarantee reordering, but we can verify it works without error
		// and processes all logs.
		if err := indexer.processBlockEvents(context.Background(), block, false); err != nil {
			t.Fatalf("Processing failed: %v", err)
		}

		if len(processedOrder) != 5 {
			t.Errorf("Expected 5 processed logs, got %d", len(processedOrder))
		}
	})
}
