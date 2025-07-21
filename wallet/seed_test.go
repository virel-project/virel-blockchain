package wallet

import (
	"fmt"
	"testing"

	"github.com/virel-project/virel-blockchain/address"
	"github.com/virel-project/virel-blockchain/bitcrypto"
	"github.com/virel-project/virel-blockchain/util"
	"github.com/zeebo/blake3"
)

type testVector struct {
	Mnemonic string
	Privkey  []byte
	Address  string
}

var testVectors = []testVector{
	{
		Mnemonic: "warfare process youth leaf attend pluck census canvas follow remember fashion kiwi",
		Privkey:  util.AssertHexDec("3fedf0226ff456f7e975c727fdc981464ac433c2713279b5bdf6b7c9face5255a3a0ab9015a343b4af0bd70f0fd39ea4810299ed5877d46545adf5755dc8929b"),
		Address:  "s112j1u96oxmjwnsf6atm65vg43cha0r5tu3pmw",
	},
	{
		Mnemonic: "culture sorry knock eyebrow whip differ hurry narrow diary ring fork crowd slow other flower pilot sentence repeat",
		Privkey:  util.AssertHexDec("7422614d2f04b49839c49a4766547a5267c668616a7f8667ae51b6ca6951d49d6a39bd3a0fafc088dc7a46bcd4f50dc2ad6e50ed7025a7c652d64b45a3ac6d41"),
		Address:  "szn85qncpxr5ls8ii2oio44tn72o1exsamr5en",
	},
	{
		Mnemonic: "enjoy hotel surround sting frost churn game inform term type olympic memory inch ostrich banana lens calm maximum",
		Privkey:  util.AssertHexDec("4d738c2ea5714a241d4490efb18a6a8b700f87cbbc72af160128589f60efd87622cd857eaf02e8ee0b2d594829fc6f284f0dabe4d9a8831eda2bac466d28b3f2"),
		Address:  "s1gth43sxli9ubiotog750lgwkkixkgfs1htxss",
	},
}

func TestSeed(t *testing.T) {

	{
		h := blake3.Sum256([]byte("test"))
		entropy := h[:SEED_ENTROPY]

		mnemonic, privk := newMnemonic(entropy)

		// Display mnemonic and keys
		fmt.Printf("Mnemonic: %v\n", mnemonic)
		fmt.Printf("private key: %x\n", privk)
		addr := address.FromPubKey(privk.Public())
		fmt.Printf("address: %v\n", addr)

		if addr.String() != "s1evmew2s567o7yf9lx0ecq0i1pkmb351wax0ws" {
			t.Fatalf("incorrect address %v", addr)
		}
	}

	for _, v := range testVectors {
		privk, err := decodeMnemonic(v.Mnemonic)
		if err != nil {
			t.Fatal(err)
		}
		if privk != bitcrypto.Privkey(v.Privkey) {
			t.Fatalf("incorrect privk %x expected %x", privk, v.Privkey)
		}
	}

}
