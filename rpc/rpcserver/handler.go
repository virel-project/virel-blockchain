package rpcserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/virel-project/virel-blockchain/v3/rpc"
)

const invalidJson = -32700
const sInvalidJson = "Parse error"

const invalidMethod = -32601
const sInvalidMethod = "Method not found"

const invalidRequest = -32600

func (s *Server) handler(res http.ResponseWriter, req *http.Request) error {
	if req.Method == "OPTIONS" {
		if len(s.config.Authentication) == 0 {
			res.Header().Set("Access-Control-Allow-Origin", "*")
			res.WriteHeader(204)
			return nil
		}
	}

	if req.Method != "POST" {
		res.WriteHeader(405)
		WriteJSON(res, rpc.ResponseOut{
			JsonRpc: "2.0",
			Error: &rpc.Error{
				Code:    405,
				Message: "Method Not Allowed",
			},
		})
		return errors.New("method not allowed")
	}

	ip := strings.Split(req.RemoteAddr, ":")[0]
	if !s.limit.CanAct(ip, 1) {
		res.WriteHeader(429)
		WriteJSON(res, rpc.ResponseOut{
			JsonRpc: "2.0",
			Error: &rpc.Error{
				Code:    429,
				Message: "Too Many Requests",
			},
		})
		return errors.New("too many requests")
	}

	if s.config.Restricted {
		origin := req.Header.Get("Origin")
		if origin != "" && origin != "127.0.0.1" && origin != "localhost" {
			res.WriteHeader(400)
			WriteJSON(res, rpc.ResponseOut{
				JsonRpc: "2.0",
				Error: &rpc.Error{
					Code:    400,
					Message: "invalid origin",
				},
			})
			return errors.New("invalid origin")
		}
	}

	if len(s.config.Authentication) != 0 {
		uname, pw, ok := req.BasicAuth()
		if !ok || uname+":"+pw != s.config.Authentication {
			res.WriteHeader(400)
			s.limit.CanAct(ip, 9)
			WriteJSON(res, rpc.ResponseOut{
				JsonRpc: "2.0",
				Error: &rpc.Error{
					Code:    400,
					Message: "unauthorized",
				},
			})
			return errors.New("unauthorized")
		}
	}

	body, err := io.ReadAll(req.Body)
	if err != nil || len(body) < 2 {
		WriteJSON(res, rpc.ResponseOut{
			JsonRpc: "2.0",
			Error: &rpc.Error{
				Code:    invalidJson,
				Message: sInvalidJson,
			},
			Id: 0,
		})
		return errors.New("invalid json")
	}

	var jsonBody rpc.RequestIn

	err = json.Unmarshal(body, &jsonBody)
	if err != nil {
		WriteJSON(res, rpc.ResponseOut{
			JsonRpc: "2.0",
			Error: &rpc.Error{
				Code:    invalidJson,
				Message: sInvalidJson,
			},
			Id: jsonBody.Id,
		})
		return fmt.Errorf("invalid rpc request body: %w", err)
	}

	res.Header().Set("Content-Type", "application/json")
	if len(s.config.Authentication) == 0 {
		res.Header().Set("Access-Control-Allow-Origin", "*")
	}

	if jsonBody.JsonRpc != "2.0" {
		WriteJSON(res, rpc.ResponseOut{
			JsonRpc: "2.0",
			Error: &rpc.Error{
				Code:    invalidRequest,
				Message: "invalid json_rpc version, expected 2.0",
			},
			Id: jsonBody.Id,
		})
		return errors.New("invalid json_rpc version, expected 2.0")
	}

	if jsonBody.Params == nil {
		jsonBody.Params = json.RawMessage{}
	}

	// Handle the input

	jsonBody.Method = strings.ToLower(jsonBody.Method)

	handler := s.handlers[jsonBody.Method]
	if handler == nil {
		WriteJSON(res, rpc.ResponseOut{
			JsonRpc: "2.0",
			Error: &rpc.Error{
				Code:    invalidMethod,
				Message: sInvalidMethod,
			},
			Id: jsonBody.Id,
		})
		return errors.New("invalid method")
	}

	ctx := NewContext(req, res, &jsonBody)

	handler(ctx)
	return nil
}

func WriteJSON(res http.ResponseWriter, v any) error {
	bin, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = res.Write(bin)
	return err
}
