package binary

import (
	"encoding/binary"
	"math/big"
)

func NewSer(reuseSlice []byte) Ser {
	return Ser{
		data: reuseSlice[0:0],
	}
}

type Ser struct {
	data []byte
}

func (s Ser) Output() []byte {
	return s.data
}

func (s *Ser) AddUint8(n uint8) {
	s.data = append(s.data, n)
}
func (s *Ser) AddUint16(n uint16) {
	s.data = binary.LittleEndian.AppendUint16(s.data, n)
}
func (s *Ser) AddUint32(n uint32) {
	s.data = binary.LittleEndian.AppendUint32(s.data, n)
}
func (s *Ser) AddUint64(n uint64) {
	s.data = binary.LittleEndian.AppendUint64(s.data, n)
}

func (s *Ser) AddUvarint(n uint64) {
	s.data = binary.AppendUvarint(s.data, n)
}

// adds a fixed-length byte array
func (s *Ser) AddFixedByteArray(a []byte) {
	s.data = append(s.data, a...)
}

// adds a variable-length byte slice
func (s *Ser) AddByteSlice(a []byte) {
	s.data = append(binary.AppendUvarint(s.data, uint64(len(a))), a...)
}

// adds a string
func (s *Ser) AddString(a string) {
	s.AddByteSlice([]byte(a))
}

// adds a BigInt
func (s *Ser) AddBigInt(n *big.Int) {
	bin := n.Bytes()

	s.data = append(binary.AppendUvarint(s.data, uint64(len(bin))), bin...)
}

// adds a boolean value
func (s *Ser) AddBool(b bool) {
	if b {
		s.data = append(s.data, 0)
	} else {
		s.data = append(s.data, 1)
	}
}
