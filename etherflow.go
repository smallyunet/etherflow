package etherflow

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/username/etherflow/pkg/config"
	"github.com/username/etherflow/pkg/core"
	"github.com/username/etherflow/pkg/metrics"
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

	// handlers maps event signatures (e.g., Topic Hash) to their handlers
	handlers map[string]core.Handler

	// middlewares to apply to all handlers
	middlewares []core.Middleware

	// reorgHandler is called when a reorg occurs
	reorgHandler core.ReorgHandler

	prefetcher *spi.Prefetcher
	logger     *zap.Logger
}

// New creates a new Indexer instance
func New(cfg *config.Config, source spi.BlockSource, store spi.StateStore) *Indexer {
	backoff := util.NewBackoff(cfg.MaxRetries, cfg.RetryDelay)
	// Wrap source with retry logic
	retryingSource := spi.NewRetryingBlockSource(source, backoff)

	// Create prefetcher (buffer size 10 is hardcoded for now, could be config)
	prefetcher := spi.NewPrefetcher(retryingSource, 10)

	logger, _ := zap.NewProduction()

	return &Indexer{
		cfg:        cfg,
		source:     retryingSource,
		store:      store,
		monitor:    monitor.NewChainMonitor(retryingSource, store, cfg.SafeWindowSize),
		handlers:   make(map[string]core.Handler),
		prefetcher: prefetcher,
		logger:     logger,
	}
}

// On registers a handler for a specific event topic
// eventTopic should be the 0x-prefixed hex string of the event signature hash
func (i *Indexer) On(eventTopic string, handler core.Handler) {
	// Apply middlewares
	wrapped := handler
	for j := len(i.middlewares) - 1; j >= 0; j-- {
		wrapped = i.middlewares[j](wrapped)
	}
	i.handlers[eventTopic] = wrapped
}

// Use adds a middleware to the indexer.
// Note: Middleware must be added BEFORE registering handlers with On().
func (i *Indexer) Use(mw ...core.Middleware) {
	i.middlewares = append(i.middlewares, mw...)
}

// OnReorg registers a handler for reorg events
func (i *Indexer) OnReorg(handler core.ReorgHandler) {
	i.reorgHandler = handler
}

// Run starts the indexing process
func (i *Indexer) Run(ctx context.Context) error {
	i.logger.Info("Starting EtherFlow Indexer...")

	// Start metrics server
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		i.logger.Info("Starting metrics server on :9090")
		if err := http.ListenAndServe(":9090", nil); err != nil {
			i.logger.Error("Failed to start metrics server", zap.Error(err))
		}
	}()

	// 1. Load initial state
	lastBlock, err := i.store.GetLastBlock(ctx)
	if err != nil {
		i.logger.Error("Error loading initial state", zap.Error(err))
	}

	if lastBlock != nil {
		i.logger.Info("Resuming from block", zap.Uint64("block", lastBlock.Number))
		i.monitor.AddBlock(lastBlock)
	} else {
		i.logger.Info("No state found. Starting from configured block", zap.Uint64("start_block", i.cfg.StartBlock))
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
		i.logger.Error("Error processing block", zap.Error(err))
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
				i.logger.Error("Error processing block", zap.Error(err))
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
	metrics.ChainHead.Set(float64(latestHeight))

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
		// Calculate lag even when we are waiting
		lag := float64(latestHeight) - float64(nextHeight-1)
		if lag < 0 {
			lag = 0
		}
		metrics.ProcessingLag.Set(lag)
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
		i.logger.Warn("Block mismatch, resetting prefetcher",
			zap.Uint64("expected", nextHeight),
			zap.Uint64("got", nextBlock.Number))
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
		metrics.ReorgCount.Inc()
		i.logger.Warn("Reorg detected!", zap.Uint64("block", nextBlock.Number))
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
	err = i.store.SaveBlock(ctx, nextBlock)
	if err == nil {
		metrics.CurrentHeight.Set(float64(nextBlock.Number))
		metrics.ProcessingLag.Set(float64(latestHeight - nextBlock.Number))
	}
	return err
}

func (i *Indexer) processBlockEvents(ctx context.Context, block *core.Block, isReorg bool) error {
	if i.cfg.ParallelProcessing {
		// Parallel processing using errgroup
		g, ctx := errgroup.WithContext(ctx)

		for _, log := range block.Logs {
			if len(log.Topics) == 0 {
				continue
			}
			topic0 := string(log.Topics[0])

			// Only launch goroutine if we have a handler
			if handler, ok := i.handlers[topic0]; ok {
				// Copy for closure
				log := log
				g.Go(func() error {
					eventCtx := &core.EventContext{
						Context: ctx,
						Block:   block,
						Log:     &log,
						IsReorg: isReorg,
					}
					return handler(eventCtx)
				})
			}
		}
		return g.Wait()
	}

	// Sequential processing (default)
	for _, log := range block.Logs {
		if len(log.Topics) == 0 {
			continue
		}

		// Optimistic Topic-based Routing
		topic0 := string(log.Topics[0])
		if handler, ok := i.handlers[topic0]; ok {
			// Create context
			eventCtx := &core.EventContext{
				Context: ctx,
				Block:   block,
				Log:     &log,
				IsReorg: isReorg,
			}

			if err := handler(eventCtx); err != nil {
				i.logger.Error("Handler failed",
					zap.String("topic", topic0),
					zap.Uint64("block", block.Number),
					zap.Error(err))
				// Decided: should we fail the block? For now, yes.
				return err
			}
		}
	}
	return nil
}
