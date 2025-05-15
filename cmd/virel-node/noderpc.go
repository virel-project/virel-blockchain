package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"virel-blockchain/address"
	"virel-blockchain/block"
	"virel-blockchain/blockchain"
	"virel-blockchain/config"
	"virel-blockchain/rpc"
	"virel-blockchain/rpc/daemonrpc"
	"virel-blockchain/rpc/rpcserver"
	"virel-blockchain/transaction"
	"virel-blockchain/util"

	"github.com/virel-project/go-randomvirel"
	bolt "go.etcd.io/bbolt"
)

type RpcServer struct {
	HttpSrv *http.Server

	Requests chan *Request
}

type Request struct {
	Req *http.Request
	Res *http.ResponseWriter

	ReqBody rpc.RequestIn
}

const invalidParams = -32602

const internalValidationErr = -32000

const internalReadFailed = -32001

const internalInsertFailed = -32002

const TX_LIST_PAGE_SIZE = 25

func startRpc(bc *blockchain.Blockchain, ip string, port uint16, restricted bool) {
	ratelimitCount := 100_000 // max 100k requests per minute for private RPC
	if restricted {
		ratelimitCount = 5_000 // max 5k requests per minute for public, restricted RPC
	}

	rs := rpcserver.New(fmt.Sprintf("%s:%d", ip, port), rpcserver.Config{
		RateLimit: ratelimitCount,
	})

	rs.Handle("get_block_by_hash", func(c *rpcserver.Context) {
		params := daemonrpc.GetBlockByHashRequest{}

		err := c.GetParams(&params)
		if err != nil {
			return
		}

		var bl *block.Block
		var hash [32]byte
		err = bc.DB.View(func(txn *bolt.Tx) error {
			bl, err = bc.GetBlock(txn, params.Hash)
			if err != nil {
				return err
			}
			topo, err := bc.GetTopo(txn, bl.Height)
			if err != nil {
				return err
			}
			hash = bl.Hash()
			if topo != hash {
				return errors.New("block is not included in mainchain")
			}
			return err
		})
		if err != nil {
			Log.Debug(err)
			c.Response(rpc.ResponseOut{
				JsonRpc: "2.0",
				Error: &rpc.Error{
					Code:    internalReadFailed,
					Message: "Block not found",
				},
				Id: c.Body.Id,
			})
			return
		}

		c.Response(rpc.ResponseOut{
			JsonRpc: "2.0",
			Result: daemonrpc.GetBlockResponse{
				Block:  *bl,
				Hash:   hex.EncodeToString(hash[:]),
				Reward: bl.Reward(),
				Miner:  bl.Recipient.String(),
			},
			Id: c.Body.Id,
		})
	})

	rs.Handle("get_transaction", func(c *rpcserver.Context) {
		params := daemonrpc.GetTransactionRequest{}
		err := c.GetParams(&params)
		if err != nil {
			return
		}

		txn, height, err := bc.GetTx(params.Txid)
		if err != nil {
			Log.Debug(err)

			var bl *block.Block
			err = bc.DB.View(func(tx *bolt.Tx) error {
				bl, err = bc.GetBlock(tx, params.Txid)
				return err
			})
			if err != nil {
				c.Response(rpc.ResponseOut{
					JsonRpc: "2.0",
					Error: &rpc.Error{
						Code:    internalReadFailed,
						Message: "transaction not found",
					},
					Id: c.Body.Id,
				})
				return
			}

			c.Response(rpc.ResponseOut{
				JsonRpc: "2.0",
				Result: daemonrpc.GetTransactionResponse{
					Sender: nil,
					Outputs: []transaction.Output{
						{
							Amount:    bl.Reward(),
							Recipient: bl.Recipient,
							Subaddr:   0,
						},
					},
					Fee:       0,
					Nonce:     0,
					Signature: nil,
					Height:    bl.Height,
					Coinbase:  true,
				},
				Id: c.Body.Id,
			})
			return
		}

		integr := address.FromPubKey(txn.Sender).Integrated()

		c.Response(rpc.ResponseOut{
			JsonRpc: "2.0",
			Result: daemonrpc.GetTransactionResponse{
				Sender:      &integr,
				TotalAmount: txn.TotalAmount(),
				Outputs:     txn.Outputs,
				Fee:         txn.Fee,
				Nonce:       txn.Nonce,
				Signature:   txn.Signature[:],
				Height:      height,
				Coinbase:    false,
			},
			Id: c.Body.Id,
		})

	})

	rs.Handle("get_info", func(c *rpcserver.Context) {
		var stats *blockchain.Stats
		var topBl *block.Block
		err := bc.DB.View(func(tx *bolt.Tx) (err error) {
			stats = bc.GetStats(tx)
			topBl, err = bc.GetBlock(tx, stats.TopHash)
			return
		})
		if err != nil {
			Log.Fatal(err)
		}

		supply := block.GetSupplyAtHeight(stats.TopHeight)

		c.Response(rpc.ResponseOut{
			JsonRpc: "2.0",
			Result: daemonrpc.GetInfoResponse{
				Height:            stats.TopHeight,
				TopHash:           stats.TopHash,
				CirculatingSupply: supply,
				MaxSupply:         config.MAX_SUPPLY,
				Coin:              config.COIN,
				Difficulty:        topBl.Difficulty.String(),
				CumulativeDiff:    stats.CumulativeDiff.String(),
				Target:            config.TARGET_BLOCK_TIME,
				BlockReward:       block.Reward(stats.TopHeight),
			},
			Id: c.Body.Id,
		})
	})

	rs.Handle("submit_transaction", func(c *rpcserver.Context) {
		params := daemonrpc.SubmitTransactionRequest{}
		err := c.GetParams(&params)
		if err != nil {
			return
		}

		Log.Debugf("submit_transaction hex: %s", params.Hex)

		tx := &transaction.Transaction{}

		err = tx.Deserialize(params.Hex)
		if err != nil {
			Log.Warn(err)
			c.Response(rpc.ResponseOut{
				JsonRpc: "2.0",
				Error: &rpc.Error{
					Code:    invalidParams,
					Message: "invalid transaction hex data",
				},
				Id: c.Body.Id,
			})
			return
		}

		err = tx.Prevalidate()
		if err != nil {
			Log.Warn(err)
			c.Response(rpc.ResponseOut{
				JsonRpc: "2.0",
				Error: &rpc.Error{
					Code:    internalValidationErr,
					Message: "transaction verification failed",
				},
				Id: c.Body.Id,
			})
			return
		}

		err = bc.DB.Update(func(txn *bolt.Tx) error {
			return bc.AddTransaction(txn, tx, tx.Hash(), true)
		})
		if err != nil {
			Log.Warn(err)
			c.Response(rpc.ResponseOut{
				JsonRpc: "2.0",
				Error: &rpc.Error{
					Code:    internalInsertFailed,
					Message: "failed to add transaction to chain",
				},
				Id: c.Body.Id,
			})
			return
		}

		txhash := tx.Hash()

		c.Response(rpc.ResponseOut{
			JsonRpc: "2.0",
			Result: daemonrpc.SubmitTransactionResponse{
				TXID: util.Hash(txhash),
			},
		})
	})

	rs.Handle("get_address", func(c *rpcserver.Context) {
		params := daemonrpc.GetAddressRequest{}
		err := c.GetParams(&params)
		if err != nil {
			return
		}

		if params.Address.Addr == address.INVALID_ADDRESS {
			c.Response(rpc.ResponseOut{
				JsonRpc: "2.0",
				Error: &rpc.Error{
					Code:    invalidParams,
					Message: "invalid wallet address",
				},
				Id: c.Body.Id,
			})
			return
		}

		result := daemonrpc.GetAddressResponse{}

		err = bc.DB.View(func(tx *bolt.Tx) error {
			state, err := bc.GetState(tx, params.Address.Addr)
			if err != nil {
				Log.Debug(err)
				return err
			} else {
				result.Balance = state.Balance
				result.LastNonce = state.LastNonce
				result.LastIncoming = state.LastIncoming
			}

			stats := bc.GetStats(tx)
			result.Height = stats.TopHeight

			return nil
		})
		if err != nil {
			Log.Debug("wallet not found:", err)
		}

		result.MempoolBalance = result.Balance
		result.MempoolNonce = result.LastNonce

		var mem *blockchain.Mempool
		bc.DB.View(func(tx *bolt.Tx) error {
			mem = bc.GetMempool(tx)
			return nil
		})
		for _, v := range mem.Entries {
			if v.Sender == params.Address.Addr || slices.ContainsFunc(v.Outputs, func(e transaction.Output) bool { return e.Recipient == params.Address.Addr }) {
				Log.Devf("adding txn %x", v.TXID)
				txn, _, err := bc.GetTx(v.TXID)
				if err != nil {
					Log.Err(err)
					return
				}
				if v.Sender == params.Address.Addr {
					result.MempoolBalance -= txn.TotalAmount()
					result.MempoolNonce++
				}

				for _, out := range v.Outputs {
					if out.Recipient == params.Address.Addr {
						result.MempoolBalance += out.Amount
					}
				}

			}
		}

		c.Response(rpc.ResponseOut{
			JsonRpc: "2.0",
			Result:  result,
			Id:      c.Body.Id,
		})
	})

	rs.Handle("get_tx_list", func(c *rpcserver.Context) {
		params := daemonrpc.GetTxListRequest{}
		err := c.GetParams(&params)
		if err != nil {
			return
		}

		err = bc.DB.View(func(tx *bolt.Tx) error {
			// TODO: test if pagination works correctly
			txType := params.TransferType
			var startNum uint64
			var endNum uint64
			page := params.Page
			var getTopoFunc = bc.GetTxTopoInc
			switch txType {
			case "incoming":
				s, err := bc.GetState(tx, params.Address.Addr)
				if err != nil {
					return err
				}
				startNum = s.LastIncoming
			case "outgoing":
				s, err := bc.GetState(tx, params.Address.Addr)
				if err != nil {
					return err
				}
				startNum = s.LastNonce
				getTopoFunc = bc.GetTxTopoOut
			default:
				c.Response(rpc.ResponseOut{
					JsonRpc: "2.0",
					Error: &rpc.Error{
						Code:    invalidParams,
						Message: "invalid transfer_type received, must be incoming or outgoing",
					},
					Id: c.Body.Id,
				})
				return nil
			}

			sn := startNum
			if sn > 1 {
				sn -= 2
			}
			maxPage := sn / TX_LIST_PAGE_SIZE

			if page*TX_LIST_PAGE_SIZE >= startNum {
				page = startNum / TX_LIST_PAGE_SIZE
			}

			startNum -= page * TX_LIST_PAGE_SIZE
			if startNum > TX_LIST_PAGE_SIZE {
				endNum = startNum - TX_LIST_PAGE_SIZE
			} else {
				endNum = 1
			}
			endNum = min(startNum, endNum)

			if startNum == 0 {
				c.Response(rpc.ResponseOut{
					JsonRpc: "2.0",
					Result: daemonrpc.GetTxListResponse{
						Transactions: []util.Hash{},
						MaxPage:      maxPage,
					},
					Id: c.Body.Id,
				})
				return nil
			}

			list := make([]util.Hash, 0, startNum-endNum)

			for i := endNum; i <= startNum; i++ {
				h, err := getTopoFunc(tx, params.Address.Addr, i)
				if err != nil {
					Log.Warn(err)
					return err
				}
				Log.Devf("new hash for list: %x", h)
				list = append(list, h)
			}

			c.Response(rpc.ResponseOut{
				JsonRpc: "2.0",
				Result: daemonrpc.GetTxListResponse{
					Transactions: list,
					MaxPage:      maxPage,
				},
				Id: c.Body.Id,
			})
			return nil
		})
		if err != nil {
			Log.Debug(err)
			c.Response(rpc.ResponseOut{
				JsonRpc: "2.0",
				Error: &rpc.Error{
					Code:    internalReadFailed,
					Message: "address not in state",
				},
				Id: c.Body.Id,
			})
		}

	})

	rs.Handle("get_block_by_height", func(c *rpcserver.Context) {
		params := daemonrpc.GetBlockByHeightRequest{}

		err := c.GetParams(&params)
		if err != nil {
			return
		}

		var bl *block.Block
		err = bc.DB.View(func(tx *bolt.Tx) (err error) {
			bl, err = bc.GetBlockByHeight(tx, params.Height)
			return
		})
		if err != nil {
			Log.Debug(err)
			c.Response(rpc.ResponseOut{
				JsonRpc: "2.0",
				Error: &rpc.Error{
					Code:    internalReadFailed,
					Message: "block not found",
				},
				Id: c.Body.Id,
			})
			return
		}

		c.Response(rpc.ResponseOut{
			JsonRpc: "2.0",
			Result: daemonrpc.GetBlockResponse{
				Block:  *bl,
				Hash:   bl.Hash().String(),
				Reward: bl.Reward(),
				Miner:  bl.Recipient.String(),
			},
		})
	})

	if !restricted {
		rs.Handle("calc_pow", func(c *rpcserver.Context) {
			params := daemonrpc.CalcPowRequest{}

			err := c.GetParams(&params)
			if err != nil {
				return
			}

			hash := randomvirel.PowHash(randomvirel.Seed(params.SeedHash), params.Blob)
			c.Response(rpc.ResponseOut{
				JsonRpc: "2.0",
				Result: daemonrpc.CalcPowResponse{
					Hash: hash,
				},
			})
		})
	}
}
