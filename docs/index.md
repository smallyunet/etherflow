# EtherFlow

**EtherFlow** is a lightweight, reorg-aware EVM data indexing framework for Go. It simplifies the process of listening to chain events and handling complex scenarios like chain reorganizations.

## Key Features

- **Reorg Handling**: Automatically detects chain forks and handles rollbacks.
- **Source Agnostic**: Works with any standard EVM JSON-RPC provider.
- **Storage Agnostic**: Pluggable state storage (SQL, KV, etc.).
- **Simple API**: Gin-style handler registration for events.

## Why EtherFlow?

Building robust indexers is hard. You have to deal with reliable RPC connections, decoding log data, and most notoriously, **chain reorgs**. EtherFlow abstracts these complexities, allowing you to focus on your business logic.

## License

MIT
