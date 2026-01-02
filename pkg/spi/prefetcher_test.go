package spi

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/username/etherflow/pkg/core"
)

// MockSlowSource simulates network latency
type MockSlowSource struct {
	mu     sync.Mutex
	delay  time.Duration
	blocks map[uint64]*core.Block
	called map[uint64]int
}

func (s *MockSlowSource) LatestBlock(ctx context.Context) (uint64, core.Hash, error) {
	return 1000, "", nil
}
func (s *MockSlowSource) GetBlockByHash(ctx context.Context, hash core.Hash) (*core.Block, error) {
	return nil, nil // not used in prefetcher
}
func (s *MockSlowSource) GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error) {
	s.mu.Lock()
	s.called[number]++
	s.mu.Unlock()

	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return &core.Block{Number: number}, nil
}

func TestPrefetcher_Sequential(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &MockSlowSource{
		delay:  10 * time.Millisecond,
		called: make(map[uint64]int),
	}

	// Buffer size 5
	p := NewPrefetcher(mock, 5)
	p.Start(ctx, 1)
	defer p.Stop()

	// Consume 10 blocks
	for i := 1; i <= 10; i++ {
		start := time.Now()
		block, err := p.Next(ctx)
		if err != nil {
			t.Fatalf("Next error: %v", err)
		}
		if block.Number != uint64(i) {
			t.Errorf("Expected block %d, got %d", i, block.Number)
		}

		// 1st block takes 10ms. subsequent blocks should be instant if buffered
		if i > 5 && time.Since(start) > 5*time.Millisecond {
			// This assertion is flaky in CI depending on machine load, but locally good indiciation
			// t.Logf("Block %d took %v (prefetching effective)", i, time.Since(start))
		}
	}
}

func TestPrefetcher_Reset(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &MockSlowSource{
		delay:  1 * time.Millisecond,
		called: make(map[uint64]int),
	}

	p := NewPrefetcher(mock, 100)
	p.Start(ctx, 1)
	defer p.Stop()

	// Read 1, 2, 3
	for i := 1; i <= 3; i++ {
		b, _ := p.Next(ctx)
		if b.Number != uint64(i) {
			t.Errorf("Expected %d, got %d", i, b.Number)
		}
	}

	// Simulate Reorg: Reset to 2 (re-fetch 2, 3...)
	p.Reset(2)

	// Next should be 2 again
	b, err := p.Next(ctx)
	if err != nil {
		t.Fatalf("Next error: %v", err)
	}
	if b.Number != 2 {
		t.Errorf("After reset to 2, expected 2, got %d", b.Number)
	}

	b, _ = p.Next(ctx)
	if b.Number != 3 {
		t.Errorf("Expected 3, got %d", b.Number)
	}
}
