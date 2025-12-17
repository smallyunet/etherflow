# Getting Started

## Installation

```bash
go get github.com/username/etherflow
```

## Basic Usage

Here is a simple example of how to setup an indexer to listen for Transfer events.

```go
package main

import (
    "context"
    "log"

    "github.com/username/etherflow"
    "github.com/username/etherflow/pkg/spi/eth" // Hypothethical
    "github.com/username/etherflow/pkg/spi/memory" // Hypothethical
)

func main() {
    // 1. Setup dependencies
    client := eth.NewClient("https://mainnet.infura.io/v3/YOUR_KEY")
    store := memory.NewStateStore() // Or a persistent store

    // 2. Create Indexer
    indexer := etherflow.New(client, store)

    // 3. Register Handlers
    indexer.On("Transfer(address,address,uint256)", func(ctx *core.EventContext) error {
        log.Printf("Transfer detected in block %d", ctx.Block.Number)
        return nil
    })

    // 4. Run
    if err := indexer.Run(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

## Configuration

You can configure the polling interval and start block:

```go
indexer.SetPollingInterval(5 * time.Second)
indexer.SetStartBlock(12000000)
```
