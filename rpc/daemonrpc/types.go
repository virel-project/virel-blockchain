package daemonrpc

import (
	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/block"
	"github.com/virel-project/virel-blockchain/v2/transaction"
	"github.com/virel-project/virel-blockchain/v2/util"
	"github.com/virel-project/virel-blockchain/v2/util/enc"
)

type GetTransactionRequest struct {
	Txid util.Hash `json:"txid"`
}

type GetTransactionResponse struct {
	Signer      *address.Integrated       `json:"sender"`
	Inputs      []transaction.StateInput  `json:"inputs"`
	Outputs     []transaction.StateOutput `json:"outputs"`
	TotalAmount uint64                    `json:"total_amount"`
	Fee         uint64                    `json:"fee"`
	Nonce       uint64                    `json:"nonce"`
	Signature   enc.Hex                   `json:"signature"`
	Height      uint64                    `json:"height"`
	Coinbase    bool                      `json:"coinbase"`
	VirtualSize uint64                    `json:"virtual_size"`
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
	Version           string    `json:"version"`
	Connections       int       `json:"peers"`
}

type GetAddressRequest struct {
	Address string `json:"address"`
}
type GetAddressResponse struct {
	Balance         uint64 `json:"balance"`
	LastNonce       uint64 `json:"last_nonce"` // last nonce used
	LastIncoming    uint64 `json:"last_incoming"`
	MempoolBalance  uint64 `json:"mempool_balance"`    // unconfirmed balance, from mempool
	MempoolNonce    uint64 `json:"mempool_last_nonce"` // unconfirmed nonce, from mempool
	MempoolIncoming uint64 `json:"mempool_incoming"`
	DelegateId      uint64 `json:"delegate_id"`
	TotalStaked     uint64 `json:"total_staked"`   // The total amount staked since the last delegate change.
	TotalUnstaked   uint64 `json:"total_unstaked"` // The total amount unstaked since the last delegate change.
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
	Block        block.Block     `json:"block"`
	Hash         string          `json:"hash"`
	TotalReward  uint64          `json:"total_reward"`
	MinerReward  uint64          `json:"miner_reward"`
	Miner        string          `json:"miner"`
	Delegate     address.Address `json:"delegate"`
	NextDelegate address.Address `json:"next_delegate"`
}

type CalcPowRequest struct {
	Blob     enc.Hex   `json:"blob"`
	SeedHash util.Hash `json:"seed_hash"`
}
type CalcPowResponse struct {
	Hash util.Hash `json:"hash"`
}

type ValidateAddressRequest struct {
	Address string `json:"address"`
}
type ValidateAddressResponse struct {
	Address      string `json:"address"`
	Valid        bool   `json:"valid"`
	ErrorMessage string `json:"error_message,omitempty"`
	MainAddress  string `json:"main_address"`
	PaymentId    uint64 `json:"payment_id"`
}

type State struct {
	Balance       uint64
	LastNonce     uint64
	LastIncoming  uint64 // not used in consensus, but we store it to list the wallet incoming transactions
	DelegateId    uint64
	TotalStaked   uint64
	TotalUnstaked uint64
}
type StateInfo struct {
	Address string
	State   *State
}

type RichListRequest struct {
}
type RichListResponse struct {
	Richest []StateInfo
}

type SubmitStakeSignatureRequest struct {
	DelegateId uint64  `json:"delegate_id"` // the delegate who signed this hash
	Hash       enc.Hex `json:"hash"`        // the block hash
	Signature  enc.Hex `json:"signature"`   // the signature
}
type SubmitStakeSignatureResponse struct {
	Accepted     bool   `json:"accepted"`
	ErrorMessage string `json:"error_message"`
}

type DelegatedFund struct {
	Owner  address.Address `json:"owner"`
	Amount uint64          `json:"amount"`
}

type GetDelegateRequest struct {
	DelegateId      uint64 `json:"delegate_id"`
	DelegateAddress string `json:"delegate_address"`
}
type GetDelegateResponse struct {
	Id      uint64           `json:"id"`
	Address address.Address  `json:"address"`
	Owner   address.Address  `json:"owner"`
	Name    string           `json:"name"`
	Funds   []*DelegatedFund `json:"funds"`
}
