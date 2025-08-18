package stratumsrv

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/virel-project/virel-blockchain/address"
	"github.com/virel-project/virel-blockchain/block"
	"github.com/virel-project/virel-blockchain/config"
	"github.com/virel-project/virel-blockchain/logger"
	"github.com/virel-project/virel-blockchain/rpc"
	"github.com/virel-project/virel-blockchain/stratum"
	"github.com/virel-project/virel-blockchain/util"
	"github.com/virel-project/virel-blockchain/util/uint128"

	"github.com/virel-project/go-randomvirel"
)

var Log *logger.Log = logger.DiscardLog

type Server struct {
	LastBlock      *block.Block
	LastMinDiff    uint128.Uint128
	conns          map[string]*Conn
	NewConnections chan *Conn
	util.RWMutex
}

type Conn struct {
	data *ConnData
	mut  util.RWMutex
}

func (c *Conn) View(f func(c *ConnData) error) error {
	c.mut.RLock()
	defer c.mut.RUnlock()

	return f(c.data)
}
func (c *Conn) Update(f func(c *ConnData) error) error {
	c.mut.Lock()
	defer c.mut.Unlock()

	return f(c.data)
}

func (c *Conn) WriteJSON(dat any) error {
	return c.Update(func(c *ConnData) error {
		return c.WriteJSON(dat)
	})
}

type ConnData struct {
	Conn    net.Conn
	Address address.Address
	Jobs    []*MinerJob
}

type MinerJob struct {
	JobID string
	Block *block.Block
	Seed  randomvirel.Seed
}

func (s *Server) StartStratum(ip string, port uint16) error {
	listener, err := net.Listen("tcp", ip+":"+strconv.FormatUint(uint64(port), 10))
	if err != nil {
		return err
	}
	defer listener.Close()

	s.Lock()
	s.conns = make(map[string]*Conn)
	s.Unlock()

	Log.Infof("Stratum server listening: %s:%d", ip, port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		Log.Info("Stratum conn:", conn.RemoteAddr())

		c := &Conn{
			data: &ConnData{
				Conn: conn,
				Jobs: make([]*MinerJob, 0, config.STRATUM_JOBS_HISTORY),
			},
		}
		s.Lock()
		s.conns[conn.RemoteAddr().String()] = c
		s.Unlock()
		s.NewConnections <- c
	}
}

// Server MUST be locked before calling this
func (s *Server) Kick(c *Conn) {
	c.Update(func(c *ConnData) error {
		c.Conn.Close()
		ip := c.Conn.RemoteAddr().String()
		if s.conns[ip] != nil {
			delete(s.conns, ip)
		}
		return nil
	})
}

func (s *Server) SendJob(bl *block.Block, diff uint128.Uint128) {
	s.Lock()
	defer s.Unlock()

	s.LastBlock = bl
	s.LastMinDiff = diff
	for _, v := range s.conns {
		go func() {
			err := v.Update(func(c *ConnData) error {
				if c.Address == address.INVALID_ADDRESS {
					Log.Debug("address is invalid (no handshake yet)")
					return nil
				}

				bl.Recipient = c.Address

				blob := bl.Commitment().MiningBlob()
				seed := blob.GetSeed()
				jobid := strconv.FormatUint(util.RandomUint64(), 36)
				target := util.GetTargetBytes(diff)

				if !config.IS_MASTERCHAIN {
					if len(blob.Chains) != 1 {
						Log.Err("blob.Chains must be 1! got", len(blob.Chains))
					}
				}

				if len(c.Jobs) >= config.STRATUM_JOBS_HISTORY {
					c.Jobs = c.Jobs[1:]
				}
				c.Jobs = append(c.Jobs, &MinerJob{
					JobID: jobid,
					Block: bl,
					Seed:  seed,
				})

				Log.Debug("sending job to stratum connection", c.Conn.RemoteAddr())
				go func() {
					err := v.WriteJSON(rpc.NoreplyRequest{
						JsonRpc: "2.0",
						Method:  "job",
						Params: stratum.Job{
							Algo:     "rx/vrl",
							Blob:     blob.Serialize(),
							JobID:    jobid,
							Target:   target,
							SeedHash: seed[:],
							Height:   bl.Height,
						},
					})
					if err != nil {
						Log.Debug("error sending job to peer:", err)
					}
				}()
				return nil
			})
			if err != nil {
				s.Kick(v)
			}
		}()
	}
}

func ReadJSON(r interface{}, scan *bufio.Scanner) error {
	ok := scan.Scan()
	if !ok {
		err := scan.Err()
		if err == nil {
			return io.EOF
		}
		return scan.Err()
	}

	data := scan.Bytes()

	Log.NetDev("Stratum server ReadJSON: " + string(data))
	err := json.Unmarshal(data, r)
	if err != nil {
		Log.Warn("failed to unmarshal json:", err)
		return err
	}
	return nil
}

func (c *ConnData) WriteJSON(dat any) error {
	m, err := json.Marshal(dat)
	if err != nil {
		return err
	}

	Log.Net("Stratum server WriteJSON: " + string(m))

	c.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = c.Conn.Write(append(m, '\n'))
	return err
}
