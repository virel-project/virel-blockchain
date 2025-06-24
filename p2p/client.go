package p2p

import (
	"net"
)

// P2P must NOT be locked before calling this
func (p2 *P2P) startClient(addr string) {
	conn, err := p2.connectClient(addr)
	if err != nil {
		Log.Net("error connecting to", addr+":", err)
		return
	}

	err = p2.handleConnection(conn)
	if err != nil {
		Log.Debug("P2P client connection error:", err)
	} else {
		Log.Debug("P2P client connection gracefully closed")
	}
}

func (p2 *P2P) connectClient(addr string) (*Connection, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		Log.Net(err)
		return &Connection{}, err
	}
	return NewConnection(conn, true), nil
}
