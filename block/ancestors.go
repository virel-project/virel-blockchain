package block

import (
	"encoding/hex"

	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/util"
)

type Ancestors [config.MINIDAG_ANCESTORS]util.Hash

func (a Ancestors) String() string {
	s := "["
	for i, v := range a {
		s += hex.EncodeToString(v[:])
		if i != len(a)-1 {
			s += ", "
		}
	}

	return s + "]"
}

func (a Ancestors) AddHash(h util.Hash) Ancestors {
	o := Ancestors{
		h,
	}
	for i := 1; i < len(a); i++ {
		o[i] = a[i-1]
	}
	return o
}

// returns the offset (>= 0); -1 if common isn't found.
// a is a list of hashes and b is an older list of hashes
func (a Ancestors) FindCommon(b Ancestors) int {
	for _, hashA := range a {
		for idxB, hashB := range b {
			if hashA == hashB {
				return idxB
			}
		}
	}
	return -1
}
