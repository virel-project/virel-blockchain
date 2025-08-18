package address_test

import (
	"testing"

	"github.com/virel-project/virel-blockchain/address"
	"github.com/virel-project/virel-blockchain/bitcrypto"
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
}
