package util

import (
	"encoding/hex"
	"errors"
	"fmt"
)

type Hash [32]byte

func (m Hash) String() string {
	return hex.EncodeToString(m[:])
}
func (m *Hash) UnmarshalText(c []byte) error {
	if len(c) != 64 {
		return errors.New("invalid length")
	}
	dst := make([]byte, 32)

	_, err := hex.Decode(dst, c)
	if err != nil {
		return err
	}

	*m = [32]byte(dst)

	return err
}

func (m Hash) MarshalJSON() ([]byte, error) {
	return []byte(`"` + m.String() + `"`), nil
}
func (m *Hash) UnmarshalJSON(c []byte) error {
	if len(c) != 66 {
		return errors.New("invalid hex length")
	} else if len(c) == 2 {
		*m = [32]byte{}
		return nil
	}

	if c[0] != '"' || c[len(c)-1] != '"' {
		return errors.New("invalid string literal")
	}

	dst := make([]byte, 32)

	_, err := hex.Decode(dst, c[1:len(c)-1])

	*m = [32]byte(dst)

	return err
}

func (m Hash) Format(f fmt.State, verb rune) {
	fmt.Fprint(f, m.String())
}
