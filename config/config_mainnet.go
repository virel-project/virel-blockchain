//go:build !testnet

package config

const P2P_BIND_PORT = 6313
const RPC_BIND_PORT = 6314
const STRATUM_BIND_PORT = 6315
const NETWORK_ID uint64 = 0x4af15cf1542ba49a // Network identifier. It MUST be unique for each chain

const NETWORK_NAME = "stagenet"

// GENESIS BLOCK INFO
const GENESIS_ADDRESS = "so3yexhnu89af4aai83uou17dupb79c3gxng1q"
const GENESIS_TIMESTAMP = 0
const BLOCK_REWARD_FEE_PERCENT = 10

var SEED_NODES = []string{"82.153.138.24"}
