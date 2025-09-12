package util

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/util/uint128"

	"github.com/sasha-s/go-deadlock"
)

// returns the timestamp (UNIX milliseconds)
func Time() uint64 {
	return uint64(time.Now().UnixMilli())
}
func FormatInt[V int | int64 | int32 | int16 | int8 | uint8 | uint16 | uint32](n V) string {
	return strconv.FormatInt(int64(n), 10)
}
func FormatUint[V uint | uint8 | uint16 | uint32 | uint64](n V) string {
	return strconv.FormatUint(uint64(n), 10)
}
func FormatCoin(n uint64) string {
	s := strconv.FormatUint(n, 10)

	for len(s) < int(config.ATOMIC)+1 {
		s = "0" + s
	}

	return s[:len(s)-int(config.ATOMIC)] + "." + s[len(s)-int(config.ATOMIC):]
}

func PadR(s string, l int) string {
	for len(s) < l {
		s = " " + s
	}
	return s
}
func PadL(s string, l int) string {
	for len(s) < l {
		s = s + " "
	}
	return s
}

func PadC(s string, l int) string {
	for len(s)+1 < l {
		s = " " + s + " "
	}
	if len(s) < l {
		s = s + " "
	}
	return s

}

func U64Bytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, v)
	return b
}

// only use this in configuration etc - panics if the hex is invalid
func AssertHexDec(s string) []byte {
	dat, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return dat
}

func init() {
	deadlock.Opts.DeadlockTimeout = 30 * time.Second
}

type Mutex = deadlock.Mutex
type RWMutex = deadlock.RWMutex

func RandomUint64() uint64 {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return binary.LittleEndian.Uint64(b)
}

func RandomInt(max int) int {
	return int(RandomUint64() % uint64(max))
}

func RemovePort(s string) string {
	return strings.Split(s, ":")[0]
}

func GetTarget(n uint128.Uint128) uint64 {
	return 0xffffffffffffffff / n.Lo
}

func GetTargetBytes(n uint128.Uint128) []byte {
	target := GetTarget(n)
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, target)
	return b
}
func ByteTargetToDiff(t []byte) uint128.Uint128 {
	if len(t) == 16 {
		return TargetToDiff(uint128.FromBytes(t))
	} else if len(t) == 8 {
		return TargetToDiff64(binary.LittleEndian.Uint64(t))
	} else if len(t) == 4 {
		return TargetToDiff64(uint64(binary.LittleEndian.Uint32(t)))
	} else {
		panic("invalid target size supplied")
	}
}

const max_uint64 = 0xffffffffffffffff

func TargetToDiff64(t uint64) uint128.Uint128 {
	return uint128.New(max_uint64, 0).Div64(t)
}
func TargetToDiff(t uint128.Uint128) uint128.Uint128 {
	return uint128.New(max_uint64, max_uint64).Div(t)
}

func IsHex(s string) bool {
	for _, v := range s {
		if v < '0' || v > 'f' || (v > '9' && v < 'a') {
			return false
		}
	}
	return true
}

func SafeAdd(a, b uint64) (uint64, error) {
	if a+b < a {
		return 0, errors.New("overflow")
	}
	return a + b, nil
}
