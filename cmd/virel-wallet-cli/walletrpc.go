package main

import (
	"fmt"
	"net/http"

	"github.com/virel-project/virel-blockchain/rpc"
	"github.com/virel-project/virel-blockchain/rpc/rpcserver"
	"github.com/virel-project/virel-blockchain/rpc/walletrpc"
	"github.com/virel-project/virel-blockchain/transaction"
	"github.com/virel-project/virel-blockchain/util"
	"github.com/virel-project/virel-blockchain/util/ratelimit"
	"github.com/virel-project/virel-blockchain/wallet"
)

type RpcServer struct {
	HttpSrv *http.Server

	Requests  chan *Request
	RateLimit ratelimit.Limit
}

type Request struct {
	Req *http.Request
	Res *http.ResponseWriter

	ReqBody rpc.RequestIn
}

const invalidJson = -32700
const sInvalidJson = "Parse error"

const internalReadFailed = -32001

func startRpcServer(w *wallet.Wallet, ip string, port uint16, auth string) {
	rs := rpcserver.New(fmt.Sprintf("%s:%d", ip, port), rpcserver.Config{
		Restricted:     true,
		Authentication: auth,
		RateLimit:      250,
	})

	rs.Handle("get_balance", func(c *rpcserver.Context) {
		params := walletrpc.GetBalanceRequest{}
		err := c.GetParams(&params)
		if err != nil {
			return
		}
		c.SuccessResponse(walletrpc.GetBalanceResponse{
			Balance:        w.GetBalance(),
			MempoolBalance: w.GetMempoolBalance(),
		})
	})

	rs.Handle("get_history", func(c *rpcserver.Context) {
		params := walletrpc.GetHistoryRequest{}
		err := c.GetParams(&params)
		if err != nil {
			return
		}

		if params.FilterIncomingByPaymentId != 0 && !params.IncludeTxData {
			c.ErrorResponse(&rpc.Error{
				Code:    -2,
				Message: "include_tx_data must be true when using filter_incoming_by_payment_id",
			})
			return
		}

		var inc []walletrpc.TxInfo
		var out []walletrpc.TxInfo

		if params.IncludeIncoming {
			inc = []walletrpc.TxInfo{}

			var page uint64 = 0
			for {
				txlist, err := w.GetTransactions(true, page)
				if err != nil {
					c.ErrorResponse(&rpc.Error{
						Code:    internalReadFailed,
						Message: "failed to get transactions",
					})
					Log.Warn(err)
					return
				}
				for _, tx := range txlist.Transactions {
					txinfo := walletrpc.TxInfo{
						Hash: tx,
					}
					okToAdd := true
					if params.IncludeTxData {
						txres, err := w.GetTransaction(tx)
						if err != nil {
							Log.Warn(err)
						} else {
							txinfo.Data = txres
						}
						if params.FilterIncomingByPaymentId != 0 {
							okToAdd = false
							for _, v := range txres.Outputs {
								if v.Subaddr == params.FilterIncomingByPaymentId {
									okToAdd = true
								}
							}
						}
					}
					if okToAdd {
						inc = append(inc, txinfo)
					}
				}
				if txlist.MaxPage <= page {
					break
				}
			}
		}

		if params.IncludeOutgoing {
			out = []walletrpc.TxInfo{}

			var page uint64 = 0
			for {
				txlist, err := w.GetTransactions(false, page)
				if err != nil {
					c.ErrorResponse(&rpc.Error{
						Code:    internalReadFailed,
						Message: "failed to get transactions",
					})
					Log.Warn(err)
					return
				}
				for _, tx := range txlist.Transactions {
					txinfo := walletrpc.TxInfo{
						Hash: tx,
					}
					if params.IncludeTxData {
						txres, err := w.GetTransaction(tx)
						if err != nil {
							Log.Warn(err)
						} else {
							txinfo.Data = txres
						}
					}
					out = append(out, txinfo)
				}
				if txlist.MaxPage <= page {
					break
				}
			}
		}

		c.SuccessResponse(walletrpc.GetHistoryResponse{
			Incoming: inc,
			Outgoing: out,
		})
	})

	rs.Handle("create_transaction", func(c *rpcserver.Context) {
		params := walletrpc.CreateTransactionRequest{}
		err := c.GetParams(&params)
		if err != nil {
			return
		}

		outs := make([]transaction.Output, len(params.Outputs))
		for i, v := range params.Outputs {
			outs[i].Amount = v.Amount
			outs[i].Recipient = v.Recipient.Addr
			outs[i].Subaddr = v.Recipient.Subaddr
		}
		tx, err := w.Transfer(outs)
		if err != nil {
			Log.Warn(err)
			c.ErrorResponse(&rpc.Error{
				Code:    -1,
				Message: "transfer failed",
			})
			return
		}

		c.SuccessResponse(walletrpc.CreateTransactionResponse{
			TxBlob: tx.Serialize(),
			TXID:   util.Hash(tx.Hash()),
		})
	})

	rs.Handle("submit_transaction", func(c *rpcserver.Context) {
		params := walletrpc.SubmitTransactionRequest{}
		err := c.GetParams(&params)
		if err != nil {
			c.ErrorResponse(&rpc.Error{
				Code:    invalidJson,
				Message: sInvalidJson,
			})
			return
		}

		tx := transaction.Transaction{}
		err = tx.Deserialize(params.TxBlob)
		if err != nil {
			Log.Warn(err)
			c.ErrorResponse(&rpc.Error{
				Code:    -1,
				Message: "could not deserialize transaction",
			})
			return
		}

		submitRes, err := w.SubmitTx(&tx)
		if err != nil {
			Log.Warn(err)
			c.ErrorResponse(&rpc.Error{
				Code:    -1,
				Message: "could not submit transaction to daemon",
			})
			return
		}

		Log.Dev("submitRes:", submitRes)

		c.SuccessResponse(walletrpc.SubmitTransactionResponse{
			TXID: util.Hash(submitRes.TXID),
		})
	})

	rs.Handle("refresh", func(c *rpcserver.Context) {
		params := walletrpc.RefreshRequest{}
		err := c.GetParams(&params)
		if err != nil {
			c.ErrorResponse(&rpc.Error{
				Code:    invalidJson,
				Message: sInvalidJson,
			})
			return
		}

		err = w.Refresh()
		if err != nil {
			Log.Warn("refresh failed:", err)
		}

		c.SuccessResponse(rpc.ResponseOut{
			JsonRpc: "2.0",
			Result: walletrpc.RefreshResponse{
				Success: err == nil,
			},
			Id: c.Body.Id,
		})
	})

}
