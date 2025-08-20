package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/virel-project/virel-blockchain/address"
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

		txns, err := w.GetTransactions(strings.HasPrefix(strings.ToLower(params.TransferType), "inc"), uint64(params.Page))
		if err != nil {
			Log.Warn(err)
			c.ErrorResponse(&rpc.Error{
				Code:    internalReadFailed,
				Message: "failed to get transactions",
			})
			return
		}

		txinfo := make([]walletrpc.TxInfo, 0, len(txns.Transactions))

		if params.IncludeTxData {
			for _, txid := range txns.Transactions {
				resp, err := w.GetTransaction(txid)
				if err != nil {
					Log.Warn(err)
					c.ErrorResponse(&rpc.Error{
						Code:    internalReadFailed,
						Message: "failed to get transaction " + txid.String(),
					})
					return
				}
				if params.FilterIncomingByPaymentId != 0 {
					ok := false
					for _, out := range resp.Outputs {
						if out.PaymentId == params.FilterIncomingByPaymentId && out.Recipient == w.GetAddress().Addr {
							ok = true
							break
						}
					}
					if !ok {
						continue
					}
				}
				txinfo = append(txinfo, walletrpc.TxInfo{
					Hash: txid,
					Data: resp,
				})
			}
		} else {
			for _, v := range txns.Transactions {
				txinfo = append(txinfo, walletrpc.TxInfo{
					Hash: v,
				})
			}
		}

		c.SuccessResponse(walletrpc.GetHistoryResponse{
			Transactions: txinfo,
			MaxPage:      txns.MaxPage,
		})
	})

	rs.Handle("get_subaddress", func(c *rpcserver.Context) {
		params := walletrpc.GetSubaddressRequest{}
		err := c.GetParams(&params)
		if err != nil {
			return
		}

		if params.Subaddress == nil {
			wa := w.GetAddress()
			params.Subaddress = &wa
			params.Subaddress.PaymentId = params.PaymentId
		}
		if params.Subaddress.Addr == address.INVALID_ADDRESS {
			c.ErrorResponse(&rpc.Error{
				Code:    -1,
				Message: "invalid subaddress",
			})
			return
		}

		if params.Subaddress.Addr != w.GetAddress().Addr {
			c.ErrorResponse(&rpc.Error{
				Code:    -1,
				Message: "the subaddress specified does not belong to this wallet",
			})
			return
		}
		if params.Subaddress.PaymentId == 0 {
			c.ErrorResponse(&rpc.Error{
				Code:    -1,
				Message: "you must specify a valid subaddress or payment id",
			})
			return
		}

		res := walletrpc.GetSubaddressResponse{
			PaymentId:            params.Subaddress.PaymentId,
			Subaddress:           *params.Subaddress,
			TotalReceived:        0,
			MempoolTotalReceived: 0,
		}

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
				txres, err := w.GetTransaction(tx)
				if err != nil {
					Log.Err(err)
					continue
				}
				for _, v := range txres.Outputs {
					if v.Recipient == params.Subaddress.Addr && v.PaymentId == params.Subaddress.PaymentId {
						if txres.Height == 0 {
							res.MempoolTotalReceived += v.Amount
						} else {
							res.TotalReceived += v.Amount
						}
					}
				}
			}
			page++
			if txlist.MaxPage <= page {
				break
			}
		}

		c.SuccessResponse(res)
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
			outs[i].PaymentId = v.Recipient.PaymentId

			if outs[i].Recipient == address.INVALID_ADDRESS {
				c.ErrorResponse(&rpc.Error{
					Code:    -1,
					Message: fmt.Sprintf("cannot transfer to invalid address %v", outs[i].Recipient),
				})
				return
			}
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
			Fee:    tx.Fee,
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
