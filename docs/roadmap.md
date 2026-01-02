# EtherFlow Roadmap

This document outlines the development plan for EtherFlow, moving from the current prototype to a production-ready EVM indexing framework.

## 🚀 Phase 1: Foundation (MVP)
**Goal**: Replace mocks with real implementations and verify core mechanics.

- [x] **RPC Integration**
    - Implement `spi.BlockSource` using `go-ethereum/ethclient`.
    - Support standard JSON-RPC methods (`eth_getBlockByNumber`, `eth_getLogs`).
- [x] **Persistence Layer**
    - Implement `spi.StateStore` using a SQL driver (PostgreSQL/SQLite).
    - Schema design for tracking cursor (block number, hash) and reorg history.
- [x] **Core Loop Refinement**
    - Implement robust error handling and exponential backoff for RPC failures.
    - Validate the `ChainMonitor` logic against a local testnet (Anvil/Hardhat) with simulated reorgs.
- [x] **Configuration**
    - Design a configuration structure (YAML/Env) for RPC URLs, DB connection strings, and safety window sizes.

## ⚡ Phase 2: Performance & Reliability
**Goal**: Optimize for throughput and observability.

- [ ] **Concurrency Model**
    - Allow parallel processing of logs within a block (optional, order-dependent).
    - [x] Async block fetching (prefetching next blocks).
- [ ] **Advanced Source Features**
    - Support WebSocket subscriptions (`eth_subscribe`) for lower latency.
    - Batch RPC requests to reduce network overhead.
- [ ] **Observability**
    - Integrate Prometheus metrics:
        - `etherflow_current_height`
        - `etherflow_chain_head`
        - `etherflow_reorg_count`
        - `etherflow_processing_lag`
    - Structured logging (Zap/Logrus).

## 🛠 Phase 3: Developer Experience (DX)
**Goal**: Simplify integration and reduce boilerplate.

- [ ] **Type-Safe Bindings**
    - Create a CLI tool to generate handler signatures from ABI files.
    - Example: `etherflow gen --abi ./token.abi --pkg handlers`
- [ ] **Middleware System**
    - Add support for middleware in the event processing pipeline (e.g., for tracing, custom metrics).
- [ ] **Filter Optimization**
    - Smart log filtering: Only fetch logs for registered addresses/topics to save bandwidth.

## 📦 Phase 4: Ecosystem & Stability
**Goal**: Prepare for v1.0.0 release.

- [ ] **Storage Adapters**
    - Add support for Redis and KV stores (BadgerDB/LevelDB).
- [ ] **Testing**
    - Comprehensive unit tests for `ChainMonitor`.
    - Integration tests with Dockerized Geth/Postgres.
- [ ] **Documentation**
    - API Reference.
    - "How to handle Reorgs" guide.
    - Example projects (ERC20 Indexer, NFT Tracker).

## Future Ideas
- **Multi-Chain Support**: Run multiple indexers in a single process.
- **Backfill Mode**: Optimized mode for indexing historical data (skipping reorg checks for finalized blocks).
