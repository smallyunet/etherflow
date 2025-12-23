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
}

// New creates a new Indexer instance
func New(cfg *config.Config, source spi.BlockSource, store spi.StateStore) *Indexer {
	backoff := util.NewBackoff(cfg.MaxRetries, cfg.RetryDelay)
	// Wrap source with retry logic
	retryingSource := spi.NewRetryingBlockSource(source, backoff)

	return &Indexer{
		cfg:      cfg,
		source:   retryingSource,
		store:    store,
		monitor:  monitor.NewChainMonitor(retryingSource, store),
		handlers: make(map[string]core.Handler),
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
	if err := i.processNextBlock(ctx); err != nil {
		fmt.Printf("Error processing block: %v\n", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
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

	// 3. Fetch the next block with retry (handled by source wrapper)
	nextBlock, err := i.source.GetBlockByNumber(ctx, nextHeight)
	if err != nil {
		return fmt.Errorf("failed to fetch block %d: %w", nextHeight, err)
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
