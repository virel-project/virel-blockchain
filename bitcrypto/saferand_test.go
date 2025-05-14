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

/*
// test the randomness using: dieharder -f random.bin -a -Y 1
// This test passes the full DieHard test suite, except sometimes diehard_sums (which is considered broken by
// the DieHarder author)
func TestWriteToFile(t *testing.T) {
	os.Remove("random.bin")

	f, err := os.OpenFile("random.bin", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}

	defer f.Close()

	b := make([]byte, 1024)

	for i := 0; i < 1024; i++ {
		RandRead(b)

		if _, err = f.Write(b); err != nil {
			panic(err)
		}

	}

}
*/
