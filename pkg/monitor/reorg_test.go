package monitor

import (
	"context"
	"errors"
	"testing"

	"github.com/username/etherflow/pkg/core"
)

// --- Mocks ---

// --- Mocks ---

type ReorgMockStore struct {
	Blocks map[uint64]*core.Block
}

func (s *ReorgMockStore) SaveBlock(ctx context.Context, block *core.Block) error {
	s.Blocks[block.Number] = block
	return nil
}

func (s *ReorgMockStore) GetLastBlock(ctx context.Context) (*core.Block, error) {
	if len(s.Blocks) == 0 {
		return nil, nil
	}
	var max uint64
	for k := range s.Blocks {
		if k > max {
			max = k
		}
	}
	return s.Blocks[max], nil
}

func (s *ReorgMockStore) GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error) {
	b, ok := s.Blocks[number]
	if !ok {
		return nil, nil
	}
	return b, nil
}

func (s *ReorgMockStore) Rewind(ctx context.Context, height uint64) error {
	for k := range s.Blocks {
		if k > height {
			delete(s.Blocks, k)
		}
	}
	return nil
}

type ReorgMockSource struct {
	BlocksByHash map[core.Hash]*core.Block
}

func (s *ReorgMockSource) LatestBlock(ctx context.Context) (uint64, core.Hash, error) {
	return 0, "", errors.New("not implemented")
}
func (s *ReorgMockSource) GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error) {
	return nil, errors.New("not implemented")
}
func (s *ReorgMockSource) GetBlockByHash(ctx context.Context, hash core.Hash) (*core.Block, error) {
	if b, ok := s.BlocksByHash[hash]; ok {
		return b, nil
	}
	return nil, errors.New("block not found in mock source")
}

// --- Tests ---

func TestResolveReorg_AcrossRestart(t *testing.T) {
	// Scenario:
	// Chain A: 99(HashA) -> 100(HashA)
	// Chain B: 99(HashA) -> 100(HashB) -> 101(HashB)
	// Store has Chain A (up to 100).
	// Monitor starts fresh (empty memory).
	// Incoming block: 101(HashB). parent is 100(HashB).
	// Consistency check fails (Store has 100A, incoming implies 100B).
	// ResolveReorg should find fork at 99.

	ctx := context.Background()

	// 1. Setup Data
	block99 := &core.Block{Number: 99, Hash: "0x99A", ParentHash: "0x98A"}
	block100A := &core.Block{Number: 100, Hash: "0x100A", ParentHash: "0x99A"}

	block100B := &core.Block{Number: 100, Hash: "0x100B", ParentHash: "0x99A"}
	block101B := &core.Block{Number: 101, Hash: "0x101B", ParentHash: "0x100B"}

	// 2. Setup Components
	store := &ReorgMockStore{
		Blocks: map[uint64]*core.Block{
			99:  block99,
			100: block100A,
		},
	}

	source := &ReorgMockSource{
		BlocksByHash: map[core.Hash]*core.Block{
			"0x100B": block100B,
			"0x99A":  block99, // Source also has the common ancestor
		},
	}

	monitor := NewChainMonitor(source, store, 10)
	// Monitor is empty (simulating restart)

	// 3. Run ResolveReorg
	// Incoming is 101B.
	forkPoint, oldChain, newChain, err := monitor.ResolveReorg(ctx, block101B)
	if err != nil {
		t.Fatalf("ResolveReorg failed: %v", err)
	}

	// 4. Verification

	// Fork Point should be 99
	if forkPoint.Number != 99 || forkPoint.Hash != "0x99A" {
		t.Errorf("Expected fork point 99(0x99A), got %d(%s)", forkPoint.Number, forkPoint.Hash)
	}

	// Old Chain should be [100A]
	// Because store had 100A, which is being invalidates
	if len(oldChain) != 1 {
		t.Errorf("Expected old chain len 1, got %d", len(oldChain))
	} else if oldChain[0].Hash != "0x100A" {
		t.Errorf("Expected old chain block to be 100A, got %s", oldChain[0].Hash)
	}

	// New Chain should be [100B, 101B]
	if len(newChain) != 2 {
		t.Errorf("Expected new chain len 2, got %d", len(newChain))
	} else {
		if newChain[0].Hash != "0x100B" {
			t.Errorf("Expected newChain[0] to be 100B, got %s", newChain[0].Hash)
		}
		if newChain[1].Hash != "0x101B" {
			t.Errorf("Expected newChain[1] to be 101B, got %s", newChain[1].Hash)
		}
	}
}
