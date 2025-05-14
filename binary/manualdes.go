package binary

import (
	"encoding/binary"
	"errors"
	"math/big"
	"runtime"
	"strconv"
	"strings"
)

func NewDes(data []byte) Des {
	return Des{
		data: data,
	}
}

type Des struct {
	data []byte
	err  error
}

func (d Des) RemainingData() []byte {
	return d.data
}

func (s *Des) ReadUint8() uint8 {
	if s.err != nil {
		return 0
	}
	if len(s.data) < 1 {
		s.err = errors.New(getCaller() + " invalid length")
		return 0
	}
	b := s.data[0]
	s.data = s.data[1:]
	return b
}
func (s *Des) ReadUint16() uint16 {
	if s.err != nil {
		return 0
	}
	if len(s.data) < 2 {
		s.err = errors.New(getCaller() + " invalid length")
		return 0
	}
	b := s.data[:2]
	s.data = s.data[2:]
	return binary.LittleEndian.Uint16(b)
}
func (s *Des) ReadUint32() uint32 {
	if s.err != nil {
		return 0
	}
	if len(s.data) < 4 {
		s.err = errors.New(getCaller() + " invalid length")
		return 0
	}
	b := s.data[:4]
	s.data = s.data[4:]
	return binary.LittleEndian.Uint32(b)
}
func (s *Des) ReadUint64() uint64 {
	if s.err != nil {
		return 0
	}
	if len(s.data) < 8 {
		s.err = errors.New(getCaller() + " invalid length")
		return 0
	}
	b := s.data[:8]
	s.data = s.data[8:]
	return binary.LittleEndian.Uint64(b)
}
func (s *Des) ReadUvarint() uint64 {
	if s.err != nil {
		return 0
	}
	if len(s.data) < 1 {
		s.err = errors.New(getCaller() + " invalid length")
		return 0
	}
	d, x := binary.Uvarint(s.data)
	if x < 0 {
		s.err = errors.New(getCaller() + " invalid uvarint")
		return 0
	}
	s.data = s.data[x:]
	return d
}

func (s *Des) ReadFixedByteArray(length int) []byte {
	if s.err != nil {
		return make([]byte, length)
	}
	if len(s.data) < length {
		s.err = errors.New(getCaller() + " invalid length")
		return make([]byte, length)
	}
	b := s.data[:length]
	s.data = s.data[length:]
	return b
}
func (s *Des) ReadByteSlice() []byte {
	if s.err != nil {
		return []byte{}
	}
	if len(s.data) < 1 {
		s.err = errors.New(getCaller() + " invalid length")
		return []byte{}
	}
	length, read := binary.Uvarint(s.data)
	if read < 0 {
		s.err = errors.New(getCaller() + " invalid uvarint length")
		return []byte{}
	}
	s.data = s.data[read:]
	if len(s.data) < int(length) {
		s.err = errors.New(getCaller() + " invalid binary length")
		return []byte{}
	}

	b := s.data[:length]
	s.data = s.data[length:]
	return b
}
func (s *Des) ReadString() string {
	return string(s.ReadByteSlice())
}

func (s *Des) ReadBool() bool {
	if s.err != nil {
		return false
	}
	if len(s.data) < 1 {
		s.err = errors.New(getCaller() + " invalid length")
		return false
	}
	b := s.data[0]
	s.data = s.data[1:]

	switch b {
	case 1:
		return true
	case 2:
		return false
	default:
		s.err = errors.New(getCaller() + " invalid boolean value")
		return false
	}
}
func (s *Des) ReadBigInt() *big.Int {
	return (&big.Int{}).SetBytes(s.ReadByteSlice())
}

func (s *Des) Error() error {
	return s.err
}

func getCaller() string {
	_, file, line, _ := runtime.Caller(2)
	fileSpl := strings.Split(file, "/")
	return fileSpl[len(fileSpl)-1] + ":" + strconv.FormatInt(int64(line), 10)
}
