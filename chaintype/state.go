package chaintype

import (
	"errors"
	"fmt"

	"github.com/virel-project/virel-blockchain/v3/binary"
)

type State struct {
	Balance      uint64
	LastNonce    uint64
	LastIncoming uint64 // not used in consensus, but we store it to list the wallet incoming transactions
	DelegateId   uint64
}

func (x *State) Serialize() []byte {
	s := binary.NewSer(make([]byte, 4))

	s.AddUvarint(x.Balance)
	s.AddUvarint(x.LastNonce)
	s.AddUvarint(x.LastIncoming)
	s.AddUint8(1) // version
	s.AddUvarint(x.DelegateId)

	return s.Output()
}

func (x *State) Deserialize(d []byte) error {
	s := binary.NewDes(d)

	x.Balance = s.ReadUvarint()
	x.LastNonce = s.ReadUvarint()
	x.LastIncoming = s.ReadUvarint()

	if len(s.RemainingData()) > 0 {
		v := s.ReadUint8()
		if v != 1 {
			return errors.New("invalid state version")
		}
		x.DelegateId = s.ReadUvarint()
		return nil
	}

	return s.Error()
}

func (x *State) String() string {
	return fmt.Sprintf("Balance: %d; LastNonce: %d; LastIncoming: %d; DelegateId: %d", x.Balance, x.LastNonce, x.LastIncoming, x.DelegateId)
}
