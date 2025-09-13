package chaintype

import (
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/virel-project/virel-blockchain/v3/address"
	"github.com/virel-project/virel-blockchain/v3/binary"
	"github.com/virel-project/virel-blockchain/v3/bitcrypto"
)

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
		s.AddUvarint(v.Unlock)
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
			Unlock: d.ReadUvarint(),
		}
	}
	return d.Error()
}

func (g *Delegate) String() string {
	s := fmt.Sprintf("Delegate id: %d", g.Id) + "\n" +
		fmt.Sprintf("Name: %s", strconv.Quote(string(g.Name))) + "\n" +
		fmt.Sprintf("Owner: %s", address.FromPubKey(g.Owner)) + "\n" +
		"Funds:\n"

	for _, v := range g.Funds {
		s += fmt.Sprintf(" - Owner %s Amount %d Unlock %d", v.Owner, v.Amount, v.Unlock)
	}
	return s
}

type DelegatedFund struct {
	Owner  address.Address `json:"owner"`
	Amount uint64          `json:"amount"`
	Unlock uint64          `json:"unlock"` // height of unlock of this fund
}
