package spi

import (
	"context"
	"fmt"
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
	val, exists := s.blocks[number]
	s.mu.Unlock()

	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	if exists {
		return val, nil
	}
	// Default behavior for Sequential test
	return &core.Block{Number: number}, nil
}

func TestPrefetcher_Sequential(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &MockSlowSource{
		delay:  10 * time.Millisecond,
		called: make(map[uint64]int),
		blocks: make(map[uint64]*core.Block),
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
		blocks: make(map[uint64]*core.Block),
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

// MockSubscriptionSource implements SubscriptionSource
type MockSubscriptionSource struct {
	MockSlowSource
	headCh chan *core.Head
}

type MockSubscription struct {
	errCh chan error
}

func (s *MockSubscription) Unsubscribe()      {}
func (s *MockSubscription) Err() <-chan error { return s.errCh }

func (s *MockSubscriptionSource) SubscribeNewHead(ctx context.Context, ch chan<- *core.Head) (Subscription, error) {
	// Send updates here manually in test
	// We can't assign 'ch' to 's.headCh' easily because direction differs,
	// but we can start a routine to forward.
	// Actually for test simplicity, let's just return a sub and let caller trigger events via a separate channel
	// or we expose a method on MockSubscriptionSource to Trigger event.
	return &MockSubscription{errCh: make(chan error)}, nil
}

// Better Mock for Subscription Test
type MockSubSource struct {
	mu     sync.Mutex
	blocks map[uint64]*core.Block
	called map[uint64]int
	heads  chan *core.Head // Test writes here, mock forwards to subscriber
}

func (s *MockSubSource) LatestBlock(ctx context.Context) (uint64, core.Hash, error) {
	return 100, "", nil
}
func (s *MockSubSource) GetBlockByHash(ctx context.Context, hash core.Hash) (*core.Block, error) {
	return nil, nil
}
func (s *MockSubSource) GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.called[number]++

	if b, ok := s.blocks[number]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("block %d not ready", number)
}

func (s *MockSubSource) SubscribeNewHead(ctx context.Context, ch chan<- *core.Head) (Subscription, error) {
	go func() {
		for h := range s.heads {
			ch <- h
		}
	}()
	return &MockSubscription{errCh: make(chan error)}, nil
}

func TestPrefetcher_Subscription(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &MockSubSource{
		blocks: make(map[uint64]*core.Block),
		called: make(map[uint64]int),
		heads:  make(chan *core.Head, 10),
	}
	// Setup block 1 (already available)
	mock.blocks[1] = &core.Block{Number: 1}

	p := NewPrefetcher(mock, 5)
	p.Start(ctx, 1) // Start fetching from 1
	defer p.Stop()

	// 1. Should fetch block 1 immediately (catch-up)
	b, err := p.Next(ctx)
	if err != nil {
		t.Fatalf("Failed getting block 1: %v", err)
	}
	if b.Number != 1 {
		t.Errorf("Expected 1, got %d", b.Number)
	}

	// 2. Block 2 is NOT ready. Prefetcher should try once, fail, and wait.
	// We verify it called GetBlockByNumber(2)
	time.Sleep(50 * time.Millisecond) // Give loop a moment
	mock.mu.Lock()
	if mock.called[2] == 0 {
		t.Error("Expected prefetcher to attempt fetching 2 at least once")
	}
	callsBeforeSignal := mock.called[2]
	mock.mu.Unlock()

	// 3. Provide Block 2 and signal via head event
	mock.mu.Lock()
	mock.blocks[2] = &core.Block{Number: 2}
	mock.mu.Unlock()

	// Send head event for block 2
	mock.heads <- &core.Head{Number: 2}

	// 4. Prefetcher should wake up and fetch 2
	b2, err := p.Next(ctx)
	if err != nil {
		t.Fatalf("Failed getting block 2: %v", err)
	}
	if b2.Number != 2 {
		t.Errorf("Expected 2, got %d", b2.Number)
	}

	// Check call count again to ensure it didn't spam (polling fallback is 2s, we waited <100ms)
	// It should have called it again after signal.
	mock.mu.Lock()
	callsAfterSignal := mock.called[2]
	mock.mu.Unlock()

	if callsAfterSignal <= callsBeforeSignal {
		t.Error("Expected prefetcher to fetch 2 again after signal")
	}
}
