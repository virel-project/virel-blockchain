package p2p

import (
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/virel-project/virel-blockchain/v2/binary"
	"github.com/virel-project/virel-blockchain/v2/bitcrypto"
	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/util"
)

func NewConnection(c net.Conn, outgoing bool) *Connection {
	conn := &Connection{
		data: &ConnData{
			Conn:      c,
			Outgoing:  outgoing,
			LastPing:  time.Now().Unix(),
			WriteChan: make(chan pack, config.PARALLEL_BLOCKS_DOWNLOAD*2),
		},
		peerData: &PeerData{},
		Time:     time.Now(),
		alive:    1,
	}
	go conn.Writer()
	return conn
}

func (c *Connection) Writer() {
	for {
		pack, ok := <-c.data.WriteChan
		if !ok {
			return
		}
		err := c.sendPacketLock(pack)
		if err != nil {
			Log.Warn("writer failed:", err)
			c.data.Conn.Close()
			return
		}
	}
}

func (c *Connection) Close() {
	if atomic.SwapInt32(&c.alive, 0) == 1 {
		c.mut.Lock()
		close(c.data.WriteChan)
		c.data.Conn.Close()
		c.mut.Unlock()
	}
}

type Connection struct {
	data     *ConnData
	peerData *PeerData
	Time     time.Time // when the connection was started
	alive    int32     // Use atomic for alive flag
	mut      util.RWMutex
	pdMut    util.Mutex
}

func (c *Connection) View(f func(c *ConnData) error) error {
	c.mut.RLock()
	defer c.mut.RUnlock()

	return f(c.data)
}
func (c *Connection) Update(f func(c *ConnData) error) error {
	c.mut.Lock()
	defer c.mut.Unlock()

	return f(c.data)
}
func (c *Connection) PeerData(f func(d *PeerData)) {
	c.pdMut.Lock()
	defer c.pdMut.Unlock()

	f(c.peerData)
}

type ConnData struct {
	Outgoing      bool             // true if this connection is outgoing, false if it is incoming
	PeerId        bitcrypto.Pubkey // the other peer's ID
	LastPing      int64            // last packet received from peer
	LastOutPacket int64            // when the last packet has been sent to the peer
	Cipher        bitcrypto.Cipher
	WriteChan     chan pack
	Conn          net.Conn
}

// returns the connection IP address (without port)
func (c *ConnData) IP() string {
	return c.Conn.RemoteAddr().(*net.TCPAddr).IP.String()
}

func (c *Connection) sendPacketLock(p pack) error {
	c.mut.Lock()
	c.data.LastOutPacket = time.Now().Unix()
	ser := binary.Ser{}
	ser.AddUint16(p.Type)
	ser.AddFixedByteArray(p.Data)
	data, err := c.data.Cipher.Encrypt(ser.Output())
	c.mut.Unlock()

	if err != nil {
		return err
	}

	ser = binary.Ser{}
	ser.AddUint32(uint32(len(data)))
	ser.AddFixedByteArray(data)

	c.data.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = c.data.Conn.Write(ser.Output())
	return err
}

func (c *Connection) SendPacket(p *Packet) error {
	if atomic.LoadInt32(&c.alive) == 0 {
		return errors.New("connection closed")
	}
	pack := pack{Data: p.Data, Type: uint16(p.Type) + 2}
	select {
	case c.data.WriteChan <- pack:
		return nil
	default:
		return errors.New("write channel full")
	}
}
