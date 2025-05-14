package bitcrypto

import (
	"crypto"
	"crypto/ed25519"
	"fmt"
)

const SIGNATURE_SIZE = 64
const PUBKEY_SIZE = 32
const PRIVKEY_SIZE = 32 + PUBKEY_SIZE

type Pubkey [PUBKEY_SIZE]byte
type Privkey [PRIVKEY_SIZE]byte
type Signature [SIGNATURE_SIZE]byte

func (p Privkey) Public() Pubkey {
	return Pubkey(p[32:])
}

func Sign(message []byte, key Privkey) (Signature, error) {
	edk := ed25519.PrivateKey(key[:])

	x, err := edk.Sign(nil, message, crypto.Hash(0))

	if err != nil {
		return Signature{}, err
	}

	if len(x) != SIGNATURE_SIZE {
		panic(fmt.Errorf("signature size: %d, expected: %d", x, SIGNATURE_SIZE))
	}

	return Signature(x), err
}

// returns true if the signature is valid
func VerifySignature(sender Pubkey, data []byte, signature Signature) bool {
	return ed25519.Verify(ed25519.PublicKey(sender[:]), data, signature[:])
}
