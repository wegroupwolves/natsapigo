package natsapi

import (
	"encoding/json"
	"github.com/bytedance/sonic"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

type JsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Timeout *float64        `json:"timeout,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params"`
	ID      string          `json:"id,omitempty"`
}

var requestCounter atomic.Uint64

func newRequestID() string {
	return fmt.Sprintf("%016x", requestCounter.Add(1))
}

func NewJsonRPCRequest(params any) (*JsonRPCRequest, error) {
	raw, err := sonic.Marshal(params)
	if err != nil {
		return nil, err
	}
	return &JsonRPCRequest{
		JSONRPC: "2.0",
		ID:      newRequestID(),
		Params:  raw,
	}, nil
}

type JsonRPCReply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JsonRPCError   `json:"error,omitempty"`
}

func (r *JsonRPCReply) Validate() error {
	if r.Result == nil && r.Error == nil {
		return errors.New("a result or error is required")
	}
	if r.Result != nil && r.Error != nil {
		return errors.New("a reply must not have both result and error")
	}
	return nil
}

type JsonRPCError struct {
	Message   string        `json:"message"`
	Timestamp time.Time     `json:"timestamp"`
	Errors    []ErrorDetail `json:"errors"`
}

type ErrorDetail struct {
	Type    string `json:"type"`
	Target  string `json:"target,omitempty"`
	Message string `json:"message"`
}

func NewResultReply(id string, result json.RawMessage) *JsonRPCReply {
	return &JsonRPCReply{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

func NewErrorReply(id string, rpcErr *JsonRPCError) *JsonRPCReply {
	return &JsonRPCReply{
		JSONRPC: "2.0",
		ID:      id,
		Error:   rpcErr,
	}
}
