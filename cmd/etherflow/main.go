package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/username/etherflow"
	"github.com/username/etherflow/pkg/core"
	"github.com/username/etherflow/pkg/spi/eth"
	"github.com/username/etherflow/pkg/spi/store/sqlite"
)

func main() {
	// 1. Configuration
	pflag.String("rpc-url", "http://localhost:8545", "Ethereum JSON-RPC URL")
	pflag.String("db-path", "etherflow.db", "Path to SQLite database")
	pflag.Uint64("start-block", 0, "Block number to start from (if no state exists)")
	pflag.String("poll-interval", "2s", "Polling interval")
	pflag.Parse()

	viper.BindPFlags(pflag.CommandLine)
	viper.SetEnvPrefix("ETHERFLOW")
	viper.AutomaticEnv()

	rpcURL := viper.GetString("rpc-url")
	dbPath := viper.GetString("db-path")
	startBlock := viper.GetUint64("start-block")
	pollIntervalStr := viper.GetString("poll-interval")

	pollInterval, err := time.ParseDuration(pollIntervalStr)
	if err != nil {
		log.Fatalf("Invalid poll-interval: %v", err)
	}

	log.Printf("Starting EtherFlow (v0.0.2)")
	log.Printf("RPC: %s", rpcURL)
	log.Printf("DB:  %s", dbPath)

	// 2. Dependencies
	client, err := eth.NewClient(rpcURL)
	if err != nil {
		log.Fatalf("Failed to connect to RPC: %v", err)
	}

	store, err := sqlite.NewStore(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}

	// 3. Indexer
	indexer := etherflow.New(client, store)
	indexer.SetStartBlock(startBlock)
	indexer.SetPollingInterval(pollInterval)

	// Register some default handlers for viewing
	// In a real app, users would import etherflow and register their own.
	// This main.go acts as a standalone runner, so maybe we just log everything?
	// OR, we can provide a plugin system later. 
	// For now, let's just log reorgs and maybe basic block progress is handled by the indexer logic (which calls handlers).
	
	// Add a dummy handler to show it's working if they don't modify code
	// Actually, the user requirement didn't specify what events to listen to.
	// But without handlers, it just indexes blocks.
	
	indexer.OnReorg(func(ctx context.Context, forkPoint *core.Block, oldChain []*core.Block, newChain []*core.Block) error {
		log.Printf("[REORG] Resolved fork at %d. Old chain len: %d, New chain len: %d", 
			forkPoint.Number, len(oldChain), len(newChain))
		return nil
	})

	// 4. Run with Graceful Shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		cancel()
	}()

	if err := indexer.Run(ctx); err != nil {
		log.Fatalf("Indexer stopped with error: %v", err)
	}
	log.Println("Goodbye.")
}
