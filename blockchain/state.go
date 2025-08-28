package blockchain

import (
	"errors"
	"fmt"

	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/binary"
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

type Delegate struct {
	Id    uint64 // delegate identifier, starting from 1
	Owner address.Address

	Funds []*DelegatedFund
}

type DelegatedFund struct {
	Owner  address.Address
	Amount uint64
}

func (g *Delegate) Serialize() []byte {
	s := binary.NewSer(make([]byte, 29+24*len(g.Funds)))

	// version of delegate struct data
	s.AddUint8(0)

	s.AddUvarint(g.Id)
	s.AddFixedByteArray(g.Owner[:])

	s.AddUvarint(uint64(len(g.Funds)))
	for _, v := range g.Funds {
		s.AddFixedByteArray(v.Owner[:])
		s.AddUvarint(v.Amount)
	}

	return s.Output()
}

func (g *Delegate) Deserialize(b []byte) error {
	d := binary.NewDes(b)

	if d.ReadUint8() != 0 {
		return errors.New("invalid delegate blob version")
	}

	g.Id = d.ReadUvarint()
	g.Owner = address.Address(d.ReadFixedByteArray(address.SIZE))

	numFunds := d.ReadUvarint()
	if numFunds > uint64(len(d.RemainingData())/20) {
		return fmt.Errorf("too many funds %d", numFunds)
	}

	g.Funds = make([]*DelegatedFund, numFunds)
	for i := 0; i < len(g.Funds); i++ {
		g.Funds[i] = &DelegatedFund{
			Owner:  address.Address(d.ReadFixedByteArray(address.SIZE)),
			Amount: d.ReadUvarint(),
		}
	}
	return d.Error()
}
