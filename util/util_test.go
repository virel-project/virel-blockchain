package util

import (
	crand "crypto/rand"
	"encoding/hex"
	"math/rand/v2"
	"testing"

	"github.com/virel-project/virel-blockchain/util/uint128"
)

func TestTarget(t *testing.T) {
	for i := 0; i < 100; i++ {
		var diff uint64 = rand.Uint64() / 0xff
		target := GetTarget(uint128.From64(diff))
		diff2 := TargetToDiff64(target)

		if diff > diff2.Lo*201/200 || diff < diff2.Lo*199/200 {
			t.Fatal("diff", diff, "and diff2", diff2, "do not match")
		}
	}
	if GetTarget(uint128.From64(45293397162912617)) != 407 {
		t.Fatal("expected target 407")
	}
}

func TestIsHex(t *testing.T) {
	b := make([]byte, 64)
	crand.Read(b)
	if !IsHex(hex.EncodeToString(b)) {
		t.Fail()
	}
	x := "123456789/"
	if IsHex(x) {
		t.Fail()
	}
	x = "123456789="
	if IsHex(x) {
		t.Fail()
	}
	x = "123456789A"
	if IsHex(x) {
		t.Fail()
	}
	x = "123456789~"
	if IsHex(x) {
		t.Fail()
	}
}
