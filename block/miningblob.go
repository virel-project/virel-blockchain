package block

import (
	"fmt"

	"github.com/virel-project/virel-blockchain/binary"
	"github.com/virel-project/virel-blockchain/config"

	"github.com/zeebo/blake3"
)

type HashingID struct {
	NetworkID uint64
	Hash      [32]byte
}

type MiningBlob struct {
	Timestamp  uint64
	Nonce      uint32
	NonceExtra [16]byte
	Chains     []HashingID
}

var id_entropy = [7]byte{0x31, 0xab, 0xce, 0x87, 0xfe, 0xe4, 0xcc}

func (m MiningBlob) Serialize() []byte {
	s := binary.Ser{}

	s.AddUint64(m.Timestamp)             // 0:8
	s.AddFixedByteArray(m.NonceExtra[:]) // 8:24

	s.AddUint64(uint64(len(m.Chains))) // 24:32
	s.AddFixedByteArray(id_entropy[:]) // 32:39

	s.AddUint32(m.Nonce) // 39:43
	for _, v := range m.Chains {
		s.AddUint64(v.NetworkID)
		s.AddFixedByteArray(v.Hash[:])
	}

	return s.Output()
}

func (m *MiningBlob) Deserialize(b []byte) error {
	s := binary.NewDes(b)

	m.Timestamp = s.ReadUint64()
	m.NonceExtra = [16]byte(s.ReadFixedByteArray(16))

	numChains := s.ReadUint64()
	idEntropy := [7]byte(s.ReadFixedByteArray(7))
	if s.Error() != nil {
		return s.Error()
	}
	if numChains == 0 || numChains > config.MAX_MERGE_MINED_CHAINS {
		return fmt.Errorf("invalid number of chains %d", numChains)
	}
	if idEntropy != id_entropy {
		return fmt.Errorf("invalid identifier %x", idEntropy)
	}

	m.Nonce = s.ReadUint32()

	for i := 0; i < int(numChains); i++ {
		m.Chains = append(m.Chains, HashingID{
			NetworkID: s.ReadUint64(),
			Hash:      [32]byte(s.ReadFixedByteArray(32)),
		})
	}
	if s.Error() != nil {
		return s.Error()
	}

	remdata := s.RemainingData()
	if len(remdata) != 0 {
		return fmt.Errorf("unexpected remaining data: %d %x", len(remdata), remdata)
	}

	return nil
}

func (m MiningBlob) GetSeed() [32]byte {
	b := make([]byte, 8)
	seedhashId := GetSeedhashId(m.Timestamp)
	binary.LittleEndian.PutUint64(b, seedhashId)
	return blake3.Sum256(b)
}
func GetSeedhashId(time uint64) uint64 {
	return time / (config.SEEDHASH_DURATION * 1000)
}

func (m MiningBlob) String() string {
	return fmt.Sprintf("time %d nonce %d extra %x chains %x", m.Timestamp, m.Nonce, m.NonceExtra, m.Chains)
}
