package etherflow

import (
	"context"
	"fmt"
	"time"

	"github.com/username/etherflow/pkg/config"
	"github.com/username/etherflow/pkg/core"
	"github.com/username/etherflow/pkg/monitor"
	"github.com/username/etherflow/pkg/spi"
	"github.com/username/etherflow/pkg/util"
	"golang.org/x/sync/errgroup"
)

// Indexer is the main entry point for the framework
type Indexer struct {
	cfg     *config.Config
	source  spi.BlockSource
	store   spi.StateStore
	monitor core.ChainMonitor

	// handlers maps event signatures (e.g., "Transfer") to their handlers
	// For simplicity, using string keys. In reality, this might be topic hashes.
	handlers map[string]core.Handler

	// reorgHandler is called when a reorg occurs
	reorgHandler core.ReorgHandler

	prefetcher *spi.Prefetcher
}

// New creates a new Indexer instance
func New(cfg *config.Config, source spi.BlockSource, store spi.StateStore) *Indexer {
	backoff := util.NewBackoff(cfg.MaxRetries, cfg.RetryDelay)
	// Wrap source with retry logic
	retryingSource := spi.NewRetryingBlockSource(source, backoff)

	// Create prefetcher (buffer size 10 is hardcoded for now, could be config)
	prefetcher := spi.NewPrefetcher(retryingSource, 10)

	return &Indexer{
		cfg:        cfg,
		source:     retryingSource,
		store:      store,
		monitor:    monitor.NewChainMonitor(retryingSource, store, cfg.SafeWindowSize),
		handlers:   make(map[string]core.Handler),
		prefetcher: prefetcher,
	}
}

// On registers a handler for a specific event
// signature could be "Transfer(address,address,uint256)" or just "Transfer" for this simplified version
func (i *Indexer) On(eventName string, handler core.Handler) {
	i.handlers[eventName] = handler
}

// OnReorg registers a handler for reorg events
func (i *Indexer) OnReorg(handler core.ReorgHandler) {
	i.reorgHandler = handler
}

// Run starts the indexing process
func (i *Indexer) Run(ctx context.Context) error {
	fmt.Println("Starting EtherFlow Indexer...")

	// 1. Load initial state
	lastBlock, err := i.store.GetLastBlock(ctx)
	if err != nil {
		fmt.Printf("Error loading initial state: %v\n", err)
	}

	if lastBlock != nil {
		fmt.Printf("Resuming from block %d\n", lastBlock.Number)
		i.monitor.AddBlock(lastBlock)
	} else {
		fmt.Printf("No state found. Starting from configured block: %d\n", i.cfg.StartBlock)
	}

	ticker := time.NewTicker(i.cfg.PollingInterval)
	defer ticker.Stop()

	// Initial check immediately
	// Start prefetcher
	nextHeight := i.cfg.StartBlock
	if lastBlock != nil {
		nextHeight = lastBlock.Number + 1
	}
	i.prefetcher.Start(ctx, nextHeight)

	if err := i.processNextBlock(ctx); err != nil {
		fmt.Printf("Error processing block: %v\n", err)
	}

	for {
		select {
		case <-ctx.Done():
			i.prefetcher.Stop()
			return nil
		case <-ticker.C:
			// Note: With prefetcher, processNextBlock is blocking if buffer empty,
			// so ticker is less of a "driver" and more of a "wake up if stuck"?
			// Actually, if we want full async, we should just loop processNextBlock eagerly?
			// But for now keeping ticker structure. If Next() blocks, ticker ticks will pile up or be skipped?
			// A simple ticker loop calling a blocking function is okay.
			if err := i.processNextBlock(ctx); err != nil {
				fmt.Printf("Error processing block: %v\n", err)
			}
		}
	}
}

