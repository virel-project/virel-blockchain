package enc

import (
	"encoding/base64"
	"errors"
)

type B64 []byte

func (b B64) String() string {
	return base64.StdEncoding.EncodeToString(b)
}
func (b *B64) UnmarshalText(c []byte) error {
	dst := make([]byte, base64.StdEncoding.EncodedLen(len(c)))

	n, err := base64.StdEncoding.Decode(dst, c)

	*b = append((*b)[0:0], dst[:n]...)

	return err
}

func (b B64) MarshalJSON() ([]byte, error) {
	return []byte(`"` + base64.StdEncoding.EncodeToString(b) + `"`), nil
}
func (b *B64) UnmarshalJSON(c []byte) error {
	if c == nil || len(c) < 2 {
		return errors.New("base64 value is too short to be a valid string")
	} else if len(c) == 2 {
		*b = append((*b)[0:0], []byte{}...)
		return nil
	}

	if c[0] != '"' || c[len(c)-1] != '"' {
		return errors.New("base64 value is not a valid string literal")
	}

	dst := make([]byte, base64.StdEncoding.EncodedLen(len(c)))

	n, err := base64.StdEncoding.Decode(dst, c[1:len(c)-1])

	*b = append((*b)[0:0], dst[:n]...)

	return err
}
