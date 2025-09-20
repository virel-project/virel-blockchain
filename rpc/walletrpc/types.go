package walletrpc

import (
	"github.com/virel-project/virel-blockchain/v3/address"
	"github.com/virel-project/virel-blockchain/v3/rpc/daemonrpc"
	"github.com/virel-project/virel-blockchain/v3/util"
	"github.com/virel-project/virel-blockchain/v3/util/enc"
)

type TxInfo struct {
	Hash util.Hash                         `json:"hash"`
	Data *daemonrpc.GetTransactionResponse `json:"data,omitempty"`
}

type TxData struct {
	Sender    *address.Integrated `json:"sender"`    // Sender
	Recipient address.Integrated  `json:"recipient"` // Recipient
	Amount    uint64              `json:"amount"`
	Fee       uint64              `json:"fee"`
	Nonce     uint64              `json:"nonce"`
	Signature enc.Hex             `json:"signature"`
}

////////

type GetBalanceRequest struct {
}
type GetBalanceResponse struct {
	Balance          uint64 `json:"balance"`
	MempoolBalance   uint64 `json:"mempool_balance"`
	LastNonce        uint64 `json:"last_nonce"`
	MempoolLastNonce uint64 `json:"mempool_last_nonce"`
	DelegateId       uint64 `json:"delegate_id"`
}

type GetHistoryRequest struct {
	IncludeTxData             bool   `json:"include_tx_data"`
	FilterIncomingByPaymentId uint64 `json:"filter_incoming_by_payment_id"`
	TransferType              string `json:"transfer_type"` // incoming or outgoing
	Page                      uint64 `json:"page"`
}
type GetHistoryResponse struct {
	Transactions []TxInfo `json:"transactions"`
	MaxPage      uint64   `json:"max_page"`
}

type Output struct {
	Amount    uint64             `json:"amount"`
	Recipient address.Integrated `json:"recipient"`
}

type CreateTransactionRequest struct {
	Outputs []Output `json:"outputs"`
}
type CreateTransactionResponse struct {
	TxBlob enc.Hex   `json:"tx_blob"`
	TXID   util.Hash `json:"txid"`
	Fee    uint64    `json:"fee"`
}

type SubmitTransactionRequest struct {
	TxBlob enc.Hex `json:"tx_blob"`
}
type SubmitTransactionResponse struct {
	TXID util.Hash `json:"txid"`
}

type RefreshRequest struct {
}
type RefreshResponse struct {
	Success bool `json:"success"`
}

type GetSubaddressRequest struct {
	PaymentId     uint64              `json:"payment_id,omitempty"`
	Subaddress    *address.Integrated `json:"subaddress,omitempty"`
	Confirmations uint64              `json:"confirmations"`
	MaxPage       uint64              `json:"max_page"`
}
type GetSubaddressResponse struct {
	PaymentId            uint64             `json:"payment_id"`
	Subaddress           address.Integrated `json:"subaddress"`
	TotalReceived        uint64             `json:"total_received"`
	MempoolTotalReceived uint64             `json:"mempool_total_received"`
	Transactions         []TxInfo           `json:"transactions"`
}
