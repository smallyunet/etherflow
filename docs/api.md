# EtherFlow API Reference

## Components

### Indexer
The `Indexer` is the main entry point.

```go
indexer := etherflow.New(cfg, source, store)
```

#### Methods
- `On(eventTopic string, handler core.Handler)`: Register a handler for a specific event topic.
- `Use(mw ...core.Middleware)`: Register middleware to run before handlers.
- `OnReorg(handler core.ReorgHandler)`: Register a handler for chain reorganizations.
- `Run(ctx context.Context) error`: Start the indexing loop.

### Middleware
Middleware allows wrapping handlers with custom logic.

```go
type Middleware func(Handler) Handler
```

Example Logging Middleware:
```go
indexer.Use(func(next core.Handler) core.Handler {
    return func(ctx *core.EventContext) error {
        log.Println("Processing event...")
        return next(ctx)
    }
})
```

### CLI
Generate type-safe bindings from ABI:

```bash
go run cmd/etherflow/main.go gen --abi <abi_file> --pkg <package> --out <output_file>
```
