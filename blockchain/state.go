package blockchain

import (
	"fmt"

	"github.com/virel-project/virel-blockchain/binary"
)

type State struct {
	Balance      uint64
	LastNonce    uint64
	LastIncoming uint64 // not used in consensus, but we store it to list the wallet incoming transactions
}

func (x State) Serialize() []byte {
	s := binary.NewSer(make([]byte, 4))

	s.AddUvarint(x.Balance)
	s.AddUvarint(x.LastNonce)
	s.AddUvarint(x.LastIncoming)

	return s.Output()
}

func (x *State) Deserialize(d []byte) error {
	s := binary.NewDes(d)

	x.Balance = s.ReadUvarint()
	x.LastNonce = s.ReadUvarint()
	x.LastIncoming = s.ReadUvarint()

	return s.Error()
}

func (x State) String() string {
	return fmt.Sprintf("Balance: %d; LastNonce: %d; LastIncoming: %d", x.Balance, x.LastNonce, x.LastIncoming)
}
