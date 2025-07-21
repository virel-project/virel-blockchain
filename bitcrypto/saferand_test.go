package bitcrypto

import (
	"fmt"
	"testing"
	"time"
)

func TestSaferand(t *testing.T) {
	b := make([]byte, 1024)

	t0 := time.Now()
	RandRead(b[:])

	fmt.Println("total elapsed:", time.Since(t0))
}
