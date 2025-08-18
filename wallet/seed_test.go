package wallet

import (
	"fmt"
	"testing"

	"github.com/virel-project/virel-blockchain/address"
	"github.com/virel-project/virel-blockchain/bitcrypto"
	"github.com/virel-project/virel-blockchain/config"
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
		Privkey:  util.AssertHexDec("1d55a334b7752aec8d69fa921278f06f75daf7e282c6c26ab59d29cabb1ebcd4544ffcd2d717070543503e0be7d464e9d980b750fc2e64019221fc78aab943eb"),
		Address:  config.WALLET_PREFIX + "q5yxhxyl1vubysjoiyo27ejwiqwwsn90il738",
	},
	{
		Mnemonic: "culture sorry knock eyebrow whip differ hurry narrow diary ring fork crowd slow other flower pilot sentence repeat",
		Privkey:  util.AssertHexDec("4e770191020963283b4cc6194fa5229921c947d2243d76ee0c1642a6003d980f859e23e1de615ea927b3bb72b342247d7db2aec697a2ac655a05434891bb0f1e"),
		Address:  config.WALLET_PREFIX + "564swxrqb24ize4mfis44a0pnhzb44jsrjrpe",
	},
	{
		Mnemonic: "enjoy hotel surround sting frost churn game inform term type olympic memory inch ostrich banana lens calm maximum",
		Privkey:  util.AssertHexDec("eb5ec44da5a9bac2f2c9d4b2173adcdf753d9a05416876167f35cad944357d1314c15304f7b85aeeb8e03eba05253840e5ccd60906e945a3435ffee1d636f0e3"),
		Address:  config.WALLET_PREFIX + "1mhz2r9jjuzfugeiesmbvibsc1jc3iuufqkits",
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

		if addr.String() != "vvpw9fao00h5nhvl3wl1bwxe3mshbbqf5sidhe" {
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

		addr := address.FromPubKey(privk.Public())

		if addr.String() != v.Address {
			t.Fatalf("incorrect address %s expected %s", addr, v.Address)
		}

	}

}
