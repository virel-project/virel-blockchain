package p2p

import (
	"crypto/ecdh"
	"crypto/rand"
	"fmt"
	"io"
	mrand "math/rand/v2"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
	"virel-blockchain/binary"
	"virel-blockchain/bitcrypto"
	"virel-blockchain/config"
	"virel-blockchain/logger"
	"virel-blockchain/p2p/packet"
	"virel-blockchain/util"

	"github.com/zeebo/blake3"
)

var Log = logger.DiscardLog

const P2P_PING_INTERVAL = 15 // seconds
const P2P_TIMEOUT = 40       // seconds

type P2P struct {
	Privkey *ecdh.PrivateKey

	// IP:PORT -> Connection
	Connections    map[string]*Connection
	PacketsIn      chan Packet
	NewConnections chan *Connection
	KnownPeers     []KnownPeer

	listener net.Listener

	util.RWMutex
}

func (p *P2P) PeerId() [32]byte {
	return [32]byte(p.Privkey.Public().(*ecdh.PublicKey).Bytes())
}

type PeerData struct {
	Stats      packet.PacketStats
	LastHeight uint64 // last block height requested to this peer
}

type KnownPeer struct {
	IP          string
	Port        uint16
	Type        PeerType
	LastConnect int64 // UNIX seconds
}

func (k KnownPeer) IsBanned() bool {
	if k.Type != PEER_RED {
		return false
	}
	return time.Now().Unix() < k.LastConnect
}

type Packet struct { // exported
	Type packet.Type // Packet type, starting from 0
	Data []byte      // Packet data
	Conn *Connection
}

type pack struct { // only used inside p2p
	Type uint16
	Data []byte
}

func (p *Packet) String() string {
	if p.Data == nil {
		return fmt.Sprintf("%d nil", p.Type)

	}
	return fmt.Sprintf("%d %x", p.Type, p.Data)
}
func (p *pack) String() string {
	if p.Data == nil {
		return fmt.Sprintf("%d nil", p.Type)

	}
	return fmt.Sprintf("%d %x", p.Type, p.Data)
}

func Start(peers []string) *P2P {
	pk, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}

	Log.Infof("P2P public key: %x", pk.PublicKey().Bytes())

	p := P2P{
		Privkey:        pk,
		PacketsIn:      make(chan Packet),
		NewConnections: make(chan *Connection),
		Connections:    make(map[string]*Connection),
	}
	for _, v := range peers {
		splv := strings.Split(v, ":")
		if len(splv) < 1 {
			continue
		}
		port := uint64(config.P2P_BIND_PORT)
		if len(splv) > 1 {
			port, err = strconv.ParseUint(splv[1], 10, 16)
			if err != nil {
				Log.Warn(err.Error())
				continue
			}
		}
		if port == 0 {
			port = config.P2P_BIND_PORT
		}

		p.KnownPeers = append(p.KnownPeers, KnownPeer{
			IP:   splv[0],
			Port: uint16(port),
			Type: PEER_WHITE,
		})
	}
	return &p
}

func (p *P2P) Close() {
	p.listener.Close()
	for _, v := range p.Connections {
		v.View(func(c *ConnData) error {
			c.Close()
			return nil
		})
	}
	Log.Info("P2P closed")
}

var curve = ecdh.X25519()

func (p *P2P) ListenServer(port uint16) {
	listen, err := net.Listen("tcp", "0.0.0.0:"+strconv.FormatUint(uint64(port), 10))
	if err != nil {
		Log.Fatal(err)
		os.Exit(1)
	}
	defer listen.Close()

	p.listener = listen

	Log.Infof("P2P server listening: %s:%d", "0.0.0.0", port)

	for {
		c, err := listen.Accept()
		if err != nil {
			Log.Debug("listener closed:", err)
			return
		}
		conn := NewConnection(c, false)

		// prevent banned peers from connecting
		for _, v := range p.KnownPeers {
			if v.IP == conn.data.IP() && v.IsBanned() {
				Log.Debugf("peer %s is banned", c.RemoteAddr().String())
				return
			}
		}

		Log.Infof("New connection with IP %s", c.RemoteAddr().String())
		p.handleConnection(conn)
	}
}
func (p *P2P) StartClients() {
	go func() {
		for {
			p.Lock()
			if len(p.Connections) < config.P2P_CONNECTIONS {
				p.connectToRandomPeer()
			}
			p.Unlock()
			time.Sleep(15 * time.Second)
		}
	}()
}

