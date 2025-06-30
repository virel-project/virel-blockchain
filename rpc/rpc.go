package rpc

import (
	"bufio"
	"encoding/json"

	"github.com/virel-project/virel-blockchain/logger"
)

var Log *logger.Log = logger.DiscardLog

// https://www.jsonrpc.org/specification#request_object
type RequestIn struct {
	JsonRpc string          `json:"jsonrpc"` // Must be "2.0"
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	Id      any             `json:"id"`
}
type RequestOut struct {
	JsonRpc string `json:"jsonrpc"` // Must be "2.0"
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	Id      any    `json:"id"`
}
type NoreplyRequest struct {
	JsonRpc string `json:"jsonrpc"` // Must be "2.0"
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// https://www.jsonrpc.org/specification#response_object
type ResponseIn struct {
	JsonRpc string          `json:"jsonrpc"` // Must be "2.0"
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
	Id      any             `json:"id"`
}
type ResponseOut struct {
	JsonRpc string `json:"jsonrpc"` // Must be "2.0"
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
	Id      any    `json:"id"`
}

// https://www.jsonrpc.org/specification#error_object
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type RequestOrResponse struct {
	JsonRpc string          `json:"jsonrpc"` // Must be "2.0"
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
	Id      any             `json:"id"`
}

// reads from a connection and parses the data as JSON
func ReadJSON(response any, reader *bufio.Reader) error {
	data, _, err := reader.ReadLine()
	if err != nil {
		return err
	}
	Log.NetDevf("ReadJSON %s", data)
	return json.Unmarshal(data, response)
}
