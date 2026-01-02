package spi

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/username/etherflow/pkg/core"
)

// Prefetcher fetches blocks in the background to improve throughput
type Prefetcher struct {
	source BlockSource
	buffer chan *core.Block
	errCh  chan error

	// control channels
	stopCh  chan struct{}
	resetCh chan uint64

	// state
	currentHeight uint64
	active        bool
	mu            sync.Mutex
	wg            sync.WaitGroup
}

// NewPrefetcher creates a new Prefetcher
func NewPrefetcher(source BlockSource, bufferSize int) *Prefetcher {
	return &Prefetcher{
		source:  source,
		buffer:  make(chan *core.Block, bufferSize),
		errCh:   make(chan error, 1),
		stopCh:  make(chan struct{}),
		resetCh: make(chan uint64),
	}
}

// Start begins the generic prefetching loop
func (p *Prefetcher) Start(ctx context.Context, startHeight uint64) {
	p.mu.Lock()
	if p.active {
		p.mu.Unlock()
		return
	}
	p.active = true
	p.currentHeight = startHeight
	p.mu.Unlock()

	p.wg.Add(1)
	go p.loop(ctx)
}

func (p *Prefetcher) loop(ctx context.Context) {
	defer p.wg.Done()

	// Check if source supports subscription
	var sub Subscription
	var headCh chan *core.Head

	// Unwrap RetryingBlockSource to check for SubscriptionSource
	var subSource SubscriptionSource

	// Try direct cast
	if s, ok := p.source.(SubscriptionSource); ok {
		subSource = s
	} else if rs, ok := p.source.(*RetryingBlockSource); ok {
		// Unwrap
		if s, ok := rs.Inner().(SubscriptionSource); ok {
			subSource = s
		}
	}

	// If found, subscribe
	if subSource != nil {
		headCh = make(chan *core.Head, 1)
		s, err := subSource.SubscribeNewHead(ctx, headCh)
		if err == nil {
			sub = s
			defer sub.Unsubscribe()
			// fmt.Println("WebSocket subscription enabled for prefetcher")
		} else {
			// fmt.Printf("Subscription failed, falling back to polling: %v\n", err)
		}
	}

	for {
		// Determine if we should attempt a fetch
		shouldFetch := false

		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		case height := <-p.resetCh:
			p.drainBuffer()
			p.currentHeight = height
			continue
		case head := <-headCh:
			// New block mined!
			if head.Number >= p.currentHeight {
				shouldFetch = true
			}
		default:
			// Polling (always try to fetch if we have capacity,
			// but we rely on blocking send or rate limiting if we don't have new data)
			// Simplification: We blindly try to fetch.
			// If we are caught up, the node returns "not found" or "future block" error?
			// Actually GetBlockByNumber usually returns nil/error if it doesn't exist yet.
			shouldFetch = true
		}

		if shouldFetch {
			err := p.fetchNext(ctx)
			if err != nil {
				// If error (e.g. block not ready), we wait a bit
				select {
				case <-ctx.Done():
					return
				case <-p.stopCh:
					return
				case height := <-p.resetCh:
					p.drainBuffer()
					p.currentHeight = height
					continue
				case <-time.After(500 * time.Millisecond): // Poll/Retry interval
					continue
				}
			}
		} else {
			// Passive wait (only if we trust subscription 100%, which we don't fully yet,
			// but here we fell through default so we actually always poll.
			// To truly save resources we should rely on headCh more.
			// But for Phase 2 MVP, just polling + eager fetch on head event is fine.
			// The eager fetch is implicitly handled because the loop runs.
		}
	}
}

func (p *Prefetcher) fetchNext(ctx context.Context) error {
	block, err := p.source.GetBlockByNumber(ctx, p.currentHeight)
	if err != nil {
		return err
	}

	select {
	case p.buffer <- block:
		p.currentHeight++
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-p.stopCh:
		return fmt.Errorf("stopped")
	case height := <-p.resetCh:
		p.drainBuffer()
		p.currentHeight = height
		return nil
	}
}

func (p *Prefetcher) drainBuffer() {
	// Non-blocking drain
L:
	for {
		select {
		case <-p.buffer:
		default:
			break L
		}
	}
	// Also drain error channel
E:
	for {
		select {
		case <-p.errCh:
		default:
			break E
		}
	}
}

// Next returns the next block or error
func (p *Prefetcher) Next(ctx context.Context) (*core.Block, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-p.errCh:
		return nil, err
	case block := <-p.buffer:
		return block, nil
	}
}

// Reset clears the buffer and restarts fetching from the given height
func (p *Prefetcher) Reset(height uint64) {
	// Blocking send to resetCh ensures loop sees it
	p.resetCh <- height
}

// Stop stops the prefetcher
func (p *Prefetcher) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.active {
		return
	}
	close(p.stopCh)
	p.wg.Wait()
	p.active = false
}
