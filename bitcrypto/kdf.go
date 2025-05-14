package bitcrypto

import (
	"golang.org/x/crypto/argon2"
)

// time is 4 by default, mem is 4 by default (4 MiB)
func KDF(pass, salt []byte, time, mem uint32) [32]byte {
	return [32]byte(argon2.IDKey(pass, salt, time, mem, 1, 32))
}
