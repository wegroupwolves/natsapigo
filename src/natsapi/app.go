package natsapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/bytedance/sonic"
	"github.com/nats-io/nats.go"
)

type NatsAPI struct {
	RootPath          string
	Title             string
	Version           string
	Config            Config
	State             map[string]any
	routes            map[string]Route
	publishRoutes     map[string]PublishRoute
	rootPaths         []string
	nc                *nats.Conn
	subs              []*nats.Subscription
	exceptionHandlers []ExceptionHandler
	onStartup         func() error
	onShutdown        func() error
	onClosed          func()
	docServerAddr string
	docServer     *http.Server
	asyncApiSpec    []byte
}

func New(rootPath string, opts ...Option) *NatsAPI {
	app := &NatsAPI{
		RootPath:      rootPath,
		Config:        DefaultConfig(),
		State:         make(map[string]any),
		routes:        make(map[string]Route),
		publishRoutes: make(map[string]PublishRoute),
		rootPaths:     []string{rootPath},
		exceptionHandlers: []ExceptionHandler{
			DefaultExceptionHandler,
		},
	}
	for _, opt := range opts {
		opt(app)
	}
	return app
}

type Option func(*NatsAPI)

func WithConfig(cfg Config) Option {
	return func(a *NatsAPI) { a.Config = cfg }
}

func WithExceptionHandler(h ExceptionHandler) Option {
	return func(a *NatsAPI) {
		a.exceptionHandlers = append([]ExceptionHandler{h}, a.exceptionHandlers...)
	}
}

func WithInfo(title, version string) Option {
	return func(a *NatsAPI) {
		a.Title = title
		a.Version = version
	}
}

func WithOnStartup(f func() error) Option {
	return func(a *NatsAPI) { a.onStartup = f }
}

func WithOnShutdown(f func() error) Option {
	return func(a *NatsAPI) { a.onShutdown = f }
}

func WithOnClosed(f func()) Option {
	return func(a *NatsAPI) { a.onClosed = f }
}

// WithAsyncAPIDocServer starts an HTTP server on addr (e.g. ":8090") when the app
// starts up, serving the AsyncAPI spec in a browser. No server is started unless
// this option is provided.
func WithAsyncAPIDocServer(addr string) Option {
	return func(a *NatsAPI) { a.docServerAddr = addr }
}

func (a *NatsAPI) Request(subject string, handler any, opts ...RouteOption) {
	fn, paramType, resultType := adaptHandler(handler)
	route := Route{
		Subject:    subject,
		Handler:    fn,
		ParamType:  paramType,
		ResultType: resultType,
	}
	for _, opt := range opts {
		opt(&route)
	}
	key := a.RootPath + "." + subject
	if _, exists := a.routes[key]; exists {
		panic(&DuplicateRouteError{Subject: key})
	}
	a.routes[key] = route
}

func (a *NatsAPI) Publish(subject string, handler any, opts ...PublishRouteOption) {
	fn, paramType := adaptPublishHandler(handler)
	route := PublishRoute{
		Subject:   subject,
		Handler:   fn,
		ParamType: paramType,
	}
	for _, opt := range opts {
		opt(&route)
	}
	key := a.RootPath + "." + subject
	if _, exists := a.publishRoutes[key]; exists {
		panic(&DuplicateRouteError{Subject: key})
	}
	a.publishRoutes[key] = route
}

func (a *NatsAPI) IncludeRouter(router *SubjectRouter, rootPath ...string) {
	rp := a.RootPath
	if len(rootPath) > 0 && rootPath[0] != "" {
		rp = rootPath[0]
		// Track additional root paths for subscription
		found := false
		for _, p := range a.rootPaths {
			if p == rp {
				found = true
				break
			}
		}
		if !found {
			a.rootPaths = append(a.rootPaths, rp)
		}
	}

	for _, route := range router.Routes {
		key := rp + "." + route.Subject
		if _, exists := a.routes[key]; exists {
			panic(&DuplicateRouteError{Subject: key})
		}
		a.routes[key] = route
	}
	for _, route := range router.PublishRoutes {
		key := rp + "." + route.Subject
		if _, exists := a.publishRoutes[key]; exists {
			panic(&DuplicateRouteError{Subject: key})
		}
		a.publishRoutes[key] = route
	}
}

func (a *NatsAPI) Startup(ctx context.Context) error {
	cfg := a.Config.Connect
	natsOpts := []nats.Option{
		nats.Name(cfg.Name),
		nats.ReconnectWait(cfg.ReconnectWait),
		nats.MaxReconnects(cfg.MaxReconnectAttempts),
		nats.Timeout(cfg.ConnectTimeout),
		nats.PingInterval(cfg.PingInterval),
		nats.MaxPingsOutstanding(cfg.MaxPingsOutstanding),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			slog.Error("NATS error", "err", err)
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			slog.Warn("NATS connection closed")
			if a.onClosed != nil {
				a.onClosed()
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			slog.Warn("NATS reconnected", "url", nc.ConnectedUrl())
		}),
	}

	if cfg.NKeysSeed != "" {
		opt, err := nats.NkeyOptionFromSeed(cfg.NKeysSeed)
		if err != nil {
			return fmt.Errorf("nkey seed error: %w", err)
		}
		natsOpts = append(natsOpts, opt)
	}
	if cfg.UserCredentials != "" {
		natsOpts = append(natsOpts, nats.UserCredentials(cfg.UserCredentials))
	}

	servers := nats.DefaultURL
	if len(cfg.Servers) > 0 {
		servers = cfg.Servers[0]
		for i := 1; i < len(cfg.Servers); i++ {
			servers += "," + cfg.Servers[i]
		}
	}

	nc, err := nats.Connect(servers, natsOpts...)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}
	a.nc = nc
	slog.Info("Connected to NATS server on "+nc.ConnectedUrl(), "servers", servers)

	a.registerSchemaHandler()

	if a.docServerAddr != "" {
		a.docServer = newAsyncAPIDocServer(a)
		go func() {
			if err := a.docServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("AsyncAPI doc server error", "err", err)
			}
		}()
		slog.Info("AsyncAPI doc server running", "addr", "http://"+a.docServerAddr)
	}

	if a.onStartup != nil {
		if err := a.onStartup(); err != nil {
			return fmt.Errorf("startup hook failed: %w", err)
		}
	}

	for _, rp := range a.rootPaths {
		subPath := rp + ".>"
		sub, err := nc.QueueSubscribe(subPath, a.Config.Subscribe.Queue, a.handleMessage)
		if err != nil {
			return fmt.Errorf("failed to subscribe to %s: %w", subPath, err)
		}
		a.subs = append(a.subs, sub)
		slog.Info("Subscribed", "subject", subPath)
	}

	return nil
}

