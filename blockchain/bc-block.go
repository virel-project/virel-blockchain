package blockchain

import (
	"errors"
	"fmt"

	"github.com/virel-project/go-randomvirel"
	"github.com/virel-project/virel-blockchain/adb"
	"github.com/virel-project/virel-blockchain/binary"
	"github.com/virel-project/virel-blockchain/block"
	"github.com/virel-project/virel-blockchain/checkpoints"
	"github.com/virel-project/virel-blockchain/config"
	"github.com/virel-project/virel-blockchain/transaction"
	"github.com/virel-project/virel-blockchain/util"
)

func (bc *Blockchain) SerializeFullBlock(txn adb.Txn, b *block.Block) ([]byte, error) {
	s := binary.NewSer(make([]byte, 0, 80))

	s.AddFixedByteArray(b.BlockHeader.Serialize())

	// difficulty is encoded as a little-endian byte slice, with leading zero bytes removed
	diff := make([]byte, 16)
	b.Difficulty.PutBytes(diff)
	for diff[len(diff)-1] == 0 {
		diff = diff[:len(diff)-1]
	}
	s.AddByteSlice(diff)
	// cumulative diff is encoded the same way as difficulty
	diff = make([]byte, 16)
	b.CumulativeDiff.PutBytes(diff)
	for diff[len(diff)-1] == 0 {
		diff = diff[:len(diff)-1]
	}
	s.AddByteSlice(diff)

	s.AddUvarint(uint64(len(b.Transactions)))

	for _, v := range b.Transactions {
		txn, _, err := bc.GetTx(txn, v)
		if err != nil {
			return nil, err
		}
		s.AddByteSlice(txn.Serialize())
	}

	return s.Output(), nil
}

// Prevalidate contains basic validity check, such as PoW hash and timestamp not in future
func (bc *Blockchain) PrevalidateBlock(b *block.Block, txs []*transaction.Transaction) error {
	// Generally, try insering the least expensive checks first, most expensive last

	if b.Version != 0 {
		return fmt.Errorf("unexpected block version %d", b.Version)
	}

	if b.Difficulty.IsZero() {
		return errors.New("difficulty is zero")
	}

	if b.Difficulty.Cmp64(config.MIN_DIFFICULTY) < 0 {
		return errors.New("difficulty is less than minimum")
	}

	if b.Timestamp > util.Time()+config.FUTURE_TIME_LIMIT*1000 {
		return errors.New("block is too much in the future")
	}

	// check that OtherChains are valid (no duplicates)
	for i, v := range b.OtherChains {
		if v.NetworkID == config.NETWORK_ID {
			return fmt.Errorf("other chain %x includes current network id", v.Hash)
		}
		for i2, v2 := range b.OtherChains {
			if i != i2 && (v.Hash == v2.Hash || v.NetworkID == v2.NetworkID) {
				return fmt.Errorf("duplicate OtherChain: %x %d; %x %d", v.Hash, v.NetworkID,
					v2.Hash, v2.NetworkID)
			}
		}
	}

	for _, tx := range txs {
		err := tx.Prevalidate()
		if err != nil {
			return fmt.Errorf("invalid transaction: %w", err)
		}
	}

	if b.Height != 440 {
		for i, v := range b.SideBlocks {
			if v.Ancestors == b.Ancestors {
				return fmt.Errorf("side block has the same height as current block")
			}
			for i2, v2 := range b.SideBlocks {
				if (v.BaseHash == v2.BaseHash &&
					v.Nonce == v2.Nonce &&
					v.NonceExtra == v2.NonceExtra) && i != i2 {
					return fmt.Errorf("duplicate side block with base hash %s at height %d", v.BaseHash, b.Height)
				}
			}
		}
	}

	if !checkpoints.IsSecured(b.Height) {
		commitment := b.Commitment()
		mb := commitment.MiningBlob()
		seed := mb.GetSeed()
		powhash := randomvirel.PowHash(mb.GetSeed(), mb.Serialize())
		if !block.ValidPowHash32(powhash, b.Difficulty) {
			return fmt.Errorf("block %d %x with PoW %x does not meet difficulty", b.Height, b.Hash(), powhash)
		}

		// prevalidate side blocks
		// TODO: verify this is ok
		for _, side := range b.SideBlocks {
			/*if side.Height < b.Height-config.MINIDAG_ANCESTORS || side.Height >= b.Height {
				return fmt.Errorf("side block has invalid height %d, current block has height %d", side.Height, b.Height)
			}*/
			if block.GetSeedhashId(side.Timestamp) != block.GetSeedhashId(b.Timestamp) {
				return fmt.Errorf("side block has a different seedhash")
			}
			// verify that side block's difficulty is at least 2/3 of current block difficulty
			if !block.ValidPowHash32(randomvirel.PowHash(seed, side.MiningBlob().Serialize()), b.Difficulty.Mul64(2).Div64(3)) {
				return fmt.Errorf("commitment does not meet difficulty")
			}
		}
	} else {
		if checkpoints.IsCheckpoint(b.Height) {
			expectedHash := checkpoints.GetCheckpoint(b.Height)
			h := b.Hash()
			if h != expectedHash {
				return fmt.Errorf("block %x does not match checkpoint %x", h, expectedHash)
			}
		}
	}

	return nil
}
