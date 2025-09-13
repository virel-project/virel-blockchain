package block

import (
	"github.com/virel-project/virel-blockchain/v3/config"
)

func reduce(n, count uint64) uint64 {
	if count == 0 {
		return n
	}
	return reduce(n*9/10, count-1)
}

func (b Block) Reward() uint64 {
	return Reward(b.Height)
}

func Reward(height uint64) uint64 {
	// supply reduction every 60 days (2 months)
	reductions := height / config.REDUCTION_INTERVAL
	if reductions == 0 {
		return config.BLOCK_REWARD
	}

	return reduce(config.BLOCK_REWARD, reductions-1)
}

func supplyAtPhase(reductions uint64) uint64 {
	var supply uint64
	for reductions > 0 {
		supply += Reward(config.REDUCTION_INTERVAL*reductions-1) * config.REDUCTION_INTERVAL
		reductions--
	}
	return supply
}

func GetSupplyAtHeight(height uint64) uint64 {
	reductions := height / config.REDUCTION_INTERVAL
	reductionBlocks := height%config.REDUCTION_INTERVAL + 1
	phaseSupply := supplyAtPhase(reductions)
	return phaseSupply + Reward(height)*reductionBlocks
}
