package stratum

import (
	"github.com/virel-project/virel-blockchain/util/enc"
	"github.com/virel-project/virel-blockchain/util/uint128"
)

// Virel follows the XMRig Stratum Mining Protocol as described here:
// https://github.com/xmrig/xmrig-proxy/blob/master/doc/STRATUM.md

type Job struct {
	Algo     string  `json:"algo"`   // algorithm name, for Virel it's rx/vrl
	Blob     enc.Hex `json:"blob"`   // this is the MiningBlob
	JobID    string  `json:"job_id"` // random string, different for each job
	Target   enc.Hex `json:"target"` // can be 4 or 8 bytes, example: b88d0600
	Height   uint64  `json:"height,omitempty"`
	SeedHash enc.Hex `json:"seed_hash"` // 32 bytes seed hash
}

// https://github.com/xmrig/xmrig-proxy/blob/master/doc/STRATUM.md#login
type LoginRequest struct {
	Login string   `json:"login"`
	Pass  string   `json:"pass"`
	Agent string   `json:"agent"`
	Algo  []string `json:"algo"` // list of supported algorithms
}
type LoginResponse struct {
	ID         string   `json:"id"`         // this field is only necessary for compatibility
	Job        Job      `json:"job"`        // first job for the miner
	Extensions []string `json:"extensions"` // should be [algo, keepalive]
	Status     string   `json:"status"`     // this field is only necessary for compatibility. Always "OK"
}

// https://github.com/xmrig/xmrig-proxy/blob/master/doc/STRATUM.md#submit
type SubmitRequest struct {
	ID         string  `json:"id,omitempty"` // this field is only for compatibility
	JobID      string  `json:"job_id"`
	Nonce      string  `json:"nonce"`                 // nonce is a hexadecimal 4-byte Little Endian number
	Blob       enc.Hex `json:"blob,omitempty"`        // used for merge mining and filled by masterchain node
	NonceExtra enc.Hex `json:"nonce_extra,omitempty"` // may be used to override the nonce extra
	Result     string  `json:"result"`                // the PoW hash result
}
type SubmitResponse struct {
	Status string           `json:"status"` // this field is only necessary for compatibility. Always "OK"
	Blocks []FoundBlockInfo `json:"blocks"` // list of found blocks (may be multiple for merge mining)
}
type FoundBlockInfo struct {
	Hash       enc.Hex         `json:"hash"`
	Height     uint64          `json:"height"`
	NetworkID  uint64          `json:"network_id"`
	Difficulty uint128.Uint128 `json:"difficulty"`
	Ok         bool
}

type KeepalivedRequest struct {
	ID string `json:"id"` // this field is only necessary for compatibility
}
type KeepalivedResponse struct {
	Status string `json:"status"` // this field is only necessary for compatibility. Always "KEEPALIVED"
}