func (a *NatsAPI) Shutdown() error {
	slog.Info("Shutting down NatsAPI")

	if a.onShutdown != nil {
		if err := a.onShutdown(); err != nil {
			slog.Error("Shutdown hook error", "err", err)
		}
	}

	if a.docServer != nil {
		if err := a.docServer.Shutdown(context.Background()); err != nil {
			slog.Error("AsyncAPI doc server shutdown error", "err", err)
		}
	}

	if a.nc != nil {
		if err := a.nc.Drain(); err != nil {
			slog.Error("NATS drain error", "err", err)
		}
		a.nc.Close()
	}

	slog.Info("NatsAPI shutdown complete")
	return nil
}

func (a *NatsAPI) Conn() *nats.Conn {
	return a.nc
}

func (a *NatsAPI) handleMessage(msg *nats.Msg) {
	if msg.Reply != "" {
		go a.handleRequest(msg)
	} else {
		go a.handlePublish(msg)
	}
}

func (a *NatsAPI) handleRequest(msg *nats.Msg) {
	var request JsonRPCRequest
	var reply *JsonRPCReply

	defer func() {
		if reply == nil {
			reply = NewErrorReply(request.ID, &JsonRPCError{
				Message:   "INTERNAL_ERROR",
				Timestamp: time.Now(),
			})
		}
		data, _ := sonic.Marshal(reply)
		if err := msg.Respond(data); err != nil {
			slog.Error("Failed to respond", "err", err)
		}
	}()

	if err := sonic.Unmarshal(msg.Data, &request); err != nil {
		request.ID = newRequestID()
		reply = NewErrorReply(request.ID, &JsonRPCError{
			Message:   "INVALID_REQUEST_FORMAT",
			Timestamp: time.Now(),
		})
		return
	}

	if request.ID == "" {
		request.ID = newRequestID()
	}

	subject := msg.Subject

	route, ok := a.routes[subject]
	if !ok && request.Method != "" {
		route, ok = a.routes[subject+"."+request.Method]
	}

	if !ok {
		reply = NewErrorReply(request.ID, &JsonRPCError{
			Message:   "NO_SUCH_ENDPOINT",
			Timestamp: time.Now(),
			Errors: []ErrorDetail{{Message: fmt.Sprintf("No such endpoint available for %s", subject)}},
		})
		return
	}

	result, err := route.Handler(a, request.Params)
	if err != nil {
		rpcErr := a.handleError(err, &request, subject)
		reply = NewErrorReply(request.ID, rpcErr)
		return
	}

	reply = NewResultReply(request.ID, result)
}

func (a *NatsAPI) handlePublish(msg *nats.Msg) {
	var request JsonRPCRequest
	if err := sonic.Unmarshal(msg.Data, &request); err != nil {
		slog.Error("Failed to parse publish message", "err", err)
		return
	}

	if request.ID == "" {
		request.ID = newRequestID()
	}

	subject := msg.Subject

	route, ok := a.publishRoutes[subject]
	if !ok {
		r, rok := a.routes[subject]
		if rok {
			if _, err := r.Handler(a, request.Params); err != nil {
				slog.Error("Publish handler error", "err", err, "subject", subject)
			}
		} else {
			slog.Warn("No handler for publish", "subject", subject)
		}
		return
	}

	if err := route.Handler(a, request.Params); err != nil {
		slog.Error("Publish handler error", "err", err, "subject", subject)
	}
}

func (a *NatsAPI) handleError(err error, request *JsonRPCRequest, subject string) *JsonRPCError {
	for _, handler := range a.exceptionHandlers {
		rpcErr := handler(err, request, subject)
		if rpcErr != nil {
			return rpcErr
		}
	}
	return DefaultExceptionHandler(err, request, subject)
}

// SendRequest sends a JSON-RPC request and waits for a reply.
func (a *NatsAPI) SendRequest(subject string, params any, timeout time.Duration) (*JsonRPCReply, error) {
	req, err := NewJsonRPCRequest(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	t := timeout.Seconds()
	req.Timeout = &t

	data, err := sonic.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	msg, err := a.nc.Request(subject, data, timeout)
	if err != nil {
		return nil, fmt.Errorf("NATS request failed: %w", err)
	}

	var reply JsonRPCReply
	if err := sonic.Unmarshal(msg.Data, &reply); err != nil {
		return nil, fmt.Errorf("failed to unmarshal reply: %w", err)
	}
	return &reply, nil
}

// SendPublish sends a JSON-RPC publish (fire-and-forget).
func (a *NatsAPI) SendPublish(subject string, params any) error {
	req, err := NewJsonRPCRequest(params)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	t := float64(-1)
	req.Timeout = &t

	data, err := sonic.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	return a.nc.Publish(subject, data)
}
