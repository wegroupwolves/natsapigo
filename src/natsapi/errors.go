package natsapi

import "fmt"

// JsonRPCException is a domain error that maps directly to a JSON-RPC error response.
// Use ErrorDetail to attach structured validation or context errors.
type JsonRPCException struct {
	Msg    string
	Errors []ErrorDetail
}

func (e *JsonRPCException) Error() string {
	return e.Msg
}

func NewJsonRPCException(message string, errors ...ErrorDetail) *JsonRPCException {
	return &JsonRPCException{Msg: message, Errors: errors}
}

type DuplicateRouteError struct {
	Subject string
}

func (e *DuplicateRouteError) Error() string {
	return fmt.Sprintf("DuplicateRouteError: %s is defined twice", e.Subject)
}
