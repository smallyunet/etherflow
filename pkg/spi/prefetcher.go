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
		headCh = make(chan *core.Head, 1) // Buffered to avoid blocking sender
		s, err := subSource.SubscribeNewHead(ctx, headCh)
		if err == nil {
			sub = s
			defer sub.Unsubscribe()
		}
	}

	pollInterval := 2 * time.Second       // Normal polling interval
	keepAliveInterval := 10 * time.Second // Keep-alive for subscription (in case we miss an event)

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		case height := <-p.resetCh:
			p.drainBuffer()
			p.currentHeight = height
			continue
		default:
			// Fallthrough to fetching logic
		}

		// FETCH ATTEMPT
		// We try to fetch the current block.
		err := p.fetchNext(ctx)

		if err == nil {
			// SUCCESS: Block found and buffered.
			// Immediate loop to try fetching the next block (Catch-up mode)
			continue
		}

		// FAILURE: Block not ready or other error.
		// We need to wait. The wait strategy depends on mode.

		if sub != nil {
			// SUBSCRIPTION MODE
			// We wait for a signal (Head Event) OR a long timeout (Keep-alive)
			select {
			case <-ctx.Done():
				return
			case <-p.stopCh:
				return
			case height := <-p.resetCh:
				p.drainBuffer()
				p.currentHeight = height
				continue // Restart loop
			case <-headCh:
				// New block event received!
				// Loop back to try fetching.
				continue
			case <-time.After(keepAliveInterval):
				// Keep-alive triggered.
				// Loop back to try fetching just in case.
				continue
			case <-sub.Err():
				// Subscription error. Fallback behavior?
				// For now, treating as a signal to retry fetching (maybe re-sub logic needed in future)
				// Returning to loop will retry fetch, if it fails again we come back here.
				// If sub is dead, we might loop tightly?
				// Ideally we should detect sub death and switch to polling.
				// For simplified Phase 2, let's just wait pollInterval if sub errs.
				select {
				case <-time.After(pollInterval):
				case <-ctx.Done():
					return
				}
				continue
			}
		} else {
			// POLLING MODE
			// We wait for pollInterval
			select {
			case <-ctx.Done():
				return
			case <-p.stopCh:
				return
			case height := <-p.resetCh:
				p.drainBuffer()
				p.currentHeight = height
				continue
			case <-time.After(pollInterval):
				continue
			}
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
