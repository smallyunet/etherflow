# EtherFlow

EtherFlow is a resilient, minimalist EVM indexing framework for Go. It is designed to handle reorgs robustly and provide a simple API for indexing EVM-based chains.

## Features

- **Robust Reorg Handling**: Automatically detects and handles chain reorganizations, ensuring data consistency even across process restarts.
- **Pluggable Architecture**:
    - **Sources**: Standard JSON-RPC support (Geth, Erigon, Reth, etc.).
    - **Stores**: Persistence support for SQLite (embedded) and PostgreSQL.
- **Resiliency**: Built-in exponential backoff and retry logic for RPC calls.
- **Simple API**: Easy-to-use event subscription model.

## Getting Started

### Installation

```bash
go get github.com/username/etherflow
```

### Usage

```go
package main

import (
	"context"
	"log"

	"github.com/username/etherflow"
	"github.com/username/etherflow/pkg/config"
	"github.com/username/etherflow/pkg/spi/eth"
	"github.com/username/etherflow/pkg/spi/store/sqlite"
)

func main() {
	// 1. Configure
	cfg := &config.Config{
		RPCURL:         "https://eth-mainnet.g.alchemy.com/v2/your-api-key",
		DBDriver:       "sqlite",
		DBPath:         "etherflow.db",
		StartBlock:     18000000,
		SafeWindowSize: 128,
	}

	// 2. Setup Components
	source, _ := eth.NewClient(cfg.RPCURL)
	store, _ := sqlite.NewStore(cfg.DBPath)

	// 3. Create Indexer
	indexer := etherflow.New(cfg, source, store)

	// 4. Register Handlers
	indexer.On("Transfer", func(ctx *core.EventContext) error {
		log.Printf("Transfer event: %s", ctx.Log.TxHash)
		return nil
	})

	// 5. Run
	if err := indexer.Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}
```

## Configuration

EtherFlow can be configured via environment variables or a YAML config file.

| Config Key | Environment Variable | Default | Description |
| :--- | :--- | :--- | :--- |
| `rpc_url` | `ETHERFLOW_RPC_URL` | `http://localhost:8545` | RPC endpoint URL. |
| `db_driver` | `ETHERFLOW_DB_DRIVER` | `sqlite` | Database driver (`sqlite` or `postgres`). |
| `db_path` | `ETHERFLOW_DB_PATH` | `etherflow.db` | Path (SQLite) or DSN (Postgres). |
| `polling_interval` | `ETHERFLOW_POLLING_INTERVAL` | `2s` | Interval to check for new blocks. |
| `start_block` | `ETHERFLOW_START_BLOCK` | `0` | Block number to start indexing from (if no state exists). |
| `max_retries` | `ETHERFLOW_MAX_RETRIES` | `5` | Max retries for RPC calls. |
| `retry_delay` | `ETHERFLOW_RETRY_DELAY` | `1s` | Base delay for exponential backoff. |
| `safe_window_size` | `ETHERFLOW_SAFE_WINDOW_SIZE` | `128` | Number of blocks to keep in history/reorg window. |

## Roadmap

- [x] **Phase 1: Foundation (MVP)** - Core loop, RPC, Persistence, Reorg Handling.
- [ ] **Phase 2: Performance & Reliability** - Concurrency, Websockets, Metrics.
- [ ] **Phase 3: Developer Experience (DX)** - Type-safe bindings, Middleware.
- [ ] **Phase 4: Ecosystem & Stability** - Redis adapter, More tests, Documentation.

See [ROADMAP.md](docs/ROADMAP.md) for details.
