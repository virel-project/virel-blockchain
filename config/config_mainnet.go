//go:build !testnet && !unittest

package config

const P2P_BIND_PORT = 6310
const RPC_BIND_PORT = 6311
const STRATUM_BIND_PORT = 6312
const NETWORK_ID uint64 = 0xd38dab1d4676d0c5 // Network identifier. It MUST be unique for each chain

const NETWORK_NAME = "mainnet"

const MIN_DIFFICULTY = 100_000
const DIFFICULTY_N = 120 // DAA half-life (30 minutes).

// GENESIS BLOCK INFO
const GENESIS_ADDRESS = "v139diixrpv0ftmip4mgpuy92u51iq4pnmgjsfn"
const GENESIS_TIMESTAMP = 1755522000 * 1000
const BLOCK_REWARD_FEE_PERCENT = 10

var SEED_NODES = []string{"82.153.138.24"}

// PROOF OF STAKE
const MIN_STAKE_AMOUNT = 100 * COIN
const REGISTER_DELEGATE_BURN = 1_000 * COIN

// HARD-FORKS

// Hardfork V2: transaction version field.
// Tx version: 1, block version: 0
const HARDFORK_V2_HEIGHT = 100_000

// Hardfork V3: Hybrid PoW/PoS
// Tx version: 1-5, block version: 1
const HARDFORK_V3_HEIGHT = 100_000 // TODO
