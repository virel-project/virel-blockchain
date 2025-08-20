package address

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"math/big"

	"github.com/virel-project/virel-blockchain/bitcrypto"
	"github.com/virel-project/virel-blockchain/config"

	"github.com/zeebo/blake3"
)

const SIZE = 22

type Address [SIZE]byte

// The zero-value of address is considered invalid
var INVALID_ADDRESS = Address{}

func FromPubKey(p bitcrypto.Pubkey) Address {
	// Address is obtained from the hash of the public key
	hash := blake3.Sum256(p[:])

	return Address(hash[:SIZE]) // the first SIZE bytes of the hash are the actual address
}
func FromString(p string) (Integrated, error) {
	if p[0] != config.WALLET_PREFIX[0] || len(p) < 4 {
		return Integrated{}, errors.New("invalid address prefix")
	}
	p = p[1:]

	bigi, success := big.NewInt(0).SetString(p, 36)
	if !success {
		return Integrated{}, errors.New("invalid address base36 encoding")
	}

	data := bigi.Bytes()

	if len(data) < SIZE+2 {
		return Integrated{}, fmt.Errorf("invalid address size: %d", len(data))
	}

	sum := checksum(data[2:])
	if data[0] != sum[0] || data[1] != sum[1] {
		return Integrated{}, fmt.Errorf("invalid address checksum %x%x calculated %x%x", data[0], data[1], sum[0], sum[1])
	}

	var subaddr uint64 = 0
	if len(data) > SIZE+2 {
		paymentIDBytes := data[SIZE+2:]
		b := make([]byte, 8)
		copy(b, paymentIDBytes)
		subaddr = binary.LittleEndian.Uint64(b)
	}

	return Integrated{
		Addr:      Address(data[2 : 2+SIZE]),
		PaymentId: subaddr,
	}, nil
}

func checksum(a []byte) []byte {
	sum := crc32.ChecksumIEEE(a[:])
	sumb := make([]byte, 2)
	binary.LittleEndian.PutUint16(sumb, uint16(sum&0xffff))
	return sumb
}

func (a Address) Integrated() Integrated {
	return Integrated{
		Addr: a,
	}
}
func (a Address) String() string {
	return a.Integrated().String()
}

type Integrated struct {
	Addr      Address
	PaymentId uint64
}

func (a Integrated) bytes() []byte {
	b := a.Addr[:]

	b = append(b, Uint64ToCompactLittleEndian(a.PaymentId)...)

	return append(checksum(b), b...)
}

func (a Integrated) String() string {
	return config.WALLET_PREFIX + big.NewInt(0).SetBytes(a.bytes()).Text(36)
}

func (a *Integrated) Marshal() ([]byte, error) {
	return []byte(`"` + a.String() + `"`), nil
}

func (a Integrated) MarshalJSON() ([]byte, error) {
	return []byte(`"` + a.String() + `"`), nil
}

func (a *Integrated) UnmarshalJSON(c []byte) error {
	if len(c) < 2 {
		return errors.New("value is too short")
	}

	if c[0] != '"' || c[len(c)-1] != '"' {
		return errors.New("invalid string literal")
	}

	addr, err := FromString(string(c[1 : len(c)-1]))

	if err != nil {
		return err
	}

	*a = addr

	return nil
}

var GenesisAddress Address

func init() {
	addr, err := FromString(config.GENESIS_ADDRESS)
	if err != nil {
		panic(err)
	}
	GenesisAddress = addr.Addr
}

func Uint64ToCompactLittleEndian(n uint64) []byte {
	// Handle zero case - return empty slice
	if n == 0 {
		return []byte{}
	}

	// Convert to little-endian byte slice (8 bytes)
	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, n)

	// Find the index of the last non-zero byte
	lastNonZero := 0
	for i := len(bytes) - 1; i >= 0; i-- {
		if bytes[i] != 0 {
			lastNonZero = i
			break
		}
	}

	// Return slice up to and including the last non-zero byte
	return bytes[:lastNonZero+1]
}
