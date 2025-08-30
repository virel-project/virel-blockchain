package blockchain

import (
	"fmt"

	"github.com/virel-project/virel-blockchain/v2/adb"
	"github.com/virel-project/virel-blockchain/v2/util"
	"github.com/virel-project/virel-blockchain/v2/util/uint128"
)

func (bc *Blockchain) GetStaker(txn adb.Txn, h util.Hash, stats *Stats) (*Delegate, error) {
	coinIndex := hashToCoinIndex(h, stats.StakedAmount)

	var coinsSeen uint64

	var delegate *Delegate

	// find the correct delegate who should stake
	err := bc.GetDelegates(txn, func(d *Delegate) (bool, error) {
		t := d.TotalAmount()

		ocs := coinsSeen
		coinsSeen += t
		if coinsSeen < ocs {
			panic("GetStaker overflow")
		}

		if coinIndex <= coinsSeen {
			delegate = d
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		return nil, fmt.Errorf("error getting delegates: %w", err)
	}

	if delegate == nil {
		return nil, fmt.Errorf("no valid delegate found")
	}

	return delegate, nil
}

func (bc *Blockchain) GetDelegates(txn adb.Txn, f func(d *Delegate) (bool, error)) error {
	return txn.ForEachInterrupt(bc.Index.Delegate, func(k, v []byte) (bool, error) {
		d := &Delegate{}

		err := d.Deserialize(v)
		if err != nil {
			return false, err
		}

		return f(d)
	})
}

func hashToCoinIndex(hash [32]byte, stakedSupply uint64) uint64 {
	// modulo bias is negligible if we use uint128 math
	hashValue := uint128.FromBytes(hash[:16])
	return hashValue.Mod64(stakedSupply)
}
