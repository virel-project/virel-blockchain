package block

import (
	crand "crypto/rand"
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/transaction"

	"github.com/virel-project/virel-blockchain/v2/util/uint128"

	"github.com/zeebo/blake3"
)

var sampleBlock = Block{
	BlockHeader: BlockHeader{
		Version:    0,
		Timestamp:  6975033,
		Nonce:      7083,
		NonceExtra: [16]byte{},
		Recipient:  address.Address{},
		Ancestors:  Ancestors{},
	},
	Difficulty:     uint128.From64(10_000),
	CumulativeDiff: uint128.From64(2),
	Transactions:   []transaction.TXID{},
}

func TestBlock(t *testing.T) {
	bl := sampleBlock
	bl2 := Block{}

	ser := bl.Serialize()

	t.Logf("ser: %x", ser)

	err := bl2.Deserialize(ser)

	if err != nil {
		t.Fatal(err)
	}

	t.Logf("bl: %s\nbl2: %s", bl.String(), bl2.String())

	if bl2.Hash() != bl.Hash() {
		t.Fatal("the two blocks are not equal")
	}
}

func TestSetMiningBlob(t *testing.T) {
	bl := sampleBlock

	nextra := [16]byte{}
	_, err := crand.Read(nextra[:])
	if err != nil {
		t.Fatal(err)
	}

	mb := MiningBlob{
		Timestamp:  rand.Uint64(),
		Nonce:      rand.Uint32(),
		NonceExtra: nextra,
		Chains: []HashingID{
			{
				NetworkID: config.NETWORK_ID + 1,
				Hash:      blake3.Sum256(nextra[:]),
			},
		},
	}
	mb.Chains = append([]HashingID{bl.Commitment().HashingID()}, mb.Chains...)

	bl.setMiningBlob(mb)

	mb2 := bl.Commitment().MiningBlob()

	if !reflect.DeepEqual(mb2, mb) {
		t.Fatal(bl.Commitment().MiningBlob(), "doesn't match with", mb)
	}
}

func TestSerialize(t *testing.T) {
	bl := sampleBlock
	bl2 := &Block{}
	if err := bl2.Deserialize(bl.Serialize()); err != nil {
		t.Fatal(err)
	}
	if bl.Hash() != bl2.Hash() {
		t.Fatal("bl and bl2 don't match")
	}
}
func BenchmarkSerialization(b *testing.B) {
	bl := sampleBlock

	for i := 0; i < b.N; i++ {
		bl.Serialize()
	}
}
func BenchmarkDeserialization(b *testing.B) {
	bl := sampleBlock
	blser := bl.Serialize()

	for i := 0; i < b.N; i++ {
		bl.Deserialize(blser)
	}
}
