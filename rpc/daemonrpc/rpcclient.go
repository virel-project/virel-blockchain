package daemonrpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/virel-project/virel-blockchain/rpc"
)

type RpcClient struct {
	DaemonAddress string
}

func NewRpcClient(addr string) *RpcClient {
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	return &RpcClient{
		DaemonAddress: addr,
	}
}

func (r *RpcClient) Request(method string, params any, output any) error {
	body := rpc.RequestOut{
		JsonRpc: "2.0",
		Method:  method,
		Params:  params,
		Id:      0,
	}

	b, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", r.DaemonAddress, bytes.NewReader(b))
	if err != nil {
		return err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	dat, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	out2 := rpc.ResponseIn{}

	err = json.Unmarshal(dat, &out2)
	if err != nil {
		return err
	}

	if out2.Error != nil {
		errorStr, err := json.Marshal(out2.Error)
		if err != nil {
			return err
		}

		return errors.New(string(errorStr))
	}

	resByte, err := json.Marshal(out2.Result)
	if err != nil {
		return err
	}

	return json.Unmarshal(resByte, output)
}
