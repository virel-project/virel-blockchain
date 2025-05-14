package wallet

import (
	"fmt"
	"testing"
	"virel-blockchain/bitcrypto"

	bip39 "github.com/tyler-smith/go-bip39"
)

func TestSeed(t *testing.T) {
	// Generate a mnemonic for memorization or user-friendly seeds
	entropy := make([]byte, SEED_ENTROPY)
	bitcrypto.RandRead(entropy)

	mnemonic, _ := bip39.NewMnemonic(entropy)

	// Generate a Bip32 HD wallet for the mnemonic and a user supplied password
	seed := bip39.NewSeed(mnemonic, "Secret Passphrase")

	// Display mnemonic and keys
	fmt.Println("Mnemonic: ", mnemonic)
	fmt.Println("Seed: ", seed)
}
