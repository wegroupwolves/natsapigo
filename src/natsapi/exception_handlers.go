package natsapi

import (
	"errors"
	"log/slog"
	"time"
)

// ExceptionHandler converts an error into a JsonRPCError for the reply.
type ExceptionHandler func(err error, request *JsonRPCRequest, subject string) *JsonRPCError

func DefaultExceptionHandler(err error, request *JsonRPCRequest, subject string) *JsonRPCError {
	var rpcErr *JsonRPCException
	if errors.As(err, &rpcErr) {
		slog.Error("JsonRPCException", "err", err, "subject", subject, "jsonrpc_id", request.ID)
		return &JsonRPCError{
			Code:      -32603,
			Message:   rpcErr.Msg,
			Timestamp: time.Now(),
			Errors:    rpcErr.Errors,
		}
	}

	slog.Error("InternalError", "err", err, "subject", subject, "jsonrpc_id", request.ID)
	return &JsonRPCError{
		Code:      -32603,
		Message:   err.Error(),
		Timestamp: time.Now(),
	}
}
