package blockchain

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"time"

	"github.com/virel-project/virel-blockchain/v2/adb"
	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/binary"
	"github.com/virel-project/virel-blockchain/v2/block"
	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/rpc"
	"github.com/virel-project/virel-blockchain/v2/stratum"
	"github.com/virel-project/virel-blockchain/v2/stratum/stratumsrv"
	"github.com/virel-project/virel-blockchain/v2/util"
	"github.com/virel-project/virel-blockchain/v2/util/uint128"

	randomvirel "github.com/virel-project/go-randomvirel"
)

func init() {
	randomvirel.InitHash(runtime.NumCPU(), false)
}

func (bc *Blockchain) StartStratum(bindIp string, bindPort uint16) {
	go func() {
		err := bc.Stratum.StartStratum(bindIp, bindPort)
		if err != nil {
			Log.Err(err)
			return
		}
	}()

	for {
		conn := <-bc.Stratum.NewConnections
		go func() {
			err := bc.handleConn(conn)
			if err != nil {
				Log.Warn(err)
				bc.Stratum.Lock()
				bc.Stratum.Kick(conn)
				bc.Stratum.Unlock()
			}
		}()
	}
}

func (bc *Blockchain) NewStratumJob(force bool) {
	// notify the integrated miner about the new job
	bc.MergesMut.Lock()
	bc.mergesUpdated = true
	now := time.Now()
	if !force && now.Sub(bc.lastJob) < 500*time.Millisecond {
		Log.Debug("not sending new job because not forced")
		bc.MergesMut.Unlock()
		return
	}
	bc.lastJob = now
	bc.MergesMut.Unlock()

	var bl *block.Block
	var mindiff uint64
	err := bc.DB.Update(func(tx adb.Txn) (err error) {
		bl, mindiff, err = bc.GetBlockTemplate(tx, address.INVALID_ADDRESS)
		return err
	})
	if err != nil {
		Log.Warn(err)
		return
	}

	Log.Dev("Sending stratum job with diff", mindiff)

	if !config.IS_MASTERCHAIN {
		if len(bl.OtherChains) != 0 {
			Log.Err("bl.OtherChains should be ZERO when outside masterchain")
		}
	}
	bc.Stratum.SendJob(bl, uint128.From64(mindiff))
}

const merge_prefix = "merge-mining:"

