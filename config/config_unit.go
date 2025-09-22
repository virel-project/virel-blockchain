//go:build !testnet && unittest

package config

const P2P_BIND_PORT = 16390
const RPC_BIND_PORT = 16391
const STRATUM_BIND_PORT = 16392
const NETWORK_ID uint64 = 0x1 // Network identifier. It MUST be unique for each chain

const NETWORK_NAME = "unittest"

const MIN_DIFFICULTY = 1
const DIFFICULTY_N = 120 // DAA half-life (30 minutes).

// GENESIS BLOCK INFO
const GENESIS_ADDRESS = "vo3yexhnu89af4aai83uou17dupb79c3gxng1q"
const GENESIS_TIMESTAMP = 0
const BLOCK_REWARD_FEE_PERCENT = 10
const TEAM_STAKE_PUBKEY = "3959a30cb83649dd38389dd6717cbadab6ceb92cd9e4c4352abfcf168bbf592e"

var SEED_NODES = []string{"127.0.0.1"}

// PROOF OF STAKE
const MIN_STAKE_AMOUNT = 1 * COIN
const REGISTER_DELEGATE_BURN = 1 * COIN
const STAKE_UNLOCK_TIME = 10 // staked funds unlock after 10 blocks

// HARD-FORKS
const HARDFORK_V2_HEIGHT = 1
const HARDFORK_V3_HEIGHT = 1
