# Handling Reorgs

EtherFlow provides built-in mechanisms to detect and handle chain reorganizations (reorgs).

## Detection
The `ChainMonitor` checks the parent hash of each new block against the hash of the last indexed block.
- If they match: Normal chain extension.
- If they mismatch: Potential reorg.

## Resolution
When a reorg is detected, EtherFlow finds the **Fork Point** (common ancestor) by tracing back the history.

1. **Find Fork Point**: Traverse back until a common block is found in both local store and remote chain.
2. **Rewind**: Delete local state (blocks) after the fork point.
3. **Replay**: Fetch and index blocks from the new chain starting from the fork point.

## Custom Handling
You can register a custom handler to react to reorgs (e.g., to notify external systems or rollback external state).

```go
indexer.OnReorg(func(ctx context.Context, forkPoint *core.Block, oldChain []*core.Block, newChain []*core.Block) error {
    log.Printf("Reorg detected at block %d", forkPoint.Number)
    // Custom logic here
    return nil
})
```
