package bitcrypto

import (
	"crypto/rand"
)

func RandRead(b []byte) {
	_,err := rand.Read(b)
	if err!=nil {
		panic(err)
	}
}