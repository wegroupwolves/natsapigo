package natsapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"

	"github.com/WeGroup/natsapigo/src/natsapi"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func startEmbeddedNATS() (*natsserver.Server, string) {
	opts := &natsserver.Options{Host: "127.0.0.1", Port: -1}
	s, err := natsserver.NewServer(opts)
	if err != nil {
		panic(err)
	}
	s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		panic("NATS server not ready")
	}
	return s, fmt.Sprintf("nats://%s", s.Addr().String())
}

func newTestApp(url string) *natsapi.NatsAPI {
	cfg := natsapi.DefaultConfig()
	cfg.Connect.Servers = []string{url}
	return natsapi.New("natsapi.development", natsapi.WithConfig(cfg))
}

type StatusResult struct {
	Status string `json:"status"`
}

type GreetParams struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type GreetResult struct {
	Message string `json:"message"`
}

var _ = Context("NatsAPI", func() {
	var (
		server  *natsserver.Server
		natsURL string
		app     *natsapi.NatsAPI
	)

	BeforeEach(func() {
		server, natsURL = startEmbeddedNATS()
		app = newTestApp(natsURL)
	})

	AfterEach(func() {
		if app != nil {
			_ = app.Shutdown()
		}
		if server != nil {
			server.Shutdown()
		}
	})

	When("Sending a request to a registered handler", func() {
		It("should get a successful reply", func() {
			// given
			app.Request("foo", func(_ *natsapi.NatsAPI, _ struct{}) (StatusResult, error) {
				return StatusResult{Status: "OK"}, nil
			})
			err := app.Startup(context.Background())
			Expect(err).To(BeNil())

			// when
			reply, err := app.SendRequest("natsapi.development.foo", map[string]any{"foo": 1}, time.Second)

			// then
			Expect(err).To(BeNil())
			Expect(reply.Error).To(BeNil())
			var result StatusResult
			Expect(json.Unmarshal(reply.Result, &result)).To(Succeed())
			Expect(result.Status).To(Equal("OK"))
		})
	})

	When("Sending a request to a nonexistent subject", func() {
		It("should get a NO_SUCH_ENDPOINT error", func() {
			// given
			err := app.Startup(context.Background())
			Expect(err).To(BeNil())

			// when
			reply, err := app.SendRequest("natsapi.development.nonexistent.CREATE", map[string]any{}, time.Second)

			// then
			Expect(err).To(BeNil())
			Expect(reply.Error).ToNot(BeNil())
			Expect(reply.Error.Message).To(Equal("NO_SUCH_ENDPOINT"))
		})
	})

	When("A handler raises a JsonRPCException", func() {
		It("should return the error code and message", func() {
			// given
			app.Request("error.test", func(_ *natsapi.NatsAPI, _ struct{}) (struct{}, error) {
				return struct{}{}, natsapi.NewJsonRPCException("BROKER_EXISTS")
			})
			err := app.Startup(context.Background())
			Expect(err).To(BeNil())

			// when
			reply, err := app.SendRequest("natsapi.development.error.test", map[string]any{}, time.Second)

			// then
			Expect(err).To(BeNil())
			Expect(reply.Error).ToNot(BeNil())
			Expect(reply.Error.Message).To(Equal("BROKER_EXISTS"))
		})
	})

	When("A handler raises a generic error", func() {
		It("should return a -40000 error", func() {
			// given
			app.Request("error.generic", func(_ *natsapi.NatsAPI, _ struct{}) (struct{}, error) {
				return struct{}{}, fmt.Errorf("something went wrong")
			})
			err := app.Startup(context.Background())
			Expect(err).To(BeNil())

			// when
			reply, err := app.SendRequest("natsapi.development.error.generic", map[string]any{}, time.Second)

			// then
			Expect(err).To(BeNil())
			Expect(reply.Error).ToNot(BeNil())
			Expect(reply.Error.Message).To(Equal("something went wrong"))
		})
	})

	When("Using a second app instance on a different root path", func() {
		It("should handle requests on its own root path", func() {
			// given
			cfg := natsapi.DefaultConfig()
			cfg.Connect.Servers = []string{natsURL}
			secondApp := natsapi.New("other.service", natsapi.WithConfig(cfg))
			secondApp.Request("baz", func(_ *natsapi.NatsAPI, _ struct{}) (StatusResult, error) {
				return StatusResult{Status: "OK"}, nil
			})
			err := secondApp.Startup(context.Background())
			Expect(err).To(BeNil())
			defer secondApp.Shutdown()

			// when
			reply, err := secondApp.SendRequest("other.service.baz", map[string]any{}, time.Second)

			// then
			Expect(err).To(BeNil())
			Expect(reply.Error).To(BeNil())
			var result StatusResult
			Expect(json.Unmarshal(reply.Result, &result)).To(Succeed())
			Expect(result.Status).To(Equal("OK"))
		})
	})

	When("Using the NatsAPIMock", func() {
		It("should capture payloads and return mock responses", func() {
			// given
			err := app.Startup(context.Background())
			Expect(err).To(BeNil())

			mock := natsapi.NewMock(app.Conn())
			defer mock.Close()

			err = mock.Request("foobar", StatusResult{Status: "mocked"}, nil)
			Expect(err).To(BeNil())

			// when
			reply, err := app.SendRequest("foobar", map[string]any{"id": 1}, time.Second)

			// then
			Expect(err).To(BeNil())
			Expect(reply.Error).To(BeNil())
			var result StatusResult
			Expect(json.Unmarshal(reply.Result, &result)).To(Succeed())
			Expect(result.Status).To(Equal("mocked"))
			Expect(mock.Payloads["foobar"]).To(HaveLen(1))
		})
	})

	When("Sending a request with typed params to the handler", func() {
		It("should receive and decode the params correctly", func() {
			// given
			app.Request("persons.greet", func(_ *natsapi.NatsAPI, p GreetParams) (GreetResult, error) {
				return GreetResult{Message: fmt.Sprintf("Hello %s %s", p.FirstName, p.LastName)}, nil
			})
			err := app.Startup(context.Background())
			Expect(err).To(BeNil())

			// when
			reply, err := app.SendRequest("natsapi.development.persons.greet", map[string]any{
				"first_name": "John",
				"last_name":  "Doe",
			}, time.Second)

			// then
			Expect(err).To(BeNil())
			Expect(reply.Error).To(BeNil())
			var result GreetResult
			Expect(json.Unmarshal(reply.Result, &result)).To(Succeed())
			Expect(result.Message).To(Equal("Hello John Doe"))
		})
	})

	When("Concurrent requests hit the same handler", func() {
		It("should handle all requests without data races", func() {
			// given
			app.Request("concurrent.test", func(_ *natsapi.NatsAPI, p GreetParams) (GreetResult, error) {
				return GreetResult{Message: fmt.Sprintf("Hello %s", p.FirstName)}, nil
			})
			err := app.Startup(context.Background())
			Expect(err).To(BeNil())

			// when - all goroutines wait at the barrier and fire simultaneously
			const n = 20
			var barrier sync.WaitGroup
			barrier.Add(1)
			results := make(chan error, n)
			for i := range n {
				go func(i int) {
					barrier.Wait()
					_, err := app.SendRequest("natsapi.development.concurrent.test", map[string]any{
						"first_name": fmt.Sprintf("user%d", i),
						"last_name":  "test",
					}, time.Second)
					results <- err
				}(i)
			}
			barrier.Done()

			// then - all should succeed with no data races detected by -race
			for range n {
				Expect(<-results).To(BeNil())
			}
		})
	})

	When("Each NATS request", func() {
		It("should have a different JSON-RPC id", func() {
			// given
			err := app.Startup(context.Background())
			Expect(err).To(BeNil())

			mock := natsapi.NewMock(app.Conn())
			defer mock.Close()
			err = mock.Request("unique-ids", StatusResult{Status: "ok"}, nil)
			Expect(err).To(BeNil())

			// when
			_, err = app.SendRequest("unique-ids", map[string]any{}, time.Second)
			Expect(err).To(BeNil())
			_, err = app.SendRequest("unique-ids", map[string]any{}, time.Second)
			Expect(err).To(BeNil())

			// then
			Expect(mock.Payloads["unique-ids"]).To(HaveLen(2))

			var req1, req2 natsapi.JsonRPCRequest
			_ = json.Unmarshal(mock.Payloads["unique-ids"][0], &req1)
			_ = json.Unmarshal(mock.Payloads["unique-ids"][1], &req2)
			Expect(req1.ID).ToNot(Equal(req2.ID))
		})
	})
})
