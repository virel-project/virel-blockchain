package address_test

import (
	"testing"
	"virel-blockchain/address"

	"github.com/zeebo/blake3"
)

func TestAddress(t *testing.T) {
	pk := address.GenerateKeypair(blake3.Sum256([]byte("example seed")))

	x := address.FromPubKey(pk.Public()).Integrated()

	str := x.String()
	t.Log(str)

	if str != "s1hbvnh60rkffrlxrrvaaatb2epi146o2gvjnox" {
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