// P2P MUST be locked before calling this
func (p *P2P) connectToRandomPeer() {
scanning:
	for i := 0; i < 5; i++ {
		if len(p.KnownPeers) == 0 {
			return
		}
		randPeer := p.KnownPeers[mrand.IntN(len(p.KnownPeers))]
		if randPeer.IsBanned() {
			continue
		}

		for _, conn := range p.Connections {
			continueScan := false
			conn.View(func(c *ConnData) error {
				if c.IP() == randPeer.IP {
					continueScan = true
				}
				return nil
			})
			if continueScan {
				continue scanning
			}
		}

		go p.startClient(randPeer.IP + ":" + strconv.FormatUint(uint64(randPeer.Port), 10))
	}
}

// p2p must NOT be locked before calling this
func (p *P2P) Kick(c *Connection) {
	c.Update(func(c *ConnData) error {
		c.Close()
		ip := c.Conn.RemoteAddr().String()
		Log.Debug("p2p kick", ip)
		c.Close()
		p.Lock()
		delete(p.Connections, ip)
		p.Unlock()
		return nil
	})
}

func (p *P2P) handleConnection(conn *Connection) {
	var ipPort string
	shouldReturn := false
	err := conn.Update(func(c *ConnData) error {
		ipPort = c.Conn.RemoteAddr().String()
		var peerid [32]byte
		err := func() error {
			p.Lock()
			defer p.Unlock()

			peerid = p.PeerId()

			if p.Connections[ipPort] != nil {
				return fmt.Errorf("peer %s is already connected", ipPort)
			}
			p.Connections[ipPort] = conn
			return nil
		}()
		if err != nil {
			c.Close()
			shouldReturn = true
			return nil
		}

		// send peer ID
		c.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, err = c.Conn.Write(peerid[:])
		return err
	})
	if shouldReturn {
		return
	}
	if err != nil {
		p.Kick(conn)
		return
	}

	// read peer ID
	conn.data.Conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	peerIdBin := make([]byte, 32)
	_, err = conn.data.Conn.Read(peerIdBin)
	if err != nil {
		err := fmt.Errorf("error occurred while reading peer id: %s", err)
		Log.Warn(err)
		p.Kick(conn)
		return
	}
	err = conn.Update(func(c *ConnData) error {
		c.PeerId = bitcrypto.Pubkey(peerIdBin)

		Log.Debugf("New connection with ID %x", c.PeerId)

		if c.PeerId == p.PeerId() {
			err := fmt.Errorf("disconnecting from peer: connection to self detected")
			return err
		}

		func() {
			p.RLock()
			conns := p.Connections
			p.RUnlock()

			for addr, vconn := range conns {
				if addr == ipPort {
					continue
				}
				err := vconn.View(func(v *ConnData) error {
					if v.PeerId == c.PeerId && v.Conn.RemoteAddr().String() != c.Conn.RemoteAddr().String() {
						err := fmt.Errorf("disconnecting from peer: duplicate ID %x", c.PeerId)
						return err
					}
					return nil
				})
				if err != nil {
					Log.Warn(err)
				}
			}
		}()

		peerpub, err := curve.NewPublicKey(c.PeerId[:])
		if err != nil {
			Log.Err(err)
			return err
		}

		sharedkey, err := p.Privkey.ECDH(peerpub)
		if err != nil {
			Log.Err(err)
			return err
		}

		// Network ID is used as P2P encryption context
		c.Cipher, err = bitcrypto.NewCipher(blake3.Sum256(append(config.BinaryNetworkID, sharedkey...)))
		if err != nil {
			Log.Err(err)
			return err
		}

		p.NewConnections <- conn

		// check if this peer is already connected
		err = func() error {
			p.RLock()
			defer p.RUnlock()

			n := 0
			for i, vconn := range p.Connections {
				if i == ipPort {
					continue
				}
				err := vconn.Update(func(v *ConnData) error {
					if v.PeerId == c.PeerId {
						n++
						if n >= 2 {
							err := fmt.Errorf("peer with id: %x is already connected - disconnecting it", c.PeerId)
							return err
						}
					}
					return nil
				})
				if err != nil {
					return err
				}
			}
			return err
		}()
		if err != nil {
			return err
		}

		// update and broadcast peerlist
		s := binary.Ser{}
		p.Lock()
		for i, v := range p.KnownPeers {
			if v.IP == c.IP() {
				v.Type = PEER_WHITE
				p.KnownPeers[i] = v
			} else if v.Type == PEER_WHITE {
				s.AddUint16(v.Port)
				s.AddString(v.IP)
			}
		}
		p.Unlock()
		c.sendPacket(pack{
			Type: 2,
			Data: s.Output(),
		})

		return nil
	})
	if err != nil {
		Log.Warn(err)
		p.Kick(conn)
		return
	}

	for {
		var encData []byte
		var packetType uint16
		var data []byte

		func() error {
			// packet length is unencrypted
			conn.data.Conn.SetReadDeadline(time.Now().Add(config.P2P_TIMEOUT * time.Second))
			packLenBuf := make([]byte, 4)
			_, err := io.ReadFull(conn.data.Conn, packLenBuf)
			if err != nil {
				Log.Warnf("connection error: %s", err)
				return err
			}

			packetLen := binary.LittleEndian.Uint32(packLenBuf)
			if packetLen > 1024*1024*4 /* 4 MiB */ {
				return fmt.Errorf("connection error: invalid packet length %d received", packetLen)
			}

			// now read encrypted data
			conn.data.Conn.SetReadDeadline(time.Now().Add(config.P2P_TIMEOUT * time.Second))
			encData = make([]byte, packetLen)
			_, err = io.ReadFull(conn.data.Conn, encData)
			if err != nil {
				return fmt.Errorf("connection error: cannot read encrypted packet data: %v", err)
			}

			data, err = conn.data.Cipher.Decrypt(encData)
			if err != nil {
				return fmt.Errorf("connection error: cannot decrypt data: %v", err)
			}

			return nil
		}()
		if err != nil {
			p.Kick(conn)
			return
		}

		err = conn.View(func(c *ConnData) error {

			des := binary.NewDes(data)

			packetType = des.ReadUint16()

			if des.Error() != nil {
				return fmt.Errorf("deserialize error: %v", des.Error())
			}

			data = des.RemainingData()

			Log.NetDevf("inc packet type %s data %x", packet.Type(packetType-2), data)

			return nil
		})
		if err != nil {
			p.Kick(conn)
			return
		}

		p.onPacketReceived(pack{Type: packetType, Data: data}, conn)
	}
}

