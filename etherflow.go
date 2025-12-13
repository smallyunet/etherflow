package etherflow

import (
	"context"
	"fmt"
	"time"

	"github.com/username/etherflow/pkg/core"
	"github.com/username/etherflow/pkg/monitor"
	"github.com/username/etherflow/pkg/spi"
)

// Indexer is the main entry point for the framework
type Indexer struct {
	source  spi.BlockSource
	store   spi.StateStore
	monitor core.ChainMonitor
	
	// handlers maps event signatures (e.g., "Transfer") to their handlers
	// For simplicity, using string keys. In reality, this might be topic hashes.
	handlers map[string]core.Handler
	
	// reorgHandler is called when a reorg occurs
	reorgHandler core.ReorgHandler
	
	pollingInterval time.Duration
}

// New creates a new Indexer instance
func New(source spi.BlockSource, store spi.StateStore) *Indexer {
	return &Indexer{
		source:          source,
		store:           store,
		monitor:         monitor.NewChainMonitor(source, store),
		handlers:        make(map[string]core.Handler),
		pollingInterval: 2 * time.Second,
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
		// If not found, maybe start from latest or 0?
		// For now, assume 0 or handle error
		fmt.Println("No previous state found, starting from scratch or config.")
	}
	
	if lastBlock != nil {
		i.monitor.AddBlock(lastBlock)
	}

	ticker := time.NewTicker(i.pollingInterval)
	defer ticker.Stop()

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
	// 1. Get latest block from node
	latestHeight, _, err := i.source.LatestBlock(ctx)
	if err != nil {
		return err
	}

	// 2. Determine next block to fetch
	// This logic needs to be robust. For now, let's assume we track the last processed block in memory via monitor or store.
	// Simplified: get last processed from store + 1
	lastProcessed, _ := i.store.GetLastBlock(ctx)
	nextHeight := uint64(0)
	if lastProcessed != nil {
		nextHeight = lastProcessed.Number + 1
	}

	if nextHeight > latestHeight {
		return nil // Up to date
	}

	// 3. Fetch the next block
	nextBlock, err := i.source.GetBlockByNumber(ctx, nextHeight)
	if err != nil {
		return err
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
			return err
		}
		
		// Re-process new chain is handled by the loop naturally as we reset state?
		// Actually, ResolveReorg updates the monitor state. We should probably process the newChain blocks now.
		for _, b := range newChain {
			if err := i.processBlockEvents(ctx, b, true); err != nil {
				return err
			}
			i.monitor.AddBlock(b)
			i.store.SaveBlock(ctx, b)
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
