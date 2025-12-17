# Architecture

EtherFlow is designed with modularity in mind.

## Core Components

### Indexer

The `Indexer` struct is the main entry point. It orchestrates the flow of data from the source to your handlers and ensures state consistency.

### BlockSource (`spi.BlockSource`)

This interface abstracts the interaction with the blockchain node. It allows us to swap out the underlying RPC client or use a mock for testing.

```go
type BlockSource interface {
    LatestBlock(ctx context.Context) (uint64, error)
    GetBlockByNumber(ctx context.Context, number uint64) (*core.Block, error)
}
```

### StateStore (`spi.StateStore`)

This interface abstraction allows EtherFlow to persist its progress. It stores the last processed block and supports rewinding state in case of re-orgs.

### ChainMonitor

The `ChainMonitor` is an internal component responsible for detecting reorgs. It maintains a buffer of recent blocks and compares them against new blocks coming from the `BlockSource`.
