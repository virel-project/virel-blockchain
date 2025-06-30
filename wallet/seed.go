package wallet

import (
	"github.com/virel-project/virel-blockchain/address"
	"github.com/virel-project/virel-blockchain/bitcrypto"

	"github.com/tyler-smith/go-bip39"
	"github.com/zeebo/blake3"
)

// seed entropy in bytes
const SEED_ENTROPY = 16

// generates a seed and returns the seed and the associated keypair
func newMnemonic() (string, bitcrypto.Privkey) {
	entropy := make([]byte, SEED_ENTROPY)
	bitcrypto.RandRead(entropy)

	seed, err := bip39.NewMnemonic(entropy)
	if err != nil {
		panic(err)
	}

	key := address.GenerateKeypair(blake3.Sum256(entropy[:]))

	return seed, key
}

// decodes a mnemonic seedphrase into a private key
func decodeMnemonic(seed string) (bitcrypto.Privkey, error) {
	entropy, err := bip39.EntropyFromMnemonic(seed)
	if err != nil {
		return bitcrypto.Privkey{}, err
	}

	key := address.GenerateKeypair(blake3.Sum256(entropy[:]))
	return key, nil
}
