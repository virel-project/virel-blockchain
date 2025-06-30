package daemonrpc

import (
	"github.com/virel-project/virel-blockchain/address"
	"github.com/virel-project/virel-blockchain/block"
	"github.com/virel-project/virel-blockchain/transaction"
	"github.com/virel-project/virel-blockchain/util"
	"github.com/virel-project/virel-blockchain/util/enc"
)

type GetTransactionRequest struct {
	Txid util.Hash `json:"txid"`
}

type GetTransactionResponse struct {
	Sender      *address.Integrated  `json:"sender"`
	Outputs     []transaction.Output `json:"outputs"`
	TotalAmount uint64               `json:"total_amount"`
	Fee         uint64               `json:"fee"`
	Nonce       uint64               `json:"nonce"`
	Signature   enc.Hex              `json:"signature"`
	Height      uint64               `json:"height"`
	Coinbase    bool                 `json:"coinbase"`
	VirtualSize uint64               `json:"virtual_size"`
}

type GetInfoRequest struct {
}
type GetInfoResponse struct {
	Height            uint64    `json:"height"`
	TopHash           util.Hash `json:"top_hash"`
	CirculatingSupply uint64    `json:"circulating_supply"`
	MaxSupply         uint64    `json:"max_supply"`
	Coin              uint64    `json:"coin"`
	Difficulty        string    `json:"difficulty"`
	CumulativeDiff    string    `json:"cumulative_diff"`
	Target            int       `json:"target_block_time"`
	BlockReward       uint64    `json:"block_reward"`
}

type GetAddressRequest struct {
	Address address.Integrated `json:"address"`
}
type GetAddressResponse struct {
	Balance         uint64 `json:"balance"`
	LastNonce       uint64 `json:"last_nonce"` // last nonce used
	LastIncoming    uint64 `json:"last_incoming"`
	MempoolBalance  uint64 `json:"mempool_balance"`    // unconfirmed balance, from mempool
	MempoolNonce    uint64 `json:"mempool_last_nonce"` // unconfirmed nonce, from mempool
	MempoolIncoming uint64 `json:"mempool_incoming"`
	Height          uint64 `json:"height"`
}

type GetTxListRequest struct {
	Address      address.Integrated `json:"address"`
	TransferType string             `json:"transfer_type"` // incoming or outgoing
	Page         uint64             `json:"page"`
}
type GetTxListResponse struct {
	Transactions []util.Hash `json:"transactions"`
	MaxPage      uint64      `json:"max_page"`
}

type SubmitTransactionRequest struct {
	Hex enc.Hex `json:"hex"` // transaction data as hex string
}
type SubmitTransactionResponse struct {
	TXID util.Hash `json:"txid"`
}

type GetBlockByHashRequest struct {
	Hash util.Hash `json:"hash"`
}
type GetBlockByHeightRequest struct {
	Height uint64 `json:"height"`
}
type GetBlockResponse struct {
	Block  block.Block `json:"block"`
	Hash   string      `json:"hash"`
	Reward uint64      `json:"reward"`
	Miner  string      `json:"miner"`
}

type CalcPowRequest struct {
	Blob     enc.Hex   `json:"blob"`
	SeedHash util.Hash `json:"seed_hash"`
}
type CalcPowResponse struct {
	Hash util.Hash `json:"hash"`
}

// TODO: implement these methods in the daemon
type GetBlockTemplateRequest struct {
	Address address.Integrated `json:"address"`
}
type GetBlockTemplateResponse struct {
	Blob       enc.Hex `json:"blob"`
	MiningBlob enc.Hex `json:"mining_blob"`
}
