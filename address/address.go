package address

import (
	"encoding/binary"
	"errors"
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
		return Integrated{}, errors.New("invalid address")
	}

	data := bigi.Bytes()

	if len(data) < SIZE+2 {
		return Integrated{}, errors.New("invalid address")
	}

	sum := checksum(data[len(data)-SIZE:])
	if data[0] != sum[0] || data[1] != sum[1] {
		return Integrated{}, errors.New("invalid address checksum")
	}

	var subaddr uint64 = 0
	if len(data) == SIZE+2 {
		var code int
		subaddr, code = binary.Uvarint(data[2+SIZE:])
		if code < 0 {
			return Integrated{}, errors.New("invalid integrated value")
		}
	}

	return Integrated{
		Addr:    Address(data[2 : 2+SIZE]),
		Subaddr: subaddr,
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
	Addr    Address
	Subaddr uint64
}

func (a Integrated) bytes() []byte {
	b := a.Addr[:]

	sbytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(sbytes, a.Subaddr)
	for len(sbytes) > 0 && sbytes[len(sbytes)-1] == 0 {
		sbytes = sbytes[:len(sbytes)-2]
	}
	b = append(b, sbytes...)

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
