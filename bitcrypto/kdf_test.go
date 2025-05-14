package bitcrypto

import (
	"testing"
	"time"
)

func BenchmarkKDF(b *testing.B) {
	t0 := time.Now()

	for i := 0; i < b.N; i++ {
		KDF([]byte("test"), []byte("somesalt"), 4, 4)
	}

	t := time.Since(t0)

	b.Log("KDF:", b.N, "hashes computed in", t)
	s := float64(t.Nanoseconds()) / float64(time.Second.Nanoseconds())

	b.Log("KDF: H/s:", float64(b.N)/s)
}
