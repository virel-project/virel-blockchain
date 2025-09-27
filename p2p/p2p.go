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

	"github.com/virel-project/virel-blockchain/v3/binary"
	"github.com/virel-project/virel-blockchain/v3/bitcrypto"
	"github.com/virel-project/virel-blockchain/v3/config"
	"github.com/virel-project/virel-blockchain/v3/logger"
	"github.com/virel-project/virel-blockchain/v3/p2p/packet"
	"github.com/virel-project/virel-blockchain/v3/util"

	"github.com/zeebo/blake3"
)

var Log = logger.DiscardLog

const P2P_PING_INTERVAL = 15 // seconds

type P2P struct {
	DataDir string
	Privkey *ecdh.PrivateKey

	// IP:PORT -> Connection
	Connections map[string]*Connection

	BindPort uint16

	PacketsIn      chan Packet
	NewConnections chan *Connection
	KnownPeers     []KnownPeer

	listener net.Listener

	Exclusive bool

	util.RWMutex
}

// Returns the local peer ID (your public key)
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
	Fails       uint16
	LastConnect int64 // UNIX seconds
	Priority    bool
}

func (k KnownPeer) IsBanned() bool {
	if k.Type != PEER_RED {
		return false
	}
	return time.Now().Unix() < k.LastConnect+config.BAN_DURATION
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

func Start(peers []string, dataDir string) *P2P {
	pk, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}

	Log.Infof("P2P public key: %x", pk.PublicKey().Bytes())

	p := &P2P{
		DataDir:        dataDir,
		Privkey:        pk,
		PacketsIn:      make(chan Packet),
		NewConnections: make(chan *Connection),
		Connections:    make(map[string]*Connection),
	}

	err = p.loadPeerlist()
	if err != nil {
		Log.Info("Peerlist loading failed:", err)
		err = nil
	}

	for _, v := range peers {
		splv := strings.Split(v, ":")
		if len(splv) < 1 {
			continue
		}
		port := uint16(config.P2P_BIND_PORT)
		if len(splv) > 1 {
			var prt uint64
			prt, err = strconv.ParseUint(splv[1], 10, 16)
			if err != nil {
				Log.Warn(err.Error())
				continue
			}
			port = uint16(prt)
		}
		if port == 0 {
			port = config.P2P_BIND_PORT
		}

		p.AddPeerToList(splv[0], port, true)
	}
	return p
}

func (p *P2P) Close() {
	p.listener.Close()
	for _, v := range p.Connections {
		v.Close()
	}

	err := p.savePeerlist()
	if err != nil {
		Log.Warn(err)
	}

	Log.Info("P2P closed")
}

var curve = ecdh.X25519()

func (p *P2P) ListenServer(port uint16, private bool) {
	p.BindPort = port

	listen, err := net.Listen("tcp", "0.0.0.0:"+strconv.FormatUint(uint64(port), 10))
	if err != nil {
		Log.Fatal(err)
		os.Exit(1)
	}
	defer listen.Close()

	p.listener = listen

	Log.Infof("P2P server listening: %s:%d", "0.0.0.0", port)

	for {
		Log.Netf("P2P server ready to accept new connections")

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

		Log.Netf("Received TCP P2P connection request with IP %s", c.RemoteAddr().String())
		err = p.handleConnection(conn, private)
		if err != nil {
			Log.Debug("P2P server connection error:", err)
		}
	}
}
func (p *P2P) StartClients(private bool) {
	for {
		p.Lock()
		connCount := 0
		for _, v := range p.Connections {
			v.View(func(c *ConnData) error {
				if c.Outgoing {
					connCount++
				}
				return nil
			})
		}
		if connCount < config.P2P_CONNECTIONS {
			p.connectToRandomPeer(private)
		}
		p.Unlock()
		time.Sleep(5 * time.Second)
	}
}

