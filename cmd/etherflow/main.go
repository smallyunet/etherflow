package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/username/etherflow"
	// Ensure we import the sub-package logic if we put it there, but wait, I put it in main package? No I put it in etherflow package?
	// Wait, I put generator.go in pkg/gen. I need to handle command parsing here.
	"github.com/username/etherflow/pkg/config"
	"github.com/username/etherflow/pkg/core"
	genpkg "github.com/username/etherflow/pkg/gen" // Alias to avoid conflict
	"github.com/username/etherflow/pkg/spi"
	"github.com/username/etherflow/pkg/spi/eth"
	"github.com/username/etherflow/pkg/spi/store/pg"
	"github.com/username/etherflow/pkg/spi/store/redis"
	"github.com/username/etherflow/pkg/spi/store/sqlite"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "gen" {
		runGen()
		return
	}

	runIndexer()
}

func runGen() {
	genCmd := flag.NewFlagSet("gen", flag.ExitOnError)
	abiPath := genCmd.String("abi", "", "Path to ABI file")
	pkgName := genCmd.String("pkg", "handlers", "Package name for generated code")
	outPath := genCmd.String("out", "handlers.go", "Output file path")

	if err := genCmd.Parse(os.Args[2:]); err != nil {
		fmt.Printf("Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	if *abiPath == "" {
		fmt.Println("Error: --abi is required")
		genCmd.PrintDefaults()
		os.Exit(1)
	}

	opts := genpkg.GenerateOptions{
		ABIPath:     *abiPath,
		PackageName: *pkgName,
		OutputPath:  *outPath,
	}

	if err := genpkg.Generate(opts); err != nil {
		fmt.Printf("Error generating bindings: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated bindings in %s\n", *outPath)
}

func runIndexer() {
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
	case "redis":
		log.Printf("Using Redis store at: %s", cfg.DBPath)
		// Assuming DBPath for redis is "addr:password:db" or just "addr"
		// Simplified parsing:
		store, err = redis.NewStore(cfg.DBPath, "", 0)
	default:
		log.Fatalf("Unknown DB driver: %s. Supported: sqlite, postgres, redis", cfg.DBDriver)
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
