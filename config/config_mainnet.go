//go:build !testnet

package config

const P2P_BIND_PORT = 6310
const RPC_BIND_PORT = 6311
const STRATUM_BIND_PORT = 6312
const NETWORK_ID uint64 = 0xd38dab1d4676d0c5 // Network identifier. It MUST be unique for each chain

const NETWORK_NAME = "mainnet"

const MIN_DIFFICULTY = 100_000

// GENESIS BLOCK INFO
const GENESIS_ADDRESS = "v139diixrpv0ftmip4mgpuy92u51iq4pnmgjsfn"
const GENESIS_TIMESTAMP = 1755522000 * 1000
const BLOCK_REWARD_FEE_PERCENT = 10

var SEED_NODES = []string{"82.153.138.24"}

// HARD-FORKS

// Hardfork V1: transaction version field
const HARDFORK_V2_HEIGHT = 100_000

// Hardfork V2: Hybrid PoW/PoS
const HARDFORK_V3_HEIGHT = 100_000 // TODO
