package blockchain

import (
	"errors"
	"fmt"
	"slices"

	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/binary"
	"github.com/virel-project/virel-blockchain/v2/bitcrypto"
)

type State struct {
	Balance        uint64
	LastNonce      uint64
	LastIncoming   uint64 // not used in consensus, but we store it to list the wallet incoming transactions
	DelegateId     uint64
	DelegateAmount uint64 // not used in consensus, only for display
}

func (x State) Serialize() []byte {
	s := binary.NewSer(make([]byte, 4))

	s.AddUvarint(x.Balance)
	s.AddUvarint(x.LastNonce)
	s.AddUvarint(x.LastIncoming)
	s.AddUint8(1) // version
	s.AddUvarint(x.DelegateId)
	s.AddUvarint(x.DelegateAmount)

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
		x.DelegateAmount = s.ReadUvarint()
	}

	return s.Error()
}

func (x State) String() string {
	return fmt.Sprintf("Balance: %d; LastNonce: %d; LastIncoming: %d", x.Balance, x.LastNonce, x.LastIncoming)
}

type Delegate struct {
	Id    uint64 `json:"id"` // delegate identifier, starting from 1
	Owner bitcrypto.Pubkey
	Name  []byte

	Funds []*DelegatedFund
}

func (d *Delegate) TotalAmount() (t uint64) {
	oldt := t
	for _, v := range d.Funds {
		t += v.Amount
		if oldt > t {
			panic("Delegate TotalAmount overflow")
		}
		oldt = t
	}
	return
}

func (d *Delegate) SortFunds() (err error) {
	var seen = make(map[address.Address]bool, len(d.Funds))

	for _, v := range d.Funds {
		if seen[v.Owner] {
			return fmt.Errorf("fund with owner %s is duplicate", v.Owner)
		}
		seen[v.Owner] = true
	}

	slices.SortStableFunc(d.Funds, func(a, b *DelegatedFund) int {
		return slices.Compare(a.Owner[:], b.Owner[:])
	})

	return nil
}

func (d *Delegate) OwnerAddress() address.Address {
	return address.FromPubKey(d.Owner)
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
	s.AddByteSlice(g.Name)

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
	g.Owner = bitcrypto.Pubkey(d.ReadFixedByteArray(bitcrypto.PUBKEY_SIZE))
	g.Name = d.ReadByteSlice()

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
