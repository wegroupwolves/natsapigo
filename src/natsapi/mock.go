package natsapi

import (
	"encoding/json"
	"github.com/bytedance/sonic"
	"sync"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// NatsAPIMock mocks NATS subjects for testing. Register fake responses,
// then assert against captured payloads.
type NatsAPIMock struct {
	nc        *nats.Conn
	responses map[string]mockResponse
	Payloads  map[string][]json.RawMessage
	subs      []*nats.Subscription
	mu        sync.Mutex
}

type mockResponse struct {
	Result any
	Error  *JsonRPCError
}

func NewMock(nc *nats.Conn) *NatsAPIMock {
	return &NatsAPIMock{
		nc:        nc,
		responses: make(map[string]mockResponse),
		Payloads:  make(map[string][]json.RawMessage),
	}
}

// Request registers a mock response for a given subject.
func (m *NatsAPIMock) Request(subject string, result any, rpcErr *JsonRPCError) error {
	m.responses[subject] = mockResponse{Result: result, Error: rpcErr}

	sub, err := m.nc.Subscribe(subject, func(msg *nats.Msg) {
		m.mu.Lock()
		m.Payloads[subject] = append(m.Payloads[subject], msg.Data)
		m.mu.Unlock()

		resp := m.responses[subject]
		var reply *JsonRPCReply
		if resp.Error != nil {
			reply = NewErrorReply(uuid.NewString(), resp.Error)
		} else {
			raw, _ := sonic.Marshal(resp.Result)
			reply = NewResultReply(uuid.NewString(), raw)
		}

		data, _ := sonic.Marshal(reply)
		if msg.Reply != "" {
			_ = msg.Respond(data)
		}
	})
	if err != nil {
		return err
	}
	m.subs = append(m.subs, sub)
	return m.nc.Flush()
}

func (m *NatsAPIMock) Close() error {
	for _, sub := range m.subs {
		if err := sub.Unsubscribe(); err != nil {
			return err
		}
	}
	return nil
}
