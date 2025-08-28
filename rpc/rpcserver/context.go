package rpcserver

import (
	"encoding/json"
	"net/http"

	"github.com/virel-project/virel-blockchain/v2/rpc"
)

type Context struct {
	req *http.Request
	res http.ResponseWriter

	Body *rpc.RequestIn
}

func NewContext(req *http.Request, res http.ResponseWriter, body *rpc.RequestIn) *Context {
	return &Context{
		req:  req,
		res:  res,
		Body: body,
	}
}

func (c *Context) GetParams(result any) error {
	err := json.Unmarshal(c.Body.Params, result)

	if err != nil {
		c.Response(rpc.ResponseOut{
			JsonRpc: "2.0",
			Error: &rpc.Error{
				Code:    invalidJson,
				Message: sInvalidJson,
			},
			Id: c.Body.Id,
		})
	}
	return err
}

func (c *Context) Response(v rpc.ResponseOut) error {
	return WriteJSON(c.res, v)
}
func (c *Context) SuccessResponse(r any) error {
	return c.Response(rpc.ResponseOut{
		JsonRpc: "2.0",
		Result:  r,
		Id:      c.Body.Id,
	})
}
func (c *Context) ErrorResponse(e *rpc.Error) error {
	return c.Response(rpc.ResponseOut{
		JsonRpc: "2.0",
		Error:   e,
		Id:      c.Body.Id,
	})
}
