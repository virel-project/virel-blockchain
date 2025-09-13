package block

import (
	"fmt"
	"slices"

	"github.com/virel-project/virel-blockchain/v3/binary"
	"github.com/virel-project/virel-blockchain/v3/bitcrypto"
	"github.com/virel-project/virel-blockchain/v3/config"
	"github.com/virel-project/virel-blockchain/v3/util"

	"github.com/zeebo/blake3"
)

// BaseHash() returns the base block hash, used for pruning unnecessary data from commitments.
// It essentially contains everything except nonce, nonce extra & timestamp, since it has to be used as part
// of the HashingID, which in turn is part of the MiningBlob. It also makes sure not to include OtherChains.
func (b Block) BaseHash() util.Hash {
	b.Timestamp = 0
	b.Nonce = 0
	b.NonceExtra = [16]byte{}
	b.StakeSignature = bitcrypto.Signature{}
	b.NextDelegateId = 0
	if len(b.OtherChains) != 0 {
		b.OtherChains = make([]HashingID, 0)
	}
	return b.Hash()
}

// A commitment is a proof of work that is verified using a block's ancestors, timestamp, nonces, chains and
// base hash.
func (b Block) Commitment() Commitment {
	return Commitment{
		BaseHash:    b.BaseHash(),
		Ancestors:   b.Ancestors,
		Timestamp:   b.Timestamp,
		Nonce:       b.Nonce,
		NonceExtra:  b.NonceExtra,
		OtherChains: b.OtherChains,
	}
}

// HashingID() returns the hash that must be found inside MiningBlob.
// HashingID is comprised of BaseHash + previous hash
func (c Commitment) HashingID() HashingID {
	data := make([]byte, 32+(32*config.MINIDAG_ANCESTORS))
	basehash := c.BaseHash
	copy(data[0:32], basehash[:])
	for i, v := range c.Ancestors {
		copy(data[32+i*32:], v[:])
	}

	return HashingID{
		NetworkID: config.NETWORK_ID,
		Hash:      blake3.Sum256(data),
	}
}

// Commitment represents a PoW commitment with Merge-Mining and MiniDAG for the current chain
type Commitment struct {
	BaseHash util.Hash

	// Block ancestors for MiniDAG
	Ancestors [config.MINIDAG_ANCESTORS]util.Hash

	// MiningBlob data
	Timestamp   uint64
	Nonce       uint32
	NonceExtra  [16]byte
	OtherChains []HashingID
}

func (a Commitment) Equals(b Commitment) bool {
	return a.BaseHash == b.BaseHash && a.Timestamp == b.Timestamp && a.Nonce == b.Nonce && a.NonceExtra == b.NonceExtra
}

func (c Commitment) Serialize() []byte {
	s := binary.NewSer(make([]byte, 50))
	s.AddFixedByteArray(c.BaseHash[:])

	for _, v := range c.Ancestors {
		s.AddFixedByteArray(v[:])
	}
	s.AddUvarint(c.Timestamp)
	s.AddUint32(c.Nonce)
	s.AddFixedByteArray(c.NonceExtra[:])

	s.AddUvarint(uint64(len(c.OtherChains)))
	for _, v := range c.OtherChains {
		s.AddUint64(v.NetworkID)
		s.AddFixedByteArray(v.Hash[:])
	}

	return s.Output()
}

func (c *Commitment) Deserialize(d *binary.Des) error {
	c.BaseHash = util.Hash(d.ReadFixedByteArray(32))

	for i := range c.Ancestors {
		c.Ancestors[i] = util.Hash(d.ReadFixedByteArray(32))
	}
	c.Timestamp = d.ReadUvarint()
	c.Nonce = d.ReadUint32()
	c.NonceExtra = [16]byte(d.ReadFixedByteArray(16))

	numChains := int(d.ReadUvarint())
	if numChains < 0 || numChains > config.MAX_MERGE_MINED_CHAINS-1 {
		return fmt.Errorf("commitment numChains exceed limit: %d", numChains)
	}
	c.OtherChains = make([]HashingID, numChains)
	for i := range c.OtherChains {
		if d.Error() != nil {
			return d.Error()
		}
		c.OtherChains[i] = HashingID{
			NetworkID: d.ReadUint64(),
			Hash:      [32]byte(d.ReadFixedByteArray(32)),
		}
	}

	return d.Error()
}

func (c Commitment) MiningBlob() MiningBlob {
	hashingid := c.HashingID()

	// sort OtherChains, since an incorrect ordering gives incorrect results during hashing
	chains := append(c.OtherChains, hashingid)
	slices.SortFunc(chains, func(a, b HashingID) int {
		if a.NetworkID < b.NetworkID {
			return -1
		} else if a.NetworkID > b.NetworkID {
			return 1
		}
		panic("possible duplicate value in OtherChains")
	})

	return MiningBlob{
		Timestamp:  c.Timestamp,
		Nonce:      c.Nonce,
		NonceExtra: c.NonceExtra,
		Chains:     chains,
	}

}
