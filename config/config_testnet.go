//go:build testnet && !unittest

package config

const P2P_BIND_PORT = 16310
const RPC_BIND_PORT = 16311
const STRATUM_BIND_PORT = 16312
const NETWORK_ID uint64 = 0x5af15cf1542ba49a // Network identifier. It MUST be unique for each chain

const NETWORK_NAME = "testnet"

const MIN_DIFFICULTY = 100
const DIFFICULTY_N = 5 * 60 / TARGET_BLOCK_TIME // DAA half-life (5 minutes).

// GENESIS BLOCK INFO
const GENESIS_ADDRESS = "vo3yexhnu89af4aai83uou17dupb79c3gxng1q"
const GENESIS_TIMESTAMP = 0
const BLOCK_REWARD_FEE_PERCENT = 10

var SEED_NODES = []string{"127.0.0.1"}

// PROOF OF STAKE
const MIN_STAKE_AMOUNT = 100 * COIN
const REGISTER_DELEGATE_BURN = 1_000 * COIN

// HARD-FORKS
const HARDFORK_V2_HEIGHT = 1
const HARDFORK_V3_HEIGHT = 10
