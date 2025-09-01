package block

import (
	"fmt"

	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/bitcrypto"
	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/transaction"
)

// CoinbaseReward is a virtual transaction type: it is invalid for the network, but it is used
// for easier block processing.
type CoinbaseOutput struct {
	Recipient  address.Address
	Amount     uint64
	DelegateId uint64 // if zero, this is not a PoS output
	Type       transaction.StateOutputType
}

func (b *Block) CoinbaseTransaction(totalReward uint64) []CoinbaseOutput {
	switch b.Version {
	case 0:
		// 10% governance, 90% PoW

		governanceReward := totalReward * config.BLOCK_REWARD_FEE_PERCENT / 100
		powReward := totalReward - governanceReward

		return []CoinbaseOutput{{
			Recipient: address.GenesisAddress,
			Amount:    governanceReward,
			Type:      transaction.OUT_COINBASE_DEV,
		}, {
			Recipient: b.Recipient,
			Amount:    powReward,
			Type:      transaction.OUT_COINBASE_POW,
		}}
	case 1:
		// 10% governance, 45% PoW, 45% PoS

		governanceReward := totalReward * config.BLOCK_REWARD_FEE_PERCENT / 100
		communityReward := totalReward - governanceReward
		powReward := communityReward / 2
		posReward := communityReward - powReward
		burnReward := uint64(0)

		if b.StakeSignature == bitcrypto.BlankSignature {
			// for blocks without a valid stake signature, apply 10% burn to PoW reward and burn all the PoS reward.
			powReward = powReward * 9 / 10
			posReward = 0
			burnReward = communityReward - powReward
		} else {
			powReward = communityReward - posReward
		}

		outs := []CoinbaseOutput{{
			Recipient: address.GenesisAddress,
			Amount:    governanceReward,
			Type:      transaction.OUT_COINBASE_DEV,
		}, {
			Recipient: b.Recipient,
			Amount:    powReward,
			Type:      transaction.OUT_COINBASE_POW,
		}}

		if posReward != 0 {
			outs = append(outs, CoinbaseOutput{
				Recipient:  address.NewDelegateAddress(b.DelegateId),
				Amount:     posReward,
				Type:       transaction.OUT_COINBASE_POS,
				DelegateId: b.DelegateId,
			})
		}
		if burnReward != 0 {
			outs = append(outs, CoinbaseOutput{
				Recipient: address.INVALID_ADDRESS,
				Amount:    burnReward,
				Type:      transaction.OUT_COINBASE_BURN,
			})
		}

		return outs
	default:
		panic(fmt.Errorf("unexpected block version %d", b.Version))
	}
}
