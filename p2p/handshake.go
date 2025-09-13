package p2p

import (
	"fmt"
	"io"

	"github.com/virel-project/virel-blockchain/v3/binary"
)

const MAX_HANDSHAKE_SIZE = 1024

// Special packet that contains information such as the peer ID
type Handshake struct {
	Version    uint64
	P2PVersion uint8

	PeerID [32]byte
	// if zero, the peer doesn't have a public P2P server
	P2PPort uint16
}

func (h *Handshake) ReadFrom(i io.Reader) (int64, error) {
	var n int64 = 0
	hndsLength := make([]byte, 4)
	c, err := i.Read(hndsLength)
	n += int64(c)
	if err != nil {
		return n, fmt.Errorf("failed to read handshake length: %v", err)
	}

	l := binary.LittleEndian.Uint32(hndsLength)

	if l > MAX_HANDSHAKE_SIZE {
		return n, fmt.Errorf("invalid handshake size: %d", l)
	}

	hndsData := make([]byte, l)
	c, err = i.Read(hndsData)
	n += int64(c)
	if err != nil {
		return n, fmt.Errorf("failed to read handshake: %v", err)
	}

	return n, h.Deserialize(hndsData)
}
func (h *Handshake) WriteTo(w io.Writer) (int64, error) {
	hnds := h.Serialize()
	hndsLength := make([]byte, 4)

	binary.LittleEndian.PutUint32(hndsLength, uint32(len(hnds)))

	var n int64
	c, err := w.Write(hndsLength)
	n += int64(c)
	if err != nil {
		return n, err
	}
	c, err = w.Write(hnds)
	n += int64(c)
	return n, err
}

func (h *Handshake) Deserialize(data []byte) error {
	d := binary.NewDes(data)

	h.Version = d.ReadUint64()
	h.P2PVersion = d.ReadUint8()
	h.PeerID = [32]byte(d.ReadFixedByteArray(32))
	h.P2PPort = d.ReadUint16()

	if d.Error() != nil {
		return d.Error()
	}

	if len(d.RemainingData()) != 0 {
		return fmt.Errorf("remaining data is not empty, but has length %d", len(d.RemainingData()))
	}

	return nil
}
func (h *Handshake) Serialize() []byte {
	b := binary.NewSer(make([]byte, 48))

	b.AddUint64(h.Version)
	b.AddUint8(h.P2PVersion)
	b.AddFixedByteArray(h.PeerID[:])
	b.AddUint16(h.P2PPort)

	return b.Output()
}
