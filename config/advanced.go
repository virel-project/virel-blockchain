package config

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"time"
)

// This file holds advanced config options. You shouldn't edit these options unless you really know what you
// are doing.

const MAX_MERGE_MINED_CHAINS = 16
const DEFAULT_CHECKPOINT_INTERVAL = 32
const SEEDHASH_DURATION = 4 * (60 * 60 * 24) // seed hash changes once every 4 days

const BLOCKS_PER_DAY = 60 * 60 * 24 / TARGET_BLOCK_TIME

const STRATUM_READ_TIMEOUT = 5 * 60 * time.Second
const STRATUM_JOBS_HISTORY = MINIDAG_ANCESTORS

var ATOMIC = math.Round(math.Log10(COIN))

const WALLET_PREFIX = "v" // Wallet prefix should be the same for all merge mined chains

// True if the current chain is a Master Chain; if false, the node will try connecting to the masterchain
// node to send Merge Mining jobs
const IS_MASTERCHAIN = NETWORK_ID == 0xd38dab1d4676d0c5 // do not change this

const PARALLEL_BLOCKS_DOWNLOAD = 50

const VERSION = VERSION_MAJOR<<32 + VERSION_MINOR<<16 + VERSION_PATCH

// fork coins should not change this if they want to be easily merge mined
const HD_COIN_TYPE = 6310

var BinaryNetworkID = make([]byte, 8)

func init() {
	binary.LittleEndian.PutUint64(BinaryNetworkID, NETWORK_ID)
}
func AssertHexDec(s string) []byte {
	dat, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return dat
}
