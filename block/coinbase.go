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
		// 10% governance, 50% PoW, 40% PoS

		governanceReward := totalReward * config.BLOCK_REWARD_FEE_PERCENT / 100
		powReward := totalReward / 2 // 50% of total
		posReward := totalReward - powReward - governanceReward
		burnReward := uint64(0)

		if b.StakeSignature == bitcrypto.BlankSignature {
			// for blocks without a valid stake signature, apply 25% burn to PoW reward and burn all the PoS reward.
			powReward = powReward * 3 / 4
			posReward = 0
			burnReward = totalReward - governanceReward - powReward
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
