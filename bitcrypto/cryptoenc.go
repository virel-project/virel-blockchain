package bitcrypto

import (
	"encoding/base64"
	"errors"
)

func (m Privkey) MarshalJSON() ([]byte, error) {
	return []byte(`"` + base64.StdEncoding.EncodeToString(m[:]) + `"`), nil
}
func (m *Privkey) UnmarshalJSON(c []byte) error {
	if len(c) < 2 {
		return errors.New("value is too short")
	} else if len(c) == 2 {
		*m = Privkey{}
		return nil
	}

	if c[0] != '"' || c[len(c)-1] != '"' {
		return errors.New("invalid string literal")
	}

	if len(c) != base64.StdEncoding.EncodedLen(PRIVKEY_SIZE)+2 {
		return errors.New("invalid privkey data length")
	}

	dst := make([]byte, base64.StdEncoding.EncodedLen(PRIVKEY_SIZE))

	_, err := base64.StdEncoding.Decode(dst, c[1:len(c)-1])

	*m = Privkey(dst)

	return err
}
