package enc

import (
	"encoding/hex"
	"errors"
)

type Hex []byte

func (h Hex) String() string {
	return hex.EncodeToString(h)
}
func (h *Hex) UnmarshalText(c []byte) error {
	dst := make([]byte, hex.EncodedLen(len(c)))

	n, err := hex.Decode(dst, c)

	*h = append((*h)[0:0], dst[:n]...)

	return err
}

func (h Hex) MarshalJSON() ([]byte, error) {
	return []byte(`"` + hex.EncodeToString(h) + `"`), nil
}
func (h *Hex) UnmarshalJSON(c []byte) error {
	if c == nil || len(c) < 2 {
		return errors.New("hex value is too short to be a valid string")
	} else if len(c) == 2 {
		*h = append((*h)[0:0], []byte{}...)
		return nil
	}

	if c[0] != '"' || c[len(c)-1] != '"' {
		return errors.New("hex value is not a valid string literal")
	}

	dst := make([]byte, hex.EncodedLen(len(c)))

	n, err := hex.Decode(dst, c[1:len(c)-1])

	*h = append((*h)[0:0], dst[:n]...)

	return err
}
