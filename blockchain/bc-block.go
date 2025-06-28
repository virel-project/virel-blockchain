package blockchain

import (
	"virel-blockchain/adb"
	"virel-blockchain/binary"
	"virel-blockchain/block"
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
