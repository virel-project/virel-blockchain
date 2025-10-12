package main

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/virel-project/virel-blockchain/v3/adb"
	"github.com/virel-project/virel-blockchain/v3/address"
	"github.com/virel-project/virel-blockchain/v3/bitcrypto"
	"github.com/virel-project/virel-blockchain/v3/block"
	"github.com/virel-project/virel-blockchain/v3/blockchain"
	"github.com/virel-project/virel-blockchain/v3/chaintype"
	"github.com/virel-project/virel-blockchain/v3/config"
	"github.com/virel-project/virel-blockchain/v3/p2p/packet"
	"github.com/virel-project/virel-blockchain/v3/rpc"
	"github.com/virel-project/virel-blockchain/v3/rpc/daemonrpc"
	"github.com/virel-project/virel-blockchain/v3/rpc/rpcserver"
	"github.com/virel-project/virel-blockchain/v3/transaction"
	"github.com/virel-project/virel-blockchain/v3/util"

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

		outputs := bl.CoinbaseTransaction(bl.Reward())
		minerReward := uint64(0)
		stakerReward := uint64(0)
		governanceReward := uint64(0)

		for _, v := range outputs {
			switch v.Type {
			case transaction.OUT_COINBASE_POW:
				minerReward += v.Amount
			case transaction.OUT_COINBASE_POS:
				stakerReward += v.Amount
			case transaction.OUT_COINBASE_DEV:
				governanceReward += v.Amount
			}
		}

		res := daemonrpc.GetBlockResponse{
			Block:            *bl,
			Hash:             bl.Hash().String(),
			TotalReward:      bl.Reward(),
			MinerReward:      minerReward,
			StakerReward:     stakerReward,
			GovernanceReward: governanceReward,
			Miner:            bl.Recipient.String(),
		}
		if bl.Version > 0 {
			res.Delegate = address.NewDelegateAddress(bl.DelegateId)
			res.NextDelegate = address.NewDelegateAddress(bl.NextDelegateId)
		}

		c.SuccessResponse(res)
	})

	rs.Handle("get_block_by_height", func(c *rpcserver.Context) {
		params := daemonrpc.GetBlockByHeightRequest{}
		err := c.GetParams(&params)
		if err != nil {
			return
		}

		var bl *block.Block
		err = bc.DB.View(func(txn adb.Txn) (err error) {
			bl, err = bc.GetBlockByHeight(txn, params.Height)
			if err != nil {
				return err
			}

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

		outputs := bl.CoinbaseTransaction(bl.Reward())
		minerReward := uint64(0)
		stakerReward := uint64(0)
		governanceReward := uint64(0)

		for _, v := range outputs {
			switch v.Type {
			case transaction.OUT_COINBASE_POW:
				minerReward += v.Amount
			case transaction.OUT_COINBASE_POS:
				stakerReward += v.Amount
			case transaction.OUT_COINBASE_DEV:
				governanceReward += v.Amount
			}
		}

		res := daemonrpc.GetBlockResponse{
			Block:            *bl,
			Hash:             bl.Hash().String(),
			TotalReward:      bl.Reward(),
			MinerReward:      minerReward,
			StakerReward:     stakerReward,
			GovernanceReward: governanceReward,
			Miner:            bl.Recipient.String(),
		}
		if bl.Version > 0 {
			res.Delegate = address.NewDelegateAddress(bl.DelegateId)
			res.NextDelegate = address.NewDelegateAddress(bl.NextDelegateId)
		}

		c.SuccessResponse(res)
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
			stats := bc.GetStats(txn)
			tx, height, err = bc.GetTx(txn, params.Txid, stats.TopHeight)
			return
		})
		if err != nil {
			Log.Debug(err)

			var bl *block.Block
			var reward uint64
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

				reward = bl.Reward()
				for _, v := range bl.Transactions {
					txn, _, err := bc.GetTx(txn, v, bl.Height)
					if err != nil {
						Log.Warn(err)
					} else {
						reward += txn.Fee
					}
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

			cout := bl.CoinbaseTransaction(reward)
			out := make([]transaction.StateOutput, len(cout))
			for i, v := range cout {
				out[i] = transaction.StateOutput{
					Recipient: v.Recipient,
					PaymentId: 0,
					Amount:    v.Amount,
					Type:      v.Type,
				}
			}

			c.SuccessResponse(daemonrpc.GetTransactionResponse{
				Signer:      nil,
				TotalAmount: reward,
				Inputs:      []transaction.StateInput{},
				Outputs:     out,
				Fee:         0,
				Nonce:       0,
				Signature:   nil,
				Height:      bl.Height,
				Coinbase:    true,
			})
			return
		}

		signer := address.FromPubKey(tx.Signer).Integrated()

		amount, err := tx.TotalAmount()
		if err != nil {
			Log.Err(err)
			c.ErrorResponse(&rpc.Error{
				Code:    internalReadFailed,
				Message: "Invalid outputs in TX",
			})
			return
		}

		c.SuccessResponse(daemonrpc.GetTransactionResponse{
			Signer:      &signer,
			TotalAmount: amount,
			Inputs:      tx.Data.StateInputs(tx, signer.Addr),
			Outputs:     tx.Data.StateOutputs(tx, signer.Addr),
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
		var burnState *chaintype.State
		err := bc.DB.View(func(txn adb.Txn) (err error) {
			stats = bc.GetStats(txn)
			topBl, err = bc.GetBlock(txn, stats.TopHash)
			if err != nil {
				return err
			}
			burnState, err = bc.GetState(txn, address.INVALID_ADDRESS)
			if err != nil {
				burnState = &chaintype.State{}
				return nil
			}
			return err
		})
		if err != nil {
			Log.Fatal(err)
		}

		supply := block.GetSupplyAtHeight(stats.TopHeight)

		c.SuccessResponse(daemonrpc.GetInfoResponse{
			Height:            stats.TopHeight,
			TopHash:           stats.TopHash,
			TotalSupply:       supply,
			CirculatingSupply: supply - burnState.Balance,
			Stake:             stats.StakedAmount,
			MaxSupply:         config.MAX_SUPPLY,
			SupplyCap:         config.MAX_SUPPLY - burnState.Balance,
			Burned:            burnState.Balance,
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

		var stats *blockchain.Stats
		bc.DB.View(func(txn adb.Txn) error {
			stats = bc.GetStats(txn)
			return nil
		})

		tx := &transaction.Transaction{}
		err = tx.Deserialize(params.Hex, stats.TopHeight+1 >= config.HARDFORK_V2_HEIGHT)
		if err != nil {
			Log.Warn(err)
			c.ErrorResponse(&rpc.Error{
				Code:    invalidParams,
				Message: "invalid transaction hex data",
			})
			return
		}

		err = tx.Prevalidate(stats.TopHeight + 1)
		if err != nil {
			Log.Warn(err)
			c.ErrorResponse(&rpc.Error{
				Code:    internalValidationErr,
				Message: "transaction verification failed",
			})
			return
		}

		err = bc.DB.Update(func(txn adb.Txn) error {
			return bc.AddTransaction(txn, tx, tx.Hash(), true, stats.TopHeight+1)
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

		addr, err := address.FromString(params.Address)
		if err != nil {
			c.ErrorResponse(&rpc.Error{
				Code:    invalidParams,
				Message: "invalid wallet address",
			})
			return
		}

		result := daemonrpc.GetAddressResponse{}

		err = bc.DB.View(func(tx adb.Txn) error {
			state, err := bc.GetState(tx, addr.Addr)
			if err != nil {
				Log.Debug(err)
				return err
			} else {
				result.Balance = state.Balance
				result.LastNonce = state.LastNonce
				result.LastIncoming = state.LastIncoming
				result.DelegateId = state.DelegateId
				result.TotalStaked = state.TotalStaked
				result.TotalUnstaked = state.TotalUnstaked
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
				if slices.ContainsFunc(v.Inputs, func(e transaction.StateInput) bool { return e.Sender == addr.Addr }) || slices.ContainsFunc(v.Outputs, func(e transaction.Output) bool { return e.Recipient == addr.Addr }) {
					for _, inp := range v.Inputs {
						if inp.Sender == addr.Addr {
							result.MempoolBalance -= inp.Amount
							result.MempoolNonce++
							// NOTE: Outgoing mempool transactions are removed from the displayed balance immediately,
							// as we consider them more trustworthy (to avoid double sending money by mistake)
							if inp.Amount > result.Balance {
								Log.Warnf("invalid mempool transaction %x", v.TXID)
							} else {
								result.Balance -= inp.Amount
								result.LastNonce++
							}
						}
					}

					for _, out := range v.Outputs {
						if out.Recipient == addr.Addr {
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

			// Calculate endNum correctly to get exactly TX_LIST_PAGE_SIZE transactions
			endNum := uint64(max(int64(startNum)-TX_LIST_PAGE_SIZE, 0))

			// Handle case where startNum < endNum after adjustment
			if startNum < endNum {
				c.SuccessResponse(daemonrpc.GetTxListResponse{
					Transactions: []util.Hash{},
					MaxPage:      maxPage,
				})
				return nil
			}

			list := make([]util.Hash, 0, startNum-endNum+1)
			for i := endNum; i < startNum; i++ {
				h, err := getTopoFunc(tx, params.Address.Addr, i+1)
				if err != nil {
					Log.Warn(err)
					return err
				}
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
			c.SuccessResponse(daemonrpc.GetTxListResponse{
				Transactions: []util.Hash{},
				MaxPage:      0,
			})
		}
	})

	rs.Handle("validate_address", func(c *rpcserver.Context) {
		params := daemonrpc.ValidateAddressRequest{}
		err := c.GetParams(&params)
		if err != nil {
			return
		}

		addr, err := address.FromString(params.Address)
		if err != nil {
			c.SuccessResponse(daemonrpc.ValidateAddressResponse{
				Address:      params.Address,
				Valid:        false,
				ErrorMessage: err.Error(),
			})
			return
		}

		if addr.Addr == address.INVALID_ADDRESS {
			c.SuccessResponse(daemonrpc.ValidateAddressResponse{
				Address:      params.Address,
				Valid:        false,
				ErrorMessage: "address is the zero address",
			})
			return
		}

		c.SuccessResponse(daemonrpc.ValidateAddressResponse{
			Address:     params.Address,
			Valid:       true,
			MainAddress: addr.Addr.String(),
			PaymentId:   addr.PaymentId,
		})
	})

	rs.Handle("submit_stake_signature", func(c *rpcserver.Context) {
		params := daemonrpc.SubmitStakeSignatureRequest{}
		err := c.GetParams(&params)
		if err != nil {
			return
		}

		if len(params.Hash) != 32 || len(params.Signature) != bitcrypto.SIGNATURE_SIZE {
			c.ErrorResponse(&rpc.Error{
				Code:    -1,
				Message: "invalid hash or signature length",
			})
			return
		}

		hash := util.Hash(params.Hash)
		sig := bitcrypto.Signature(params.Signature)

		err = bc.HandleStakeSignature(&packet.PacketStakeSignature{
			DelegateId: params.DelegateId,
			Hash:       hash,
			Signature:  sig,
		})

		if err != nil {
			c.SuccessResponse(daemonrpc.SubmitStakeSignatureResponse{
				Accepted:     false,
				ErrorMessage: err.Error(),
			})
			return
		}

		c.SuccessResponse(daemonrpc.SubmitStakeSignatureResponse{
			Accepted: true,
		})
	})

	rs.Handle("get_delegate", func(c *rpcserver.Context) {
		params := daemonrpc.GetDelegateRequest{}
		err := c.GetParams(&params)
		if err != nil {
			return
		}

		if params.DelegateId == 0 {
			if !strings.HasPrefix(params.DelegateAddress, config.DELEGATE_ADDRESS_PREFIX) {
				c.ErrorResponse(&rpc.Error{
					Code:    -1,
					Message: "invalid delegate address prefix",
				})
				return
			}

			params.DelegateAddress = strings.TrimPrefix(params.DelegateAddress, config.DELEGATE_ADDRESS_PREFIX)

			params.DelegateId, err = strconv.ParseUint(params.DelegateAddress, 10, 64)
			if err != nil {
				Log.Debug(err)
				c.ErrorResponse(&rpc.Error{
					Code:    -1,
					Message: "invalid delegate address",
				})
				return
			}
		}

		var delegate *chaintype.Delegate
		err = bc.DB.View(func(txn adb.Txn) error {
			delegate, err = bc.GetDelegate(txn, params.DelegateId)
			return err
		})
		if err != nil {
			Log.Warn(err)
			c.ErrorResponse(&rpc.Error{
				Code:    internalReadFailed,
				Message: "failed to get delegate",
			})
			return
		}

		c.SuccessResponse(daemonrpc.GetDelegateResponse{
			Id:          params.DelegateId,
			Address:     address.NewDelegateAddress(params.DelegateId),
			Owner:       delegate.OwnerAddress(),
			TotalAmount: delegate.TotalAmount(),
			Name:        string(delegate.Name),
			Funds:       delegate.Funds,
		})
	})

	if !restricted {
		rs.Handle("get_rich_list", func(c *rpcserver.Context) {
			const COUNT = 100

			resp := daemonrpc.RichListResponse{
				Richest: make([]daemonrpc.StateInfo, 0, COUNT),
			}

			err := bc.DB.View(func(txn adb.Txn) error {
				return txn.ForEach(bc.Index.State, func(k, v []byte) error {
					if len(k) != address.SIZE {
						return errors.New("invalid address length")
					}
					addr := address.Address(k)
					if addr.IsDelegate() {
						return nil
					}

					st := &chaintype.State{}
					err := st.Deserialize(v)
					if err != nil {
						return err
					}

					var staked uint64
					if st.DelegateId != 0 {
						deleg, err := bc.GetDelegate(txn, st.DelegateId)
						if err != nil {
							return err
						}
						for _, v := range deleg.Funds {
							if v.Owner == addr {
								staked = v.Amount
								break
							}
						}
					}

					if len(resp.Richest) < COUNT {
						resp.Richest = append(resp.Richest, daemonrpc.StateInfo{
							Address: addr.String(),
							State:   st,
							Staked:  staked,
						})
						// Sort the slice when we reach the count
						if len(resp.Richest) == COUNT {
							sort.Slice(resp.Richest, func(i, j int) bool {
								return resp.Richest[i].State.Balance > resp.Richest[j].State.Balance
							})
						}
					} else {
						// Check if this address has a higher balance than the smallest in our list
						if st.Balance+staked > resp.Richest[COUNT-1].Total() {
							// Replace the smallest balance
							resp.Richest[COUNT-1] = daemonrpc.StateInfo{
								Address: addr.String(),
								State:   st,
								Staked:  staked,
							}

							// Re-sort the slice to maintain order
							sort.Slice(resp.Richest, func(i, j int) bool {
								return resp.Richest[i].Total() > resp.Richest[j].Total()
							})
						}
					}

					return nil
				})
			})
			if err != nil {
				Log.Err(err)
				c.ErrorResponse(&rpc.Error{
					Code: internalReadFailed,
				})
				return
			}

			slices.SortStableFunc(resp.Richest, func(a, b daemonrpc.StateInfo) int {
				return int(int64(b.Total()) - int64(a.Total()))
			})

			c.SuccessResponse(resp)
		})

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
