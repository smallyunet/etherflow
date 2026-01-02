package monitor

import (
	"context"
	"fmt"
	"testing"

	"github.com/username/etherflow/pkg/core"
)

// MockSource for testing
type MockSource struct {
	blocks map[core.Hash]*core.Block
}

func (m *MockSource) LatestBlock(ctx context.Context) (uint64, core.Hash, error) {
	return 0, "", nil
}

func (m *MockSource) GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error) {
	return nil, nil
}

func (m *MockSource) GetBlockByHash(ctx context.Context, hash core.Hash) (*core.Block, error) {
	if b, ok := m.blocks[hash]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("block not found: %s", hash)
}

// MockStore for testing
type MockStore struct{}

func (m *MockStore) SaveBlock(ctx context.Context, block *core.Block) error { return nil }
func (m *MockStore) GetLastBlock(ctx context.Context) (*core.Block, error)  { return nil, nil }
func (m *MockStore) GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error) {
	return nil, nil
}
func (m *MockStore) Rewind(ctx context.Context, height uint64) error { return nil }

func TestChainMonitor_CheckConsistency(t *testing.T) {
	monitor := NewChainMonitor(&MockSource{}, &MockStore{}, 10)

	// 1. Empty monitor should be consistent
	b1 := &core.Block{Number: 1, Hash: "0x1", ParentHash: "0x0"}
	consistent, err := monitor.CheckConsistency(b1)
	if err != nil {
		t.Fatalf("Empty monitor should be consistent, got error: %v", err)
	}
	if !consistent {
		t.Fatal("Empty monitor should be consistent")
	}

	monitor.AddBlock(b1)

	// 2. Sequential block
	b2 := &core.Block{Number: 2, Hash: "0x2", ParentHash: "0x1"}
	consistent, err = monitor.CheckConsistency(b2)
	if err != nil || !consistent {
		t.Fatalf("Sequential block should be consistent")
	}
	monitor.AddBlock(b2)

	// 3. Gap detection
	b4 := &core.Block{Number: 4, Hash: "0x4", ParentHash: "0x3"}
	_, err = monitor.CheckConsistency(b4)
	if err == nil {
		t.Fatal("Gap should return error")
	}

	// 4. Fork detection (Reorg)
	// b2_fork has same number as b2 but different hash/parent
	// Actually, usually we detect reorg when we see b3' that points to b2' instead of b2
	// Or if we see b2' directly.

	// Case A: Same height, different hash (implied different parent or content)
	b2Prime := &core.Block{Number: 2, Hash: "0x2b", ParentHash: "0x1"}
	consistent, err = monitor.CheckConsistency(b2Prime)
	if err != nil {
		t.Fatalf("CheckConsistency failed: %v", err)
	}
	if consistent {
		t.Fatal("Same height block with different hash should be inconsistent (reorg start)")
	}

	// Case B: Next height, but parent is not our head
	b3Prime := &core.Block{Number: 3, Hash: "0x3b", ParentHash: "0x2b"} // Parent is 0x2b, but we have 0x2
	consistent, err = monitor.CheckConsistency(b3Prime)
	if err != nil {
		t.Fatalf("CheckConsistency failed: %v", err)
	}
	if consistent {
		t.Fatal("Block with unknown parent should be inconsistent")
	}
}

func TestChainMonitor_ResolveReorg(t *testing.T) {
	// Setup chain: 1 -> 2 -> 3
	b1 := &core.Block{Number: 1, Hash: "0x1", ParentHash: "0x0"}
	b2 := &core.Block{Number: 2, Hash: "0x2", ParentHash: "0x1"}
	b3 := &core.Block{Number: 3, Hash: "0x3", ParentHash: "0x2"}

	// Fork chain: 1 -> 2b -> 3b
	b2b := &core.Block{Number: 2, Hash: "0x2b", ParentHash: "0x1"}
	b3b := &core.Block{Number: 3, Hash: "0x3b", ParentHash: "0x2b"}

	source := &MockSource{
		blocks: map[core.Hash]*core.Block{
			"0x1":  b1,
			"0x2":  b2,
			"0x3":  b3,
			"0x2b": b2b,
			"0x3b": b3b,
		},
	}
	monitor := NewChainMonitor(source, &MockStore{}, 10)
	monitor.AddBlock(b1)
	monitor.AddBlock(b2)
	monitor.AddBlock(b3)

	// Now we receive b3b.
	// ResolveReorg should find that b3b -> b2b -> b1 (which is in monitor).
	// Fork point is b1.
	// Old chain: b2, b3
	// New chain: b2b, b3b

	forkPoint, oldChain, newChain, err := monitor.ResolveReorg(context.Background(), b3b)
	if err != nil {
		t.Fatalf("ResolveReorg failed: %v", err)
	}

	if forkPoint.Hash != b1.Hash {
		t.Errorf("Expected fork point 0x1, got %s", forkPoint.Hash)
	}

	if len(oldChain) != 2 {
		t.Errorf("Expected old chain len 2, got %d", len(oldChain))
	}
	if oldChain[0].Hash != "0x2" || oldChain[1].Hash != "0x3" {
		t.Errorf("Unexpected old chain structure")
	}

	if len(newChain) != 2 {
		t.Errorf("Expected new chain len 2, got %d", len(newChain))
	}
	if newChain[0].Hash != "0x2b" || newChain[1].Hash != "0x3b" {
		t.Errorf("Unexpected new chain structure")
	}
}
