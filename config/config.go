package config

import "time"

const COIN = 1_000_000_000                     // 1e9
const FEE_PER_BYTE = 500_000                   // ~0.06 coins per tx
const BLOCK_REWARD = 184 * COIN                // initial block reward
const REDUCTION_INTERVAL = BLOCKS_PER_DAY * 90 // block reward reduces by 10% every 90 days

// The exact number is slightly smaller than this because of rounding errors. You can see the accurate result
// using the reward_test.go file. Error is less than 0.00000005% so it doesn't matter much, anyway.
const MAX_SUPPLY = REDUCTION_INTERVAL*BLOCK_REWARD*10 +
	(BLOCK_REWARD * REDUCTION_INTERVAL / 2) // also include initial half-reward phase

const P2P_CONNECTIONS = 1
const P2P_PING_INTERVAL = 5
const P2P_TIMEOUT = 40

const MAX_TX_PER_BLOCK = 1_000
const MAX_HEIGHT = 5_000_000_000

const MIN_DIFFICULTY = 1000
const TARGET_BLOCK_TIME = 15
const FUTURE_TIME_LIMIT = 10
const DIFFICULTY_N = 30 * 60 / TARGET_BLOCK_TIME // DAA half-life (30 minutes).

const MEMPOOL_EXPIRATION = 2 * time.Hour

const MAX_TX_SIZE = 512                      // Hard cap for the maximum VSize of a transaction
const MAX_BLOCK_SIZE = 1000 + 25*MAX_TX_SIZE // Hard cap for the maximum VSize of a block

const MINIDAG_ANCESTORS = 3 // number of ancestors saved for each block
const MAX_SIDE_BLOCKS = 2   // max number of side blocks that can be referenced by a block

const MAX_OUTPUTS = 16 // max output count for a transaction
