package bitcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

type Cipher struct {
	gcm       cipher.AEAD
	noncesize int
}

// Uses AES-256 under the hood.
func NewCipher(key [32]byte) (Cipher, error) {
	cip, err := aes.NewCipher(key[:])
	if err != nil {
		return Cipher{}, err
	}

	aead, err := cipher.NewGCM(cip)
	if err != nil {
		return Cipher{}, err
	}

	return Cipher{
		gcm:       aead,
		noncesize: aead.NonceSize(),
	}, nil
}

func (c *Cipher) Encrypt(data []byte) ([]byte, error) {
	if data == nil {
		return nil, errors.New("Encrypt: data cannot be nil")
	}

	nonce := make([]byte, c.noncesize)
	_, err := rand.Read(nonce)
	if err != nil {
		return nil, err
	}

	if c.gcm == nil {
		return nil, errors.New("gcm is nil")
	}

	var sealed []byte
	sealed = c.gcm.Seal(sealed, nonce, data, []byte{})

	if len(sealed) < len(data) {
		return nonce, fmt.Errorf("sealed %x is shorter than data %x", sealed, data)
	}

	return append(nonce, sealed...), nil
}

func (c *Cipher) Decrypt(data []byte) ([]byte, error) {
	nonceSize := c.gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("encrypted data is too short")
	}
	nonce, data := data[:nonceSize], data[nonceSize:]

	msg, err := c.gcm.Open(nil, nonce, data, nil)
	return msg, err
}