// P2P MUST be locked before calling this
func (p *P2P) connectToRandomPeer(private bool) {
scanning:
	for range 5 {
		if len(p.KnownPeers) == 0 {
			return
		}

		// 1/3 chance of trying to pick a priority node
		priority := util.RandomInt(3) == 0

		var randPeer KnownPeer

		randPeer = p.KnownPeers[mrand.IntN(len(p.KnownPeers))]
		if priority {
			for range len(p.KnownPeers)/2 + 1 {
				if randPeer.Priority {
					break
				}
				randPeer = p.KnownPeers[mrand.IntN(len(p.KnownPeers))]
			}
		}
		if randPeer.IsBanned() {
			continue
		}

		if randPeer.Type == PEER_BLACK && time.Now().Unix() < randPeer.LastConnect+config.BLACK_DURATION {
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

		Log.Debug("connecting to peer", randPeer.IP, randPeer.Port)
		go p.startClient(randPeer.IP, randPeer.Port, private)
		return
	}
}

// p2p must NOT be locked before calling this
// connection must NOT be locked before calling this
func (p *P2P) Kick(c *Connection) {
	var ip string
	c.View(func(c *ConnData) error {
		ip = c.Conn.RemoteAddr().String()
		return nil
	})
	c.Close()
	p.Lock()
	delete(p.Connections, ip)
	p.Unlock()
}

func (p *P2P) sendPeerList(conn *Connection) error {
	ip := ""
	conn.View(func(c *ConnData) error {
		ip = c.IP()
		return nil
	})

	Log.Debug("sending peer list to", ip)

	s := binary.Ser{}
	p.RLock()
	for i, v := range p.KnownPeers {
		if v.IP == ip {
			v.Type = PEER_WHITE
			v.LastConnect = time.Now().Unix()
			p.KnownPeers[i] = v
		} else if v.Type == PEER_WHITE {
			Log.NetDevf("sendPeerList: sending %v:%v", v.IP, v.Port)
			s.AddUint16(v.Port)
			s.AddString(v.IP)
		}
	}
	p.RUnlock()
	return conn.sendPacketLock(pack{
		Type: 1,
		Data: s.Output(),
	})

}

func (p *P2P) handleConnection(conn *Connection, private bool) error {
	var ipPort string
	conn.View(func(c *ConnData) error {
		ipPort = c.Conn.RemoteAddr().String()
		return nil
	})

	err := func() error {
		p.Lock()
		defer p.Unlock()
		if p.Connections[ipPort] != nil {
			return fmt.Errorf("peer %s is already connected", ipPort)
		}
		p.Connections[ipPort] = conn
		return nil
	}()
	if err != nil {
		conn.Close()
		return err
	}

	go conn.Writer()

	go func() {
		err := p.connectionMainHandling(conn, private, ipPort)
		if err != nil {
			Log.Debugf("connection error with peer %v: %v", ipPort, err)
		}
		p.Kick(conn)
	}()
	return nil
}

func (p *P2P) connectionMainHandling(conn *Connection, private bool, ipPort string) error {
	p.RLock()
	// Peer ID of this node
	var peerid = p.PeerId()
	p.RUnlock()

	// send handshake
	err := conn.Update(func(c *ConnData) error {
		c.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		port := p.BindPort
		if private {
			port = 0
		}
		hnds := &Handshake{
			Version:    config.VERSION,
			P2PVersion: config.P2P_VERSION,

			PeerID:  peerid,
			P2PPort: port,
		}
		_, err := hnds.WriteTo(c.Conn)
		return err
	})
	if err != nil {
		Log.Debug("failed to send handshake:", err)
		return err
	}

	// read handshake
	conn.data.Conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	hnds := &Handshake{}
	_, err = hnds.ReadFrom(conn.data.Conn)
	if err != nil {
		err := fmt.Errorf("error occurred while reading handshake: %s", err)
		return err
	}

	if hnds.P2PVersion < 2 {
		err := fmt.Errorf("outdated peer %s with version %d", ipPort, hnds.P2PVersion)
		return err
	}

	err = conn.View(func(c *ConnData) error {
		// add incoming connections, if possible, to peer list
		if !c.Outgoing && hnds.P2PPort != 0 {
			go func() {
				p.Lock()
				p.AddPeerToList(c.IP(), hnds.P2PPort, false)
				p.Unlock()
			}()
		}
		return nil
	})
	if err != nil {
		err := fmt.Errorf("error occurred while adding peer to list: %s", err)
		Log.Warn(err)
	}

	err = func() error {
		p.RLock()
		defer p.RUnlock()

		for addr, vconn := range p.Connections {
			if addr == ipPort {
				continue
			}
			err := vconn.View(func(v *ConnData) error {
				if v.PeerId == hnds.PeerID && v.Conn.RemoteAddr().String() != ipPort {
					return fmt.Errorf("duplicate ID %x", hnds.PeerID)
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	}()
	if err != nil {
		return err
	}

	err = conn.Update(func(c *ConnData) error {
		c.PeerId = hnds.PeerID

		if c.PeerId == p.PeerId() {
			err := fmt.Errorf("disconnecting from peer %s: connection to self detected", ipPort)
			return err
		}

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

		// update and broadcast peerlist
		go func() {
			err := p.sendPeerList(conn)
			if err != nil {
				Log.Debug(err)
			}
		}()

		Log.Debugf("New connection with ID %x IP %v", c.PeerId, c.Conn.RemoteAddr().String())

		p.NewConnections <- conn

		return nil
	})
	if err != nil {
		return err
	}

	// check if this peer is already connected
	err = func() error {
		p.RLock()
		defer p.RUnlock()
		n := 0
		for i, vconn := range p.Connections {
			if i == ipPort {
				continue
			}
			err := vconn.View(func(v *ConnData) error {
				if v.PeerId == hnds.PeerID {
					n++
					if n >= 2 {
						err := fmt.Errorf("peer with id: %x is already connected - disconnecting it", hnds.PeerID)
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

	for {
		var encData []byte
		var packetType uint16
		var data []byte

		err = func() error {
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
			if err == io.EOF {
				return nil
			}
			return err
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
			return err
		}

		p.onPacketReceived(pack{Type: packetType, Data: data}, conn)
	}
}

func (p *P2P) onPacketReceived(pk pack, c *Connection) {
	c.Update(func(c *ConnData) error {
		c.LastPing = time.Now().Unix()
		return nil
	})

	switch pk.Type {
	case 0: // reserved for future
		return
	case 1: // addPeer
		Log.Debug("AddPeer Packet")
		p.OnAddPeerPacket(pk.Data)
	default:
		p.PacketsIn <- Packet{Data: pk.Data, Type: packet.Type(pk.Type - 2), Conn: c}
	}
}

func (p2 *P2P) OnAddPeerPacket(packetData []byte) error {
	if len(packetData) < 3 {
		return fmt.Errorf("invalid AddPeer packet length")
	}

	d := binary.NewDes(packetData)

	var added bool = false

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
		added = p2.AddPeerToList(address.String(), port, false)
		p2.Unlock()
	}

	if added {
		go func() {
			err := p2.savePeerlist()
			if err != nil {
				Log.Warn(err)
			}
		}()
	}

	return nil
}

// P2P must be locked before calling this
func (p *P2P) AddPeerToList(ip string, port uint16, force bool) bool {
	if port == 0 {
		return false
	}
	if p.Exclusive && !force {
		return force
	}

	shouldAdd := true
	for i, v := range p.KnownPeers {
		if v.IP == ip {
			shouldAdd = false
			if force {
				v.Priority = true
				p.KnownPeers[i] = v
			}
			break
		}
	}
	Log.Net("AddPeerToList", ip, port, "shouldAdd:", shouldAdd)
	if shouldAdd {
		p.KnownPeers = append(p.KnownPeers, KnownPeer{
			IP:       ip,
			Port:     port,
			Type:     PEER_WHITE,
			Priority: force,
		})
	}
	return shouldAdd
}
