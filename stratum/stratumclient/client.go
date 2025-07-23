package stratumclient

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/virel-project/virel-blockchain/rpc"
	"github.com/virel-project/virel-blockchain/stratum"
	"github.com/virel-project/virel-blockchain/util"
)

type StratumResponse struct {
	ID      uint64 `json:"id"`
	Jsonrpc string `json:"jsonrpc,omitempty"`
	Method  string `json:"method,omitempty"`
	Status  string `json:"status,omitempty"`

	Job    *stratum.Job     `json:"params,omitempty"`
	Result *json.RawMessage `json:"result,omitempty"`

	Error any `json:"error,omitempty"`
}

const (
	max_read_size = 10 * 1024
	read_timeout  = 60 * 4
)

type Client struct {
	conn        net.Conn
	clientId    string
	destination string
	alive       bool
	wallet      string

	JobChan      chan *stratum.Job
	responseChan chan *rpc.ResponseIn

	util.RWMutex
}

func (cl *Client) Alive() bool {
	cl.RLock()
	defer cl.RUnlock()
	return cl.alive
}

func New(destination, wallet string) (
	cl *Client, err error,
) {
	return &Client{
		destination: destination,
		wallet:      wallet,
	}, nil
}

const merge_prefix = "merge-mining:"

// isNode should be true only if this client is used by the main chain node when it
// gets blocks from other chains for merge mining.
func (cl *Client) Start(isNode bool) error {
	var job *stratum.Job

	cl.JobChan = make(chan *stratum.Job, 1)
	cl.responseChan = make(chan *rpc.ResponseIn, 1)

	errored := func() error {
		var err error

		cl.conn, err = net.DialTimeout("tcp", cl.destination, time.Second*30)
		if err != nil {
			return err
		}

		cl.alive = true

		// send login
		var lg string
		if isNode {
			lg = strconv.Quote(merge_prefix + cl.wallet)
		} else {
			lg = strconv.Quote(cl.wallet)
		}
		data := `{` +
			`"id":1,` +
			`"method":"login",` +
			`"params":{` +
			`"login":` + lg + `,` +
			`"pass":` + strconv.Quote("x") + `,` +
			`"agent":` + strconv.Quote("stratum-client") +
			`}` +
			"}\n"

		if err = cl.Write([]byte(data)); err != nil {
			return err
		}

		// read the login response
		response := rpc.ResponseIn{}
		cl.conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		rdr := bufio.NewReaderSize(cl.conn, max_read_size)
		err = rpc.ReadJSON(&response, rdr)
		if err != nil {
			return err
		}
		if response.Error != nil {
			return errors.New("stratum server error")
		}

		loginRes := stratum.LoginResponse{}
		err = json.Unmarshal(response.Result, &loginRes)
		if err != nil {
			return err
		}

		cl.alive = true

		cl.clientId = loginRes.ID
		job = &loginRes.Job

		cl.JobChan <- job

		return nil
	}()

	if errored != nil {
		return errored
	}
	go cl.scanJobs()

	return nil
}

func (cl *Client) request(requestData any, expectedResponseId uint32) (*rpc.ResponseIn, error) {
	cl.Lock()
	if !cl.alive {
		cl.Unlock()
		return nil, errors.New("client is not alive")
	}
	data, err := json.Marshal(requestData)
	if err != nil {
		cl.Unlock()
		return nil, fmt.Errorf("failed to marshal work data: %w", err)
	}
	if err = cl.Write(append(data, '\n')); err != nil {
		cl.Unlock()
		return nil, fmt.Errorf("failed to write work: %w", err)
	}
	respChan := cl.responseChan
	cl.Unlock()

	// await a response from the stratum server
	response, ok := <-respChan
	if response == nil || !ok {
		return nil, fmt.Errorf("resp chan closed")
	}
	var resId uint32

	switch response.Id.(type) {
	case float32:
		resId = uint32(response.Id.(float32))
	case float64:
		resId = uint32(response.Id.(float64))
	case uint32:
		resId = uint32(response.Id.(uint32))
	case uint64:
		resId = uint32(response.Id.(uint64))
	default:
		return nil, fmt.Errorf("failed to submit work: invalid type of response id")
	}

	if resId != expectedResponseId {
		return nil, fmt.Errorf("failed to submit work: unexpected response %d, expected %d", response.Id, expectedResponseId)
	}
	return response, nil
}

func (cl *Client) SendWork(sr stratum.SubmitRequest) (*rpc.ResponseIn, error) {
	id := rand.Uint32()
	req := &rpc.RequestOut{
		Id:     id,
		Method: "submit",
		Params: sr,
	}
	return cl.request(req, id)
}

func (cl *Client) Close() {
	cl.Lock()
	defer cl.Unlock()
	if !cl.alive {
		return
	}
	cl.alive = false
	cl.conn.Close()
}

func (cl *Client) scanJobs() error {
	defer func() {
		close(cl.JobChan)
		close(cl.responseChan)
	}()
	reader := bufio.NewReaderSize(cl.conn, max_read_size)
	for {
		reqres := &rpc.RequestOrResponse{}
		cl.conn.SetReadDeadline(time.Now().Add(read_timeout * time.Second))
		err := rpc.ReadJSON(reqres, reader)
		if err != nil {
			return fmt.Errorf("failed to read jobs from pool: %w", err)
		}
		if reqres.Method == "" {
			cl.responseChan <- &rpc.ResponseIn{
				JsonRpc: reqres.JsonRpc,
				Result:  reqres.Result,
				Error:   reqres.Error,
				Id:      reqres.Id,
			}
			continue
		}
		if reqres.Method != "job" {
			continue
		}

		// this is a server-to-client Job request, unmarshal the job and send it to the jobs channel.
		var job stratum.Job
		err = json.Unmarshal(reqres.Params, &job)
		if err != nil {
			return fmt.Errorf("failed to unmarshal S2C Job: %w", err)
		}

		cl.JobChan <- &job
	}
}

func (cl *Client) Write(data []byte) error {
	cl.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := cl.conn.Write(data)
	return err
}
