package enc

import (
	"encoding/json"
	"reflect"
	"testing"
)

type testStruct struct {
	Base64val B64
	Hexval    Hex
}

func TestEnc(t *testing.T) {
	str := testStruct{
		Base64val: B64("This value will be encoded as Base64 in JSON"),
		Hexval:    Hex("While this value will be encoded using the hexadecimal format"),
	}

	mar, err := json.Marshal(str)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("%s", mar)

	str2 := testStruct{}
	err = json.Unmarshal(mar, &str2)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(str, str2) {
		t.Fatal("structs are different")
	}
}
