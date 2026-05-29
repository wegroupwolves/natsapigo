package natsapi

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
)

type JsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Timeout *float64        `json:"timeout,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params"`
	ID      uuid.UUID       `json:"id,omitempty"`
}

func newRequestID() uuid.UUID {
	return uuid.New()
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
	ID      uuid.UUID       `json:"id"`
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

func NewResultReply(id uuid.UUID, result json.RawMessage) *JsonRPCReply {
	return &JsonRPCReply{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

func NewErrorReply(id uuid.UUID, rpcErr *JsonRPCError) *JsonRPCReply {
	return &JsonRPCReply{
		JSONRPC: "2.0",
		ID:      id,
		Error:   rpcErr,
	}
}
