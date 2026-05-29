package natsapi

import (
	"context"
	"encoding/json"
	"reflect"
)

// HandlerFunc is the internal low-level handler type used by Route.
type HandlerFunc func(ctx context.Context, app *NatsAPI, params json.RawMessage) (json.RawMessage, error)

// PublishHandlerFunc is the internal low-level publish handler type used by PublishRoute.
type PublishHandlerFunc func(ctx context.Context, app *NatsAPI, params json.RawMessage) error

// Handler is the typed handler signature for request/reply handlers.
// Use this to define handlers with typed params and results:
//
//	func HandleHealth() natsapi.Handler[struct{}, StatusResult] {
//	    return func(ctx context.Context, _ *natsapi.NatsAPI, _ struct{}) (StatusResult, error) {
//	        return StatusResult{Status: "OK"}, nil
//	    }
//	}
type Handler[P, R any] func(context.Context, *NatsAPI, P) (R, error)

// PublishHandler is the typed handler signature for publish (fire-and-forget) handlers.
type PublishHandler[P any] func(context.Context, *NatsAPI, P) error

type Route struct {
	Subject     string
	Handler     HandlerFunc
	Description string
	Tags        []string
	Deprecated  bool
	ParamType   reflect.Type
	ResultType  reflect.Type
}

type PublishRoute struct {
	Subject     string
	Handler     PublishHandlerFunc
	Description string
	Tags        []string
	Deprecated  bool
	ParamType   reflect.Type
}

type SubjectRouter struct {
	Prefix        string
	Tags          []string
	Routes        []Route
	PublishRoutes []PublishRoute
}

func NewSubjectRouter(prefix string, tags ...string) *SubjectRouter {
	return &SubjectRouter{
		Prefix: prefix,
		Tags:   tags,
	}
}

func (r *SubjectRouter) Request(subject string, handler any, opts ...RouteOption) {
	fn, paramType, resultType := adaptHandler(handler)
	route := Route{
		Subject:    r.prefixed(subject),
		Handler:    fn,
		Tags:       r.Tags,
		ParamType:  paramType,
		ResultType: resultType,
	}
	for _, opt := range opts {
		opt(&route)
	}
	r.Routes = append(r.Routes, route)
}

func (r *SubjectRouter) Publish(subject string, handler any, opts ...PublishRouteOption) {
	fn, paramType := adaptPublishHandler(handler)
	route := PublishRoute{
		Subject:   r.prefixed(subject),
		Handler:   fn,
		Tags:      r.Tags,
		ParamType: paramType,
	}
	for _, opt := range opts {
		opt(&route)
	}
	r.PublishRoutes = append(r.PublishRoutes, route)
}

func (r *SubjectRouter) prefixed(subject string) string {
	if r.Prefix == "" {
		return subject
	}
	return r.Prefix + "." + subject
}

type RouteOption func(*Route)

func WithDescription(desc string) RouteOption {
	return func(r *Route) { r.Description = desc }
}

func WithTags(tags ...string) RouteOption {
	return func(r *Route) { r.Tags = append(r.Tags, tags...) }
}

type PublishRouteOption func(*PublishRoute)

func WithPublishDescription(desc string) PublishRouteOption {
	return func(r *PublishRoute) { r.Description = desc }
}
