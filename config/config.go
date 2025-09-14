package config

import "time"

const NAME = "virel"

const VERSION_MAJOR = 3
const VERSION_MINOR = 0
const VERSION_PATCH = 3

const COIN = 1_000_000_000                     // 1e9
const FEE_PER_BYTE = 500_000                   // 0.0615 coins for an 1-output tx
const FEE_PER_BYTE_V2 = 2_000_000              // 0.246 coins for an 1-output tx
const BLOCK_REWARD = 175 * COIN                // initial block reward
const REDUCTION_INTERVAL = BLOCKS_PER_DAY * 91 // block reward reduces by 10% every 91 days (4 times a year)

// The exact number is slightly smaller than this because of rounding errors. You can see the accurate result
// using the reward_test.go file. Error is less than 0.00000005% so it doesn't matter much, anyway.
const MAX_SUPPLY = REDUCTION_INTERVAL*BLOCK_REWARD*10 +
	(BLOCK_REWARD * REDUCTION_INTERVAL) // also include initial flat-reward phase

const P2P_CONNECTIONS = 16
const P2P_PING_INTERVAL = 5
const P2P_TIMEOUT = 20

const P2P_VERSION = 2

const MAX_TX_PER_BLOCK = 1_000
const MAX_HEIGHT = 5_000_000_000

const TARGET_BLOCK_TIME = 15
const FUTURE_TIME_LIMIT = 10

const MEMPOOL_EXPIRATION = 2 * time.Hour

const MAX_BLOCK_SIZE = 64 * 1024 // Hard cap for the maximum VSize of a block (64 KiB)

const MINIDAG_ANCESTORS = 3 // number of ancestors saved for each block
const MAX_SIDE_BLOCKS = 2   // max number of side blocks that can be referenced by a block

const MAX_OUTPUTS = 32 // max output count for a transaction

const UPDATE_CHECK_URL = "https://api.github.com/repos/virel-project/virel-blockchain/releases/latest"

const DELEGATE_ADDRESS_PREFIX = "delegate"
