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
		c.Response(rpc.ResponseOut{
			JsonRpc: "2.0",
			Result: walletrpc.GetBalanceResponse{
				Balance:        w.GetBalance(),
				MempoolBalance: w.GetMempoolBalance(),
			},
			Id: c.Body.Id,
		})
	})

	rs.Handle("get_history", func(c *rpcserver.Context) {
		params := walletrpc.GetHistoryRequest{}
		err := c.GetParams(&params)
		if err != nil {
			return
		}

		// TODO: implement IncludeTxData
		if params.IncludeTxData {
			Log.Warn("IncludeTxData not implemented")
		}

		var inc []walletrpc.TxInfo
		var out []walletrpc.TxInfo

		if params.IncludeIncoming {
			inc = []walletrpc.TxInfo{}

			var page uint64 = 0
			for {
				txlist, err := w.GetTransations(true, page)
				if err != nil {
					c.Response(rpc.ResponseOut{
						JsonRpc: "2.0",
						Error: &rpc.Error{
							Code:    internalReadFailed,
							Message: "failed to get transactions",
						},
						Id: c.Body.Id,
					})
					Log.Warn(err)
					return
				}
				for _, tx := range txlist.Transactions {
					txinfo := walletrpc.TxInfo{
						Hash: tx,
					}
					inc = append(inc, txinfo)
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
				txlist, err := w.GetTransations(false, page)
				if err != nil {
					c.Response(rpc.ResponseOut{
						JsonRpc: "2.0",
						Error: &rpc.Error{
							Code:    internalReadFailed,
							Message: "failed to get transactions",
						},
						Id: c.Body.Id,
					})
					Log.Warn(err)
					return
				}
				for _, tx := range txlist.Transactions {
					txinfo := walletrpc.TxInfo{
						Hash: tx,
					}
					out = append(out, txinfo)
				}
				if txlist.MaxPage <= page {
					break
				}
			}
		}

		c.Response(rpc.ResponseOut{
			JsonRpc: "2.0",
			Result:  walletrpc.GetHistoryResponse{},
			Id:      c.Body.Id,
		})
	})

	rs.Handle("create_transaction", func(c *rpcserver.Context) {
		params := walletrpc.CreateTransactionRequest{}
		err := c.GetParams(&params)
		if err != nil {
			return
		}

		tx, err := w.Transfer(params.Amount, params.Destination)
		if err != nil {
			Log.Warn(err)
			c.Response(rpc.ResponseOut{
				JsonRpc: "2.0",
				Error: &rpc.Error{
					Code:    -1,
					Message: "transfer failed",
				},
				Id: c.Body.Id,
			})
			return
		}

		c.Response(rpc.ResponseOut{
			JsonRpc: "2.0",
			Result: walletrpc.CreateTransactionResponse{
				TxBlob: tx.Serialize(),
				TXID:   util.Hash(tx.Hash()),
			},
			Id: c.Body.Id,
		})
	})

	rs.Handle("submit_transaction", func(c *rpcserver.Context) {
		params := walletrpc.SubmitTransactionRequest{}
		err := c.GetParams(&params)
		if err != nil {
			c.Response(rpc.ResponseOut{
				JsonRpc: "2.0",
				Error: &rpc.Error{
					Code:    invalidJson,
					Message: sInvalidJson,
				},
				Id: c.Body.Id,
			})
			return
		}

		tx := transaction.Transaction{}
		err = tx.Deserialize(params.TxBlob)
		if err != nil {
			Log.Warn(err)
			c.Response(rpc.ResponseOut{
				JsonRpc: "2.0",
				Error: &rpc.Error{
					Code:    -1,
					Message: "could not deserialize transaction",
				},
				Id: c.Body.Id,
			})
			return
		}

		submitRes, err := w.SubmitTx(&tx)
		if err != nil {
			Log.Warn(err)
			c.Response(rpc.ResponseOut{
				JsonRpc: "2.0",
				Error: &rpc.Error{
					Code:    -1,
					Message: "could not submit transaction to daemon",
				},
				Id: c.Body.Id,
			})
			return
		}

		Log.Dev("submitRes:", submitRes)

		c.Response(rpc.ResponseOut{
			JsonRpc: "2.0",
			Result: walletrpc.SubmitTransactionResponse{
				TXID: util.Hash(submitRes.TXID),
			},
			Id: c.Body.Id,
		})

	})

}