func (p *P2P) onPacketReceived(pk pack, c *Connection) {
	c.Update(func(c *ConnData) error {
		c.LastPing = time.Now().Unix()
		return nil
	})

	if pk.Type == 0 { // unused
		return
	} else if pk.Type == 1 { // addPeer
		p.OnAddPeerPacket(pk.Data)
	} else {
		p.PacketsIn <- Packet{Data: pk.Data, Type: packet.Type(pk.Type - 2), Conn: c}
	}
}

func (p2 *P2P) OnAddPeerPacket(packetData []byte) error {
	if len(packetData) < 3 {
		return fmt.Errorf("invalid AddPeer packet length")
	}

	d := binary.NewDes(packetData)

	for len(packetData) > 3 {
		port := d.ReadUint16()

		if port == 0 {
			return fmt.Errorf("invalid AddPeer port")
		}

		packetData = packetData[2:]
		address := net.ParseIP(d.ReadString())
		if address == nil {
			return fmt.Errorf("invalid AddPeer IP address")
		}
		if d.Error() != nil {
			return d.Error()
		}

		p2.Lock()
		shouldAdd := true
		for _, v := range p2.KnownPeers {
			if v.IP == address.String() {
				shouldAdd = false
				break
			}
		}
		Log.Debug("Add Peer Packet", address, port, "shouldAdd:", shouldAdd)
		if shouldAdd {
			p2.KnownPeers = append(p2.KnownPeers, KnownPeer{
				IP:   address.String(),
				Port: port,
				Type: PEER_GRAY,
			})
		}
		p2.Unlock()
	}

	go p2.savePeerlist()

	return nil
}
