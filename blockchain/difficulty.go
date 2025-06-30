package blockchain

import (
	"github.com/virel-project/virel-blockchain/adb"
	"github.com/virel-project/virel-blockchain/block"
	"github.com/virel-project/virel-blockchain/config"
	"github.com/virel-project/virel-blockchain/util/uint128"
)

// LTTC: maximum deviation in block timestamp before the algorithm starts adjusting the difficulty
const maxDeviation = config.TARGET_BLOCK_TIME * 1000 * 2 * config.DIFFICULTY_N

// returns the difficulty of the block after the provided block
func (bc *Blockchain) GetNextDifficulty(tx adb.Txn, bl *block.Block) (uint128.Uint128, error) {
	if bl.Height < 2 {
		return uint128.From64(config.MIN_DIFFICULTY), nil
	}

	prev, err := bc.GetBlock(tx, bl.PrevHash())
	if err != nil {
		return uint128.Zero, err
	}

	diff := bl.Difficulty

	// this is safe to do, because blocks cannot have a decreasing timestamp (protocol rule)
	var deltaTime uint64 = (bl.Timestamp - prev.Timestamp)

	// make sure that deltaTime is not too small
	if deltaTime < 100 {
		deltaTime = 100
	}

	// LTTC enabled if GENESIS_TIMESTAMP is set
	if config.GENESIS_TIMESTAMP != 0 {
		expectedBlockTime := bl.Height*config.TARGET_BLOCK_TIME*1000 + config.GENESIS_TIMESTAMP
		timeDeviation := int64(bl.Timestamp) - int64(expectedBlockTime)
		if timeDeviation > maxDeviation { // block is too old
			deltaTime = deltaTime * 3 / 2 // multiply deltaTime by 3/2, so the difficulty decreases
		} else if timeDeviation < -maxDeviation { // block is too recent
			deltaTime = deltaTime * 2 / 3 // multiply deltaTime by 2/3, so the difficulty increases
		}
		Log.Debug("LTTC deviation:", float64(timeDeviation)/1000)
	}

	// compute difficulty using EMA algorithm
	newDiff := difficultyEMA(deltaTime, bl.Difficulty)

	Log.Debug("diff:", diff, "->", newDiff)

	if newDiff.Cmp64(config.MIN_DIFFICULTY) < 0 {
		newDiff = uint128.From64(config.MIN_DIFFICULTY)
	}

	return newDiff, nil
}

func difficultyEMA(solveTime uint64, prevDiff uint128.Uint128) uint128.Uint128 {
	const N = config.DIFFICULTY_N
	const target = config.TARGET_BLOCK_TIME * 1000

	// Convert solveTime and prevDiff to Uint128 and calculate the next difficulty.
	// (prevDiff * N * target) / (N*target - target + solveTime)
	num := prevDiff.Mul64(N * target)            // prevDiff * N * target
	den := uint64(N)*target - target + solveTime // (N * target - target + solveTime)
	nextD := num.Div64(den)                      // num / den

	return nextD
}
