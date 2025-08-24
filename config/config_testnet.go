//go:build testnet

package config

const P2P_BIND_PORT = 16310
const RPC_BIND_PORT = 16311
const STRATUM_BIND_PORT = 16312
const NETWORK_ID uint64 = 0x5af15cf1542ba49a // Network identifier. It MUST be unique for each chain

const NETWORK_NAME = "testnet"

const MIN_DIFFICULTY = 1000

// GENESIS BLOCK INFO
const GENESIS_ADDRESS = "vo3yexhnu89af4aai83uou17dupb79c3gxng1q"
const GENESIS_TIMESTAMP = 0
const BLOCK_REWARD_FEE_PERCENT = 10

var SEED_NODES = []string{"127.0.0.1"}
