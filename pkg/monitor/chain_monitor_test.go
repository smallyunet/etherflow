package monitor

import (
	"context"
	"testing"

	"github.com/username/etherflow/pkg/core"
)

type mockStore struct {
	blocks map[uint64]*core.Block
}

func (m *mockStore) GetLastBlock(ctx context.Context) (*core.Block, error) {
	return nil, nil
}
func (m *mockStore) SaveBlock(ctx context.Context, b *core.Block) error {
	m.blocks[b.Number] = b
	return nil
}
func (m *mockStore) GetBlockByNumber(ctx context.Context, n uint64) (*core.Block, error) {
	return m.blocks[n], nil
}
func (m *mockStore) Rewind(ctx context.Context, to uint64) error {
	return nil
}

type mockSource struct{}

func (m *mockSource) LatestBlock(ctx context.Context) (uint64, core.Hash, error) { return 0, "", nil }
func (m *mockSource) GetBlockByNumber(ctx context.Context, n uint64) (*core.Block, error) {
	return nil, nil
}
func (m *mockSource) GetBlockByHash(ctx context.Context, h core.Hash) (*core.Block, error) {
	return nil, nil
}
func (m *mockSource) SubscribeNewHeads(ctx context.Context) (<-chan *core.Head, error) {
	return nil, nil
}
func (m *mockSource) Close() {}

func TestChainMonitor_CheckConsistency(t *testing.T) {
	store := &mockStore{blocks: make(map[uint64]*core.Block)}
	mon := NewChainMonitor(&mockSource{}, store, 10)

	// Add genesis like block
	b1 := &core.Block{Number: 1, Hash: "hash1", ParentHash: "hash0"}
	mon.AddBlock(b1)

	// Test consistent block
	b2 := &core.Block{Number: 2, Hash: "hash2", ParentHash: "hash1"}
	consistent, err := mon.CheckConsistency(b2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !consistent {
		t.Error("expected consistent block")
	}

	// Test reorg (inconsistent parent)
	b2Fork := &core.Block{Number: 2, Hash: "hash2_fork", ParentHash: "hash1_fork"}
	consistent, err = mon.CheckConsistency(b2Fork)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if consistent {
		t.Error("expected inconsistent block")
	}
}
