package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/username/etherflow"
	"github.com/username/etherflow/pkg/config"
	"github.com/username/etherflow/pkg/core"
	"github.com/username/etherflow/pkg/spi"
	"github.com/username/etherflow/pkg/spi/eth"
	"github.com/username/etherflow/pkg/spi/store/pg"
	"github.com/username/etherflow/pkg/spi/store/sqlite"
)

func main() {
	configPath := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	if *configPath != "" {
		os.Setenv("ETHERFLOW_CONFIG_PATH", *configPath)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Starting EtherFlow (v0.0.2)")
	log.Printf("RPC: %s", cfg.RPCURL)
	log.Printf("DB Driver: %s", cfg.DBDriver)

	// 1. Setup Source
	source, err := eth.NewClient(cfg.RPCURL)
	if err != nil {
		log.Fatalf("Failed to connect to RPC at %s: %v", cfg.RPCURL, err)
	}

	// 2. Setup Store
	var store spi.StateStore
	switch cfg.DBDriver {
	case "postgres":
		log.Printf("Using PostgreSQL store with DSN provided in config")
		store, err = pg.NewStore(cfg.DBPath)
	case "sqlite":
		log.Printf("Using SQLite store at: %s", cfg.DBPath)
		store, err = sqlite.NewStore(cfg.DBPath)
	default:
		log.Fatalf("Unknown DB driver: %s. Supported: sqlite, postgres", cfg.DBDriver)
	}

	if err != nil {
		log.Fatalf("Failed to initialize store (%s): %v", cfg.DBDriver, err)
	}

	// 3. Initialize Indexer
	indexer := etherflow.New(cfg, source, store)

	// Register generic reorg handler
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
