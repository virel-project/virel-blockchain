package blockchain

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/virel-project/virel-blockchain/v3/address"
	"github.com/virel-project/virel-blockchain/v3/transaction"
	"github.com/virel-project/virel-blockchain/v3/util"
	"github.com/virel-project/virel-blockchain/v3/util/uint128"
)

type Stats struct {
	TopHash        util.Hash
	TopHeight      uint64
	CumulativeDiff uint128.Uint128
	Tips           map[util.Hash]*AltchainTip
	Orphans        map[util.Hash]*Orphan // hash -> orphan
	StakedAmount   uint64
}

func (s *Stats) String() string {
	return fmt.Sprintf("top hash: %s height: %d cumulativediff %s staked amount %s", s.TopHash, s.TopHeight, s.CumulativeDiff, util.FormatCoin(s.StakedAmount))
}

type AltchainTip struct {
	Hash           util.Hash
	Height         uint64
	CumulativeDiff uint128.Uint128
}
type Orphan struct {
	Hash     util.Hash
	PrevHash util.Hash
}

func (s *Stats) Serialize() []byte {
	var network bytes.Buffer        // Stand-in for a network connection
	enc := gob.NewEncoder(&network) // Will write to network.

	err := enc.Encode(s)
	if err != nil {
		Log.Fatal(err)
	}
	d, err := io.ReadAll(&network)
	if err != nil {
		Log.Fatal(err)
	}
	return d
}

func DeserializeStats(d []byte) (*Stats, error) {
	var network bytes.Buffer
	dec := gob.NewDecoder(&network)

	_, err := network.Write(d)
	if err != nil {
		return nil, err
	}

	s := Stats{}

	err = dec.Decode(&s)
	if err != nil {
		return nil, err
	}

	if s.Orphans == nil {
		s.Orphans = make(map[util.Hash]*Orphan)
	}
	if s.Tips == nil {
		s.Tips = make(map[util.Hash]*AltchainTip)
	}

	return &s, err
}

func (s *Stats) Staked(amount uint64) error {
	if s.StakedAmount+amount < s.StakedAmount {
		return errors.New("add overflow")
	}
	s.StakedAmount += amount
	return nil
}
func (s *Stats) Unstaked(amount uint64) error {
	if s.StakedAmount-amount > s.StakedAmount {
		return errors.New("sub overflow")
	}
	s.StakedAmount -= amount
	return nil
}

type Mempool struct {
	Entries []*MempoolEntry
}
type MempoolEntry struct {
	TXID      [32]byte
	TxVersion uint8
	Size      uint64
	Fee       uint64
	Expires   int64
	Signer    address.Address
	Inputs    []transaction.StateInput
	Outputs   []transaction.Output
}

func (s *Mempool) Serialize() []byte {
	var network bytes.Buffer        // Stand-in for a network connection
	enc := gob.NewEncoder(&network) // Will write to network.

	// Remove expired entries from the mempool
	t := time.Now().Unix()
	i := 0
	for _, v := range s.Entries {
		if v.Expires >= t { // keep non-expired entries
			s.Entries[i] = v
			i++
		} else {
			Log.Debugf("removing expired mempool transaction %x", v.TXID)
		}
	}
	s.Entries = s.Entries[:i]

	err := enc.Encode(s)
	if err != nil {
		Log.Fatal(err)
	}
	d, err := io.ReadAll(&network)
	if err != nil {
		Log.Fatal(err)
	}
	return d
}

func DeserializeMempool(d []byte) (*Mempool, error) {
	var network bytes.Buffer
	dec := gob.NewDecoder(&network)

	_, err := network.Write(d)
	if err != nil {
		return nil, err
	}

	s := Mempool{}

	err = dec.Decode(&s)
	if err != nil {
		return nil, err
	}

	if s.Entries == nil {
		s.Entries = make([]*MempoolEntry, 0)
	}

	return &s, err
}

func (m *Mempool) GetEntry(hash [32]byte) *MempoolEntry {
	for _, v := range m.Entries {
		if v.TXID == hash {
			return v
		}
	}
	return nil
}

func (m *Mempool) DeleteEntry(hash [32]byte) {
	for i, v := range m.Entries {
		if v.TXID == hash {
			Log.Debugf("Removing transaction %x from mempool", v.TXID)
			m.Entries = append(m.Entries[:i], m.Entries[i+1:]...)
			return
		}
	}
}
