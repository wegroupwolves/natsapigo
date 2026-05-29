package natsapi

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/bytedance/sonic"
)

var natsAPIType = reflect.TypeOf((*NatsAPI)(nil))
var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
var errorType = reflect.TypeOf((*error)(nil)).Elem()
var zeroReflectValue = reflect.Value{}

// adaptHandler accepts either a HandlerFunc or func(context.Context, *NatsAPI, P) (R, error)
// and returns a HandlerFunc plus the param and result types for schema generation.
func adaptHandler(handler any) (HandlerFunc, reflect.Type, reflect.Type) {
	if hf, ok := handler.(HandlerFunc); ok {
		return hf, nil, nil
	}

	v := reflect.ValueOf(handler)
	t := v.Type()

	if t.Kind() != reflect.Func ||
		t.NumIn() != 3 || t.NumOut() != 2 ||
		!t.In(0).Implements(contextType) ||
		t.In(1) != natsAPIType ||
		!t.Out(1).Implements(errorType) {
		panic(fmt.Sprintf("handler must be func(context.Context, *NatsAPI, P) (R, error), got %T", handler))
	}

	paramType := t.In(2)
	resultType := t.Out(0)
	zero := reflect.Zero(paramType)

	pool := sync.Pool{
		New: func() any { return reflect.New(paramType).Interface() },
	}

	return func(ctx context.Context, a *NatsAPI, raw json.RawMessage) (json.RawMessage, error) {
		ptr := pool.Get()
		reflect.ValueOf(ptr).Elem().Set(zero)

		if err := sonic.Unmarshal(raw, ptr); err != nil {
			pool.Put(ptr)
			return nil, NewJsonRPCException("INVALID_PARAMETERS_RECEIVED", ErrorDetail{Message: err.Error()})
		}

		out := v.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(a), reflect.ValueOf(ptr).Elem()})
		pool.Put(ptr)

		if !out[1].IsNil() {
			return nil, out[1].Interface().(error)
		}
		return sonic.Marshal(out[0].Interface())
	}, paramType, resultType
}

// adaptPublishHandler accepts either a PublishHandlerFunc or func(context.Context, *NatsAPI, P) error
// and returns a PublishHandlerFunc plus the param type for schema generation.
func adaptPublishHandler(handler any) (PublishHandlerFunc, reflect.Type) {
	if hf, ok := handler.(PublishHandlerFunc); ok {
		return hf, nil
	}

	v := reflect.ValueOf(handler)
	t := v.Type()

	if t.Kind() != reflect.Func ||
		t.NumIn() != 3 || t.NumOut() != 1 ||
		!t.In(0).Implements(contextType) ||
		t.In(1) != natsAPIType ||
		!t.Out(0).Implements(errorType) {
		panic(fmt.Sprintf("publish handler must be func(context.Context, *NatsAPI, P) error, got %T", handler))
	}

	paramType := t.In(2)
	zero := reflect.Zero(paramType)

	pool := sync.Pool{
		New: func() any { return reflect.New(paramType).Interface() },
	}

	return func(ctx context.Context, a *NatsAPI, raw json.RawMessage) error {
		ptr := pool.Get()
		reflect.ValueOf(ptr).Elem().Set(zero)

		if err := sonic.Unmarshal(raw, ptr); err != nil {
			pool.Put(ptr)
			return NewJsonRPCException("INVALID_PARAMETERS_RECEIVED", ErrorDetail{Message: err.Error()})
		}

		out := v.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(a), reflect.ValueOf(ptr).Elem()})
		pool.Put(ptr)

		if !out[0].IsNil() {
			return out[0].Interface().(error)
		}
		return nil
	}, paramType
}
