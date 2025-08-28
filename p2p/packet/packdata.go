package packet

import (
	"fmt"

	"github.com/virel-project/virel-blockchain/v2/binary"
	"github.com/virel-project/virel-blockchain/v2/util/uint128"
)

type Uint128 = uint128.Uint128

// PacketStats
type PacketStats struct {
	Height         uint64
	CumulativeDiff Uint128
	Hash           [32]byte
}

func (p PacketStats) Serialize() []byte {
	s := binary.NewSer(make([]byte, 34))

	s.AddUvarint(p.Height)

	diff := make([]byte, 16)
	p.CumulativeDiff.PutBytes(diff)
	for diff[len(diff)-1] == 0 {
		diff = diff[:len(diff)-1]
	}
	s.AddByteSlice(diff)

	s.AddFixedByteArray(p.Hash[:])

	return s.Output()
}

func (p *PacketStats) Deserialize(data []byte) error {
	d := binary.NewDes(data)

	p.Height = d.ReadUvarint()

	// read difficulty
	diff := make([]byte, 16)
	copy(diff, d.ReadByteSlice())
	p.CumulativeDiff = Uint128{
		Hi: binary.LittleEndian.Uint64(diff[8:]),
		Lo: binary.LittleEndian.Uint64(diff[:8]),
	}

	p.Hash = [32]byte(d.ReadFixedByteArray(32))

	return d.Error()
}

func (p PacketStats) String() string {
	return fmt.Sprintf("Height: %d; Cumulative diff: %s; Hash: %x", p.Height, p.CumulativeDiff, p.Hash)
}

type PacketBlockRequest struct {
	Height uint64 // if height is zero, then request is by hash
	Hash   [32]byte
	Count  uint8 // how many blocks to request after the requested block (only if height is not zero)
}

func (p PacketBlockRequest) Serialize() []byte {
	s := binary.NewSer(make([]byte, 0, 9))
	s.AddUvarint(p.Height)
	if p.Height == 0 {
		s.AddFixedByteArray(p.Hash[:])
	} else {
		s.AddUint8(uint8(p.Count))
	}
	return s.Output()
}
func (p *PacketBlockRequest) Deserialize(d []byte) error {
	s := binary.NewDes(d)
	p.Height = s.ReadUvarint()
	if p.Height == 0 {
		p.Hash = [32]byte(s.ReadFixedByteArray(32))
	} else {
		p.Count = s.ReadUint8()
	}
	return s.Error()
}
