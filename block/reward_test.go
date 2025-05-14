package block

import (
	"testing"
	"virel-blockchain/config"
	"virel-blockchain/util"
)

func TestReward(t *testing.T) {
	var supply uint64 = 0
	var lastRed uint64 = 0
	var rew uint64 = Reward(0)
	for height := uint64(0); height < config.BLOCKS_PER_DAY*365*200; height++ {
		reductions := height / config.REDUCTION_INTERVAL
		if reductions != lastRed {
			rew = Reward(height)
			lastRed = reductions
		}
		supply += rew
		if height%(config.BLOCKS_PER_DAY*365) == 0 {
			t.Logf("supply at year %d is %s", height/(config.BLOCKS_PER_DAY*365), util.FormatCoin(supply))
			if GetSupplyAtHeight(height) != supply {
				t.Fatalf("height %d: mismatched supply %d, should be %d", height, GetSupplyAtHeight(height),
					supply)
			}
		}
		if rew == 0 {
			t.Logf("reward 0 reached after %d reductions", reductions)
			break
		}
	}
	t.Logf("max supply is %d (%s)", supply, util.FormatCoin(supply))
	// allow a small margin of error due to loss of precision of uint64 divisions
	const errorMargin = 0.000000001
	if float64(supply) < float64(config.MAX_SUPPLY)*(1-errorMargin) ||
		float64(supply) > float64(config.MAX_SUPPLY)*(1+errorMargin) {
		t.Fatalf("unexpected max supply %s: config.MAX_SUPPLY is %s", util.FormatCoin(supply),
			util.FormatCoin(config.MAX_SUPPLY))
	}
	t.Logf("block reward discrepancy: %.9f %%", (1-float64(supply)/float64(config.MAX_SUPPLY))*100)
}
