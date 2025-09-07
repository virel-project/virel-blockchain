package p2p

import (
	"fmt"
	"net"
	"time"
)

const MAX_PEER_FAILURES = 1

// P2P must NOT be locked before calling this
func (p2 *P2P) startClient(addr string, port uint16, private bool) {
	conn, err := p2.connectClient(net.JoinHostPort(addr, fmt.Sprintf("%d", port)))
	if err != nil {
		Log.Net("error connecting to", addr+":", err)

		func() {
			p2.Lock()
			defer p2.Unlock()

			for i, kp := range p2.KnownPeers {
				if kp.IP == addr && kp.Port == port {
					kp.Fails++
					kp.LastConnect = time.Now().Unix()

					if kp.Fails > MAX_PEER_FAILURES {
						// The peer should be considered black, because it has too many failures
						kp.Type = PEER_BLACK
					}

					p2.KnownPeers[i] = kp
					return
				}
			}
		}()

		return
	} else {
		go func() {
			p2.Lock()
			defer p2.Unlock()

			for i, kp := range p2.KnownPeers {
				if kp.IP == addr && kp.Port == port {
					kp.Fails = 0
					if kp.Type == PEER_GRAY {
						kp.Type = PEER_WHITE
						kp.LastConnect = time.Now().Unix()
					}

					p2.KnownPeers[i] = kp
					return
				}
			}
		}()
	}

	err = p2.handleConnection(conn, private)
	if err != nil {
		Log.Debug("P2P client connection error:", err)
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