func (bc *Blockchain) handleConn(v *stratumsrv.Conn) error {

	var scan *bufio.Scanner

	v.Update(func(c *stratumsrv.ConnData) error {
		scan = bufio.NewScanner(c.Conn)
		c.Conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		return nil
	})

	// read login request
	loginReq := rpc.RequestIn{}
	err := stratumsrv.ReadJSON(&loginReq, scan)
	if err != nil {
		return err
	}

	if loginReq.Method != "login" {
		v.Update(func(c *stratumsrv.ConnData) error {
			return c.WriteJSON(rpc.ResponseOut{
				JsonRpc: "2.0",
				Error: &rpc.Error{
					Code:    -1,
					Message: "invalid method",
				},
			})
		})
		return errors.New("invalid method, expected login")
	}

	loginParams := stratum.LoginRequest{}
	err = json.Unmarshal(loginReq.Params, &loginParams)
	if err != nil {
		v.Update(func(c *stratumsrv.ConnData) error {
			return c.WriteJSON(rpc.ResponseOut{
				JsonRpc: "2.0",
				Error: &rpc.Error{
					Code:    -1,
					Message: "failed to decode login request",
				},
			})
		})
		Log.Warn(err)
		return err
	}
	addr, err := address.FromString(loginParams.Login)
	if err != nil {
		if len(loginParams.Login) > len(merge_prefix) && loginParams.Login[:len(merge_prefix)] == merge_prefix {
			if config.IS_MASTERCHAIN {
				v.Update(func(c *stratumsrv.ConnData) error {
					return c.WriteJSON(rpc.ResponseOut{
						JsonRpc: "2.0",
						Error: &rpc.Error{
							Code:    -1,
							Message: "merge mining: masterchain can't connect to another masterchain",
						},
					})
				})
				return errors.New("merge mining: masterchain can't connect to another masterchain")
			} else {
				addr, err = address.FromString(loginParams.Login[len(merge_prefix):])
				if err != nil {
					Log.Warn(err)
					v.Update(func(c *stratumsrv.ConnData) error {
						return c.WriteJSON(rpc.ResponseOut{
							JsonRpc: "2.0",
							Error: &rpc.Error{
								Code:    -1,
								Message: "invalid merge mining wallet address",
							},
						})
					})
					return err
				}
			}
		} else {
			v.Update(func(c *stratumsrv.ConnData) error {
				return c.WriteJSON(rpc.ResponseOut{
					JsonRpc: "2.0",
					Error: &rpc.Error{
						Code:    -1,
						Message: "invalid wallet address",
					},
				})
			})
			return errors.New("invalid wallet address")
		}
	}
	if addr.Addr == address.INVALID_ADDRESS {
		v.Update(func(c *stratumsrv.ConnData) error {
			return c.WriteJSON(rpc.ResponseOut{
				JsonRpc: "2.0",
				Error: &rpc.Error{
					Code:    -1,
					Message: "invalid wallet address",
				},
			})
		})
		return errors.New("invalid wallet address")
	}
	v.Update(func(c *stratumsrv.ConnData) error {
		c.Address = addr.Addr
		return nil
	})

	if bc.Stratum.LastBlock == nil {
		v.Update(func(c *stratumsrv.ConnData) error {
			return c.WriteJSON(rpc.ResponseOut{
				JsonRpc: "2.0",
				Error: &rpc.Error{
					Code:    -1,
					Message: "block template is not ready yet",
				},
			})
		})
		return errors.New("block template is not ready yet")
	}

	// send login response
	{
		bl := bc.Stratum.LastBlock

		bl.Recipient = addr.Addr

		blob := bl.Commitment().MiningBlob()
		seed := blob.GetSeed()
		jobid := strconv.FormatUint(util.RandomUint64(), 36)
		target := util.GetTargetBytes(bc.Stratum.LastMinDiff)

		if !config.IS_MASTERCHAIN {
			if len(blob.Chains) != 1 {
				Log.Errf("blob.Chains MUST be 1, got %d %x", len(blob.Chains), blob.Chains)
			}
		}

		err = v.Update(func(c *stratumsrv.ConnData) error {
			if len(c.Jobs) >= config.STRATUM_JOBS_HISTORY {
				c.Jobs = c.Jobs[1:]
			}
			c.Jobs = append(c.Jobs, &stratumsrv.MinerJob{
				JobID: jobid,
				Block: bl,
				Seed:  seed,
			})
			return c.WriteJSON(rpc.ResponseOut{
				JsonRpc: "2.0",
				Result: stratum.LoginResponse{
					ID: jobid,
					Job: stratum.Job{
						Algo:     "rx/vrl",
						Blob:     blob.Serialize(),
						JobID:    jobid,
						Target:   target,
						SeedHash: seed[:],
						Height:   bl.Height,
					},
					Extensions: []string{"algo", "keepalive"},
					Status:     "OK",
				},
				Id: loginReq.Id,
			})
		})
		if err != nil {
			Log.Warn(err)
			return err
		}
	}

	for {
		v.View(func(c *stratumsrv.ConnData) error {
			c.Conn.SetReadDeadline(time.Now().Add(config.STRATUM_READ_TIMEOUT))
			return nil
		})
		req := rpc.RequestIn{}
		err = stratumsrv.ReadJSON(&req, scan)
		if err != nil {
			return err
		}

		switch req.Method {
		case "submit":
			t0 := time.Now()
			err := func() error {
				params := stratum.SubmitRequest{}
				err := json.Unmarshal(req.Params, &params)
				if err != nil {
					v.WriteJSON(rpc.ResponseOut{
						JsonRpc: "2.0",
						Error: &rpc.Error{
							Code:    -1,
							Message: "failed to unmarshal json",
						},
						Id: req.Id,
					})
					Log.Warn(err)
					return err
				}
				nonceBin, err := hex.DecodeString(params.Nonce)
				if err != nil {
					v.WriteJSON(rpc.ResponseOut{
						JsonRpc: "2.0",
						Error: &rpc.Error{
							Code:    -1,
							Message: "malformed job",
						},
						Id: req.Id,
					})
					return err
				}
				nonce := binary.LittleEndian.Uint32(nonceBin)

				var job *stratumsrv.MinerJob
				v.View(func(c *stratumsrv.ConnData) error {
					for _, j := range c.Jobs {
						if j.JobID == params.JobID {
							job = j
							break
						}
					}
					return nil
				})
				if job == nil {
					Log.Debug("miner submit stale job")
					v.WriteJSON(rpc.ResponseOut{
						JsonRpc: "2.0",
						Error: &rpc.Error{
							Code:    -1,
							Message: "stale job",
						},
						Id: req.Id,
					})
					return nil
				}

				jb := *job.Block

				// accept merge mining solutions via the Blob field if this is not masterchain
				if len(params.Blob) != 0 {
					if config.IS_MASTERCHAIN {
						// Masterchain node cannot accept Merge Mining solutions
						v.WriteJSON(rpc.ResponseOut{
							JsonRpc: "2.0",
							Error: &rpc.Error{
								Code:    -1,
								Message: "masterchain node cannot accept merge mining solutions",
							},
							Id: req.Id,
						})
						return errors.New("masterchain node cannot accept merge mining solutions")
					}

					blob := block.MiningBlob{}
					err = blob.Deserialize(params.Blob)
					if err != nil {
						v.WriteJSON(rpc.ResponseOut{
							JsonRpc: "2.0",
							Error: &rpc.Error{
								Code:    -1,
								Message: "failed to deserialize mining blob",
							},
							Id: req.Id,
						})
						return fmt.Errorf("failed to deserialize mining blob: %v", err)
					}

					err = jb.SetMiningBlob(blob)
					if err != nil {
						v.WriteJSON(rpc.ResponseOut{
							JsonRpc: "2.0",
							Error: &rpc.Error{
								Code:    -1,
								Message: "failed to set mining blob",
							},
							Id: req.Id,
						})
						return fmt.Errorf("failed to set mining blob: %v", err)
					}
				}

				if len(params.NonceExtra) == 16 {
					jb.NonceExtra = [16]byte(params.NonceExtra)
				}

				var blocks []stratum.FoundBlockInfo

				jb.Nonce = nonce
				commitment := jb.Commitment()
				mb := commitment.MiningBlob()
				powhash := randomvirel.PowHash(mb.GetSeed(), mb.Serialize())
				blocks, err = bc.blockFound(&jb, [16]byte(powhash[16:]))
				if err != nil {
					v.WriteJSON(rpc.ResponseOut{
						JsonRpc: "2.0",
						Error: &rpc.Error{
							Code:    -1,
							Message: "invalid block: " + err.Error(),
						},
						Id: req.Id,
					})
					Log.Debug("stratum miner submit invalid block:", err.Error())
					return nil
				}

				err = v.WriteJSON(rpc.ResponseOut{
					JsonRpc: "2.0",
					Result: stratum.SubmitResponse{
						Status: "OK",
						Blocks: blocks,
					},
					Id: req.Id,
				})
				return err
			}()
			Log.Debug("submit request handled in", time.Since(t0))
			if err != nil {
				return err
			}
		case "keepalived":
			err = v.WriteJSON(rpc.ResponseOut{
				JsonRpc: "2.0",
				Result: stratum.KeepalivedResponse{
					Status: "KEEPALIVED",
				},
				Id: req.Id,
			})
			if err != nil {
				return err
			}
		default:
			Log.Debug("unknown method", req.Method)
		}
	}
}
