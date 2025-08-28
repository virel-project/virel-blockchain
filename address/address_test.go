package address_test

import (
	"crypto/rand"
	"testing"

	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/bitcrypto"
	"github.com/zeebo/blake3"
)

func TestAddress(t *testing.T) {
	pk := bitcrypto.Pubkey(blake3.Sum256([]byte("test")))

	x := address.FromPubKey(pk).Integrated()

	str := x.String()
	t.Log(str)

	if str != "v7xxb13ng6gzd3tugb8nqzx28btuq0ihkhqxm6" {
		t.Error("address is not valid")
	}

	x2, err := address.FromString(str)
	if err != nil {
		t.Error(err)
	}

	str2 := x2.String()
	t.Log(str2)

	if str != str2 {
		t.Error("address does not match")
	}

	a := "vywgto01ztd27qt1kqc1bl8qps3x41uut1n2mosgg"

	b, err := address.FromString(a)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%x", b.Addr[2:])
	t.Log("payment id", b.PaymentId)
	if b.String() != a {
		t.Fatalf("addresses do not match %v %v", a, b)
	}
}

func TestAddress2(t *testing.T) {
	r := make([]byte, address.SIZE)
	rand.Read(r)
	a := address.Integrated{
		Addr:      address.Address(r),
		PaymentId: 124,
	}
	t.Log(a.String())
	b, err := address.FromString(a.String())
	if err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() {
		t.Fatalf("address %v and %v not matching", a, b)
	}
}
