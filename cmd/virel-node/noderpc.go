package main

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"slices"

	"github.com/virel-project/virel-blockchain/adb"
	"github.com/virel-project/virel-blockchain/address"
	"github.com/virel-project/virel-blockchain/block"
	"github.com/virel-project/virel-blockchain/blockchain"
	"github.com/virel-project/virel-blockchain/config"
	"github.com/virel-project/virel-blockchain/rpc"
	"github.com/virel-project/virel-blockchain/rpc/daemonrpc"
	"github.com/virel-project/virel-blockchain/rpc/rpcserver"
	"github.com/virel-project/virel-blockchain/transaction"
	"github.com/virel-project/virel-blockchain/util"

	"github.com/virel-project/go-randomvirel"
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

var err_orphan = fmt.Errorf("coinbase tx is orphan")
var err_block_orphan = fmt.Errorf("block is orphan")

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
		err = bc.DB.View(func(txn adb.Txn) error {
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
				return err_block_orphan
			}
			return err
		})
		if err != nil {
			if err == err_block_orphan {
				c.ErrorResponse(&rpc.Error{
					Code:    internalReadFailed,
					Message: "block is orphan",
				})
				return
			}
			Log.Debug(err)
			c.ErrorResponse(&rpc.Error{
				Code:    internalReadFailed,
				Message: "Block not found",
			})
			return
		}

		c.SuccessResponse(daemonrpc.GetBlockResponse{
			Block:       *bl,
			Hash:        hex.EncodeToString(hash[:]),
			TotalReward: bl.Reward(),
			MinerReward: bl.Reward() * (100 - config.BLOCK_REWARD_FEE_PERCENT) / 100,
			Miner:       bl.Recipient.String(),
		})
	})

	rs.Handle("get_transaction", func(c *rpcserver.Context) {
		params := daemonrpc.GetTransactionRequest{}
		err := c.GetParams(&params)
		if err != nil {
			return
		}

		var tx *transaction.Transaction
		var height uint64

		err = bc.DB.View(func(txn adb.Txn) (err error) {
			tx, height, err = bc.GetTx(txn, params.Txid)
			return
		})
		if err != nil {
			Log.Debug(err)

			var bl *block.Block
			err = bc.DB.View(func(txn adb.Txn) error {
				bl, err = bc.GetBlock(txn, params.Txid)
				if err != nil {
					return err
				}
				topoHash, err := bc.GetTopo(txn, bl.Height)
				if err != nil {
					return err
				}
				if topoHash != params.Txid {
					return err_orphan
				}
				return nil
			})
			if err != nil {
				if err == err_orphan {
					c.ErrorResponse(&rpc.Error{
						Code:    internalReadFailed,
						Message: "coinbase transaction is orphan",
					})
					return
				}
				c.ErrorResponse(&rpc.Error{
					Code:    internalReadFailed,
					Message: "transaction not found",
				})
				return
			}

			rewardFee := bl.Reward() * config.BLOCK_REWARD_FEE_PERCENT / 100

			c.SuccessResponse(daemonrpc.GetTransactionResponse{
				Sender:      nil,
				TotalAmount: bl.Reward(),
				Outputs: []transaction.Output{
					{
						Amount:    bl.Reward() - rewardFee,
						Recipient: bl.Recipient,
						PaymentId: 0,
					},
				},
				Fee:       rewardFee,
				Nonce:     0,
				Signature: nil,
				Height:    bl.Height,
				Coinbase:  true,
			})
			return
		}

		integr := address.FromPubKey(tx.Sender).Integrated()

		c.SuccessResponse(daemonrpc.GetTransactionResponse{
			Sender:      &integr,
			TotalAmount: tx.TotalAmount(),
			Outputs:     tx.Outputs,
			Fee:         tx.Fee,
			Nonce:       tx.Nonce,
			Signature:   tx.Signature[:],
			Height:      height,
			Coinbase:    false,
			VirtualSize: tx.GetVirtualSize(),
		})
	})

	rs.Handle("get_info", func(c *rpcserver.Context) {
		var stats *blockchain.Stats
		var topBl *block.Block
		err := bc.DB.View(func(tx adb.Txn) (err error) {
			stats = bc.GetStats(tx)
			topBl, err = bc.GetBlock(tx, stats.TopHash)
			return
		})
		if err != nil {
			Log.Fatal(err)
		}

		supply := block.GetSupplyAtHeight(stats.TopHeight)

		c.SuccessResponse(daemonrpc.GetInfoResponse{
			Height:            stats.TopHeight,
			TopHash:           stats.TopHash,
			CirculatingSupply: supply,
			MaxSupply:         config.MAX_SUPPLY,
			Coin:              config.COIN,
			Difficulty:        topBl.Difficulty.String(),
			CumulativeDiff:    stats.CumulativeDiff.String(),
			Target:            config.TARGET_BLOCK_TIME,
			BlockReward:       block.Reward(stats.TopHeight),
                        Version:           fmt.Sprintf("%d.%d.%d", config.VERSION_MAJOR, config.VERSION_MINOR, config.VERSION_PATCH),
                        Connections:       len(bc.P2P.Connections),
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
			c.ErrorResponse(&rpc.Error{
				Code:    invalidParams,
				Message: "invalid transaction hex data",
			})
			return
		}

		err = tx.Prevalidate()
		if err != nil {
			Log.Warn(err)
			c.ErrorResponse(&rpc.Error{
				Code:    internalValidationErr,
				Message: "transaction verification failed",
			})
			return
		}

		err = bc.DB.Update(func(txn adb.Txn) error {
			return bc.AddTransaction(txn, tx, tx.Hash(), true)
		})
		if err != nil {
			Log.Warn(err)
			c.ErrorResponse(&rpc.Error{
				Code:    internalInsertFailed,
				Message: "failed to add transaction to chain",
			})
			return
		}

		txhash := tx.Hash()

		c.SuccessResponse(daemonrpc.SubmitTransactionResponse{
			TXID: util.Hash(txhash),
		})
	})

	rs.Handle("get_address", func(c *rpcserver.Context) {
		params := daemonrpc.GetAddressRequest{}
		err := c.GetParams(&params)
		if err != nil {
			return
		}

		if params.Address.Addr == address.INVALID_ADDRESS {
			c.ErrorResponse(&rpc.Error{
				Code:    invalidParams,
				Message: "invalid wallet address",
			})
			return
		}

		result := daemonrpc.GetAddressResponse{}

		err = bc.DB.View(func(tx adb.Txn) error {
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
		bc.DB.View(func(tx adb.Txn) error {
			mem = bc.GetMempool(tx)
			return nil
		})

		err = bc.DB.View(func(txn adb.Txn) (err error) {
			for _, v := range mem.Entries {
				if v.Sender == params.Address.Addr || slices.ContainsFunc(v.Outputs, func(e transaction.Output) bool { return e.Recipient == params.Address.Addr }) {
					Log.Devf("adding txn %x", v.TXID)
					txn, _, err := bc.GetTx(txn, v.TXID)
					if err != nil {
						Log.Err(err)
						return err
					}
					if v.Sender == params.Address.Addr {
						result.MempoolBalance -= txn.TotalAmount()
						result.MempoolNonce++
						// NOTE: Outgoing mempool transactions are removed from the displayed balance immediately,
						// as we consider them more trustworthy (to avoid double sending money by mistake)
						result.Balance -= txn.TotalAmount()
						result.LastNonce++
					}

					for _, out := range v.Outputs {
						if out.Recipient == params.Address.Addr {
							result.MempoolBalance += out.Amount
						}
					}

				}
			}
			return
		})
		if err != nil {
			Log.Warn(err)
			c.ErrorResponse(&rpc.Error{
				Code:    internalReadFailed,
				Message: "could not get transactions",
			})
			return
		}

		c.SuccessResponse(result)
	})

	rs.Handle("get_tx_list", func(c *rpcserver.Context) {
		params := daemonrpc.GetTxListRequest{}
		err := c.GetParams(&params)
		if err != nil {
			return
		}

		err = bc.DB.View(func(tx adb.Txn) error {
			txType := params.TransferType
			var startNum uint64
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
				c.ErrorResponse(&rpc.Error{
					Code:    invalidParams,
					Message: "invalid transfer_type received, must be incoming or outgoing",
				})
				return nil
			}

			// Calculate maxPage using correct ceiling division
			var maxPage uint64
			if startNum > 0 {
				maxPage = (startNum - 1) / TX_LIST_PAGE_SIZE
			}

			page := params.Page
			// Ensure requested page doesn't exceed maxPage
			if page > maxPage {
				page = maxPage
			}

			// Adjust startNum for pagination
			startNum -= page * TX_LIST_PAGE_SIZE

			if startNum == 0 {
				c.SuccessResponse(daemonrpc.GetTxListResponse{
					Transactions: []util.Hash{},
					MaxPage:      maxPage,
				})
				return nil
			}

			// Calculate endNum correctly to get exactly TX_LIST_PAGE_SIZE transactions
			endNum := startNum - TX_LIST_PAGE_SIZE + 1
			if endNum < 1 {
				endNum = 1
			}

			// Handle case where startNum < endNum after adjustment
			if startNum < endNum {
				c.SuccessResponse(daemonrpc.GetTxListResponse{
					Transactions: []util.Hash{},
					MaxPage:      maxPage,
				})
				return nil
			}

			list := make([]util.Hash, 0, startNum-endNum+1)
			for i := endNum; i <= startNum; i++ {
				h, err := getTopoFunc(tx, params.Address.Addr, i)
				if err != nil {
					Log.Warn(err)
					return err
				}
				Log.Devf("new hash for list: %x", h)
				list = append(list, h)
			}

			c.SuccessResponse(daemonrpc.GetTxListResponse{
				Transactions: list,
				MaxPage:      maxPage,
			})
			return nil
		})
		if err != nil {
			Log.Debug(err)
			c.ErrorResponse(&rpc.Error{
				Code:    internalReadFailed,
				Message: "address not in state",
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
		err = bc.DB.View(func(tx adb.Txn) (err error) {
			bl, err = bc.GetBlockByHeight(tx, params.Height)
			return
		})
		if err != nil {
			Log.Debug(err)
			c.ErrorResponse(&rpc.Error{
				Code:    internalReadFailed,
				Message: "block not found",
			})
			return
		}

		c.SuccessResponse(daemonrpc.GetBlockResponse{
			Block:       *bl,
			Hash:        bl.Hash().String(),
			TotalReward: bl.Reward(),
			MinerReward: bl.Reward() * (100 - config.BLOCK_REWARD_FEE_PERCENT) / 100,
			Miner:       bl.Recipient.String(),
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
			c.SuccessResponse(daemonrpc.CalcPowResponse{
				Hash: hash,
			})
		})
	}
}
