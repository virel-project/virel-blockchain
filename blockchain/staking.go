package blockchain

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/virel-project/virel-blockchain/v3/adb"
	"github.com/virel-project/virel-blockchain/v3/chaintype"
	"github.com/virel-project/virel-blockchain/v3/p2p/packet"
	"github.com/virel-project/virel-blockchain/v3/util"
	"github.com/virel-project/virel-blockchain/v3/util/uint128"
)

// GetStaker returns the correct staker for a given block PrevHash.
// Can only be called when the stakedhash is at chain tip.
func (bc *Blockchain) GetStaker(txn adb.Txn, hash util.Hash, stats *Stats) (*chaintype.Delegate, error) {
	if stats.StakedAmount == 0 {
		// return dummy delegate
		return &chaintype.Delegate{
			Id:   0,
			Name: []byte("no delegate"),
		}, nil
	}

	coinIndex := hashToCoinIndex(hash, stats.StakedAmount)

	var coinsSeen uint64

	var delegate *chaintype.Delegate

	// find the correct delegate who should stake
	err := bc.GetDelegates(txn, func(d *chaintype.Delegate) (bool, error) {
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

func (bc *Blockchain) SetDelegate(txn adb.Txn, delegate *chaintype.Delegate) error {
	idb := make([]byte, 8)
	binary.LittleEndian.PutUint64(idb, delegate.Id)
	return txn.Put(bc.Index.Delegate, idb, delegate.Serialize())
}
func (bc *Blockchain) GetDelegate(txn adb.Txn, id uint64) (*chaintype.Delegate, error) {
	idb := make([]byte, 8)
	binary.LittleEndian.PutUint64(idb, id)

	delegatedata := txn.Get(bc.Index.Delegate, idb)
	if len(delegatedata) < 1 {
		return nil, errors.New("no delegate found")
	}
	delegate := &chaintype.Delegate{}
	err := delegate.Deserialize(delegatedata)
	if err != nil {
		return nil, err
	}
	return delegate, nil
}
func (bc *Blockchain) RemoveDelegate(txn adb.Txn, delegateId uint64) error {
	idb := make([]byte, 8)
	binary.LittleEndian.PutUint64(idb, delegateId)
	return txn.Del(bc.Index.Delegate, idb)
}
func (bc *Blockchain) GetDelegates(txn adb.Txn, f func(d *chaintype.Delegate) (bool, error)) error {
	return txn.ForEachInterrupt(bc.Index.Delegate, func(k, v []byte) (bool, error) {
		d := &chaintype.Delegate{}

		err := d.Deserialize(v)
		if err != nil {
			return false, err
		}

		return f(d)
	})
}

func (bc *Blockchain) SetDelegateHistory(txn adb.Txn, blockhash util.Hash, delegate *chaintype.Delegate) error {
	return txn.Put(bc.Index.DelegateHistory, blockhash[:], delegate.Serialize())
}
func (bc *Blockchain) GetDelegateHistory(txn adb.Txn, blockhash util.Hash) (*chaintype.Delegate, error) {
	delegatedata := txn.Get(bc.Index.DelegateHistory, blockhash[:])
	if len(delegatedata) < 1 {
		return nil, errors.New("no delegate found")
	}
	delegate := &chaintype.Delegate{}
	err := delegate.Deserialize(delegatedata)
	if err != nil {
		return nil, err
	}
	return delegate, nil
}

func (bc *Blockchain) GetStakeSig(txn adb.Txn, hash util.Hash) (*packet.PacketStakeSignature, error) {
	stakesigdata := txn.Get(bc.Index.StakeSig, hash[:])
	if len(stakesigdata) < 1 {
		return nil, errors.New("no stake signature found")
	}
	stakesig := &packet.PacketStakeSignature{}
	err := stakesig.Deserialize(stakesigdata)
	if err != nil {
		return nil, err
	}
	return stakesig, nil
}
func (bc *Blockchain) SetStakeSig(txn adb.Txn, stakesig *packet.PacketStakeSignature) error {
	return txn.Put(bc.Index.StakeSig, stakesig.Hash[:], stakesig.Serialize())
}
func (bc *Blockchain) RemoveStakeSig(txn adb.Txn, blockhash util.Hash) error {
	return txn.Del(bc.Index.StakeSig, blockhash[:])
}

func hashToCoinIndex(hash [32]byte, stakedSupply uint64) uint64 {
	// modulo bias is negligible if we use uint128 math
	hashValue := uint128.FromBytes(hash[:16])
	return hashValue.Mod64(stakedSupply)
}