func (i *Indexer) processNextBlock(ctx context.Context) error {
	// 1. Get latest block from node with retry (handled by source wrapper)
	latestHeight, _, err := i.source.LatestBlock(ctx)
	if err != nil {
		return fmt.Errorf("failed to get latest block: %w", err)
	}

	// 2. Determine next block to fetch
	lastProcessed, err := i.store.GetLastBlock(ctx)
	if err != nil {
		return fmt.Errorf("failed to get last processed block: %w", err)
	}

	nextHeight := i.cfg.StartBlock
	if lastProcessed != nil {
		nextHeight = lastProcessed.Number + 1
	}

	if nextHeight > latestHeight {
		return nil // Up to date
	}

	// Wait, if nextHeight > latestHeight, we shouldn't even call prefetcher.Next() usually.
	// But prefetcher is running in background. It might already have fetched blocks if it's ahead.
	// Or it sits waiting.

	// 3. Fetch the next block from Prefetcher
	nextBlock, err := i.prefetcher.Next(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch block from prefetcher: %w", err)
	}

	// Sanity check: Ensure we got the block (height) we expected
	if nextBlock.Number != nextHeight {
		// This can happen if prefetcher was ahead, we restarted, and it gave us old logical next?
		// Or if we reset prefetcher incorrectly.
		// For now, let's trust prefetcher logic or reset it if mismatch.
		fmt.Printf("Warning: Expected block %d, got %d. Resetting prefetcher.\n", nextHeight, nextBlock.Number)
		i.prefetcher.Reset(nextHeight)
		// Try again next tick
		return nil
	}

	// 4. Check for Reorg
	consistent, err := i.monitor.CheckConsistency(nextBlock)
	if err != nil {
		return err
	}

	if !consistent {
		fmt.Printf("Reorg detected at block %d!\n", nextBlock.Number)
		forkPoint, oldChain, newChain, err := i.monitor.ResolveReorg(ctx, nextBlock)
		if err != nil {
			return fmt.Errorf("failed to resolve reorg: %w", err)
		}

		// Handle Reorg
		if i.reorgHandler != nil {
			if err := i.reorgHandler(ctx, forkPoint, oldChain, newChain); err != nil {
				return err
			}
		}

		// Rewind store
		if err := i.store.Rewind(ctx, forkPoint.Number); err != nil {
			return fmt.Errorf("failed to rewind store: %w", err)
		}

		// Process new chain
		for _, b := range newChain {
			if err := i.processBlockEvents(ctx, b, true); err != nil {
				return err
			}
			i.monitor.AddBlock(b)
			if err := i.store.SaveBlock(ctx, b); err != nil {
				return err
			}
		}

		// CRITICAL: Reset prefetcher to follow the new chain tip
		// The forkPoint is the last common ancestor.
		// We just processed `newChain` which ends at `nextBlock`.
		// So the next block we need is nextBlock.Number + 1.
		i.prefetcher.Reset(nextBlock.Number + 1)

		return nil
	}

	// 5. Normal Processing
	if err := i.processBlockEvents(ctx, nextBlock, false); err != nil {
		return err
	}

	// 6. Update State
	i.monitor.AddBlock(nextBlock)
	return i.store.SaveBlock(ctx, nextBlock)
}

func (i *Indexer) processBlockEvents(ctx context.Context, block *core.Block, isReorg bool) error {
	if i.cfg.ParallelProcessing {
		// Parallel processing using errgroup
		g, ctx := errgroup.WithContext(ctx)

		for _, log := range block.Logs {
			// Copy log for closure
			log := log
			g.Go(func() error {
				for _, handler := range i.handlers {
					eventCtx := &core.EventContext{
						Context: ctx,
						Block:   block,
						Log:     &log,
						IsReorg: isReorg,
					}
					if err := handler(eventCtx); err != nil {
						return err
					}
				}
				return nil
			})
		}
		return g.Wait()
	}

	// Sequential processing (default)
	for _, log := range block.Logs {
		// Match log to handlers
		// Simplified matching logic
		// In reality, check log.Topics[0] vs event signature hash

		// For this example, we just iterate all handlers (inefficient but simple)
		for _, handler := range i.handlers {
			// Create context
			eventCtx := &core.EventContext{
				Context: ctx,
				Block:   block,
				Log:     &log,
				IsReorg: isReorg,
			}

			if err := handler(eventCtx); err != nil {
				return err
			}
		}
	}
	return nil
}
