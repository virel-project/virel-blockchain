package binary

import (
	"testing"
)

func BenchmarkBinary(b *testing.B) {
	s := NewSer(make([]byte, b.N*4))

	n := uint64(b.N)
	for i := uint64(0); i < n; i++ {
		s.AddUvarint(i)
	}

	b.Logf("actual encoded length: %d; n*4: %d", len(s.Output()), n*4)

	d := NewDes(s.Output())

	for i := uint64(0); i < n; i++ {
		d.ReadUvarint()
	}

	if d.Error() != nil {
		b.Fatal(d.err)
	}
}

func TestSomething(t *testing.T) {
	b := make([]byte, 10000)
	b2 := b[0:0]
	t.Log(b2)
	t.Log(len(b2), cap(b2))
}
