package wallet

import (
	"fmt"

	"github.com/virel-project/virel-blockchain/v2/bitcrypto"
	"github.com/virel-project/virel-blockchain/v2/bitcrypto/slip10"
	"github.com/virel-project/virel-blockchain/v2/config"

	"github.com/tyler-smith/go-bip39"
)

// seed entropy in bytes
const SEED_ENTROPY = 24

// generates a seed and returns the seed and the associated keypair
func newMnemonic(entropy []byte) (string, bitcrypto.Privkey) {
	seed, err := bip39.NewMnemonic(entropy)
	if err != nil {
		panic(err)
	}

	node, err := slip10.DeriveForPath(fmt.Sprintf("m/44'/%d'/0'/0'/0'", config.HD_COIN_TYPE), entropy)
	if err != nil {
		panic(err)
	}
	_, privk := node.Keypair()

	return seed, bitcrypto.Privkey(privk)
}

// decodes a mnemonic seedphrase into a private key
func decodeMnemonic(seed string) (bitcrypto.Privkey, error) {
	entropy, err := bip39.EntropyFromMnemonic(seed)
	if err != nil {
		return bitcrypto.Privkey{}, err
	}

	node, err := slip10.DeriveForPath(fmt.Sprintf("m/44'/%d'/0'/0'/0'", config.HD_COIN_TYPE), entropy)
	if err != nil {
		panic(err)
	}

	_, privk := node.Keypair()

	return bitcrypto.Privkey(privk), nil
}
