package binary

import (
	"encoding/binary"
)

var (
	LittleEndian  = binary.LittleEndian
	BigEndian     = binary.BigEndian
	DefaultEndian = LittleEndian
)

var AppendUvarint = binary.AppendUvarint
