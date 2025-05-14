package block

import (
	"testing"

	"github.com/zeebo/blake3"
)

func TestAncestors(t *testing.T) {
	t.Log("Test")

	// a is more recent
	a := Ancestors{}
	a = a.AddHash(blake3.Sum256([]byte("4")))
	a = a.AddHash(blake3.Sum256([]byte("3")))
	t.Log(a)

	// b is older
	b := Ancestors{}
	b = b.AddHash(blake3.Sum256([]byte("3")))
	b = b.AddHash(blake3.Sum256([]byte("2")))
	t.Log(b)

	if a.FindCommon(b) != 1 {
		t.Fatal("common is", a.FindCommon(b), "expected 1")
	}

}
