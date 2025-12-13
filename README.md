# EtherFlow

EtherFlow is a lightweight, reorg-aware EVM data indexing framework for Go. It simplifies the process of listening to chain events and handling complex scenarios like chain reorganizations.

## Features

- **Reorg Handling**: Automatically detects chain forks and handles rollbacks.
- **Source Agnostic**: Works with any standard EVM JSON-RPC provider.
- **Storage Agnostic**: Pluggable state storage (SQL, KV, etc.).
- **Simple API**: Gin-style handler registration.

## Roadmap

See [ROADMAP.md](ROADMAP.md) for the detailed development plan.

