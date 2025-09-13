package block

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/virel-project/virel-blockchain/v3/config"

	"github.com/zeebo/blake3"
)

func TestHashingBlob(t *testing.T) {
	mb := MiningBlob{
		Timestamp: uint64(time.Now().Unix()),
		Nonce:     0x37133713,
		Chains: []HashingID{
			{
				NetworkID: config.NETWORK_ID,
				Hash:      blake3.Sum256([]byte("test")),
			},
		},
	}
	rand.Read(mb.NonceExtra[:])

	ms := mb.Serialize()

	mb2 := MiningBlob{}

	if err := mb2.Deserialize(ms); err != nil {
		t.Fatal(err)
	}
}
