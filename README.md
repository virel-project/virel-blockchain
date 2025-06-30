# Virel Blockchain
Copyright (c) 2025 The Virel Project. All rights reserved.

Virel is the world's first MiniDAG, a novel system that simulates a Directed Acyclic Graph (DAG) on a linear blockchain.  
Virel will be fairly launched and secured by multi-chain Proof-of-Work via Merge-Mining.

## Running from source
To run the Virel Blockchain node or CLI wallet you need the latest version of [Go](https://go.dev/) installed.

Build the node:
```sh
go build ./cmd/virel-node/
```

Build the wallet:
```sh
go build ./cmd/virel-wallet-cli/
```

You will find binaries inside the `cmd/virel-node` and `cmd/virel-wallet-cli` directories.

## Current phase
The Virel Blockchain is under active development. The current phase is "public stagenet": there *might be* be breaking changes, but you can run the Virel node and connet to the global stagenet.
The mainnet is expected in August 2025, but might be delayed if deemed necessary.