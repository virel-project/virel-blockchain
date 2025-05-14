package util

import (
	"encoding/json"
	"testing"

	"github.com/zeebo/blake3"
)

func TestHash(t *testing.T) {
	type MyType struct {
		Data Hash
	}

	st := MyType{
		Data: Hash(blake3.Sum256([]byte("test"))),
	}

	data, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Data1: %s", data)

	st2 := MyType{}

	err = json.Unmarshal(data, &st2)
	if err != nil {
		t.Fatal(err)
	}

	data, err = json.Marshal(st2)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Data2: %s", data)

	if st.Data != st2.Data {
		t.Fatal("st.Data is different from st2.Data")
	}
}
