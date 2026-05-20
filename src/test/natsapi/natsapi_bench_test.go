package natsapi_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/wegroupwolves/natsapigo/src/natsapi"
)

func BenchmarkRequestReply(b *testing.B) {
	server, natsURL := startEmbeddedNATS()
	defer server.Shutdown()

	app := newTestApp(natsURL)
	app.Request("bench.handler", func(_ *natsapi.NatsAPI, p GreetParams) (GreetResult, error) {
		return GreetResult{Message: fmt.Sprintf("Hello %s %s", p.FirstName, p.LastName)}, nil
	})
	if err := app.Startup(context.Background()); err != nil {
		b.Fatal(err)
	}
	defer app.Shutdown()

	params := map[string]any{"first_name": "John", "last_name": "Doe"}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, err := app.SendRequest("natsapi.development.bench.handler", params, time.Second)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRequestReplyParallel(b *testing.B) {
	server, natsURL := startEmbeddedNATS()
	defer server.Shutdown()

	app := newTestApp(natsURL)
	app.Request("bench.parallel", func(_ *natsapi.NatsAPI, p GreetParams) (GreetResult, error) {
		return GreetResult{Message: fmt.Sprintf("Hello %s %s", p.FirstName, p.LastName)}, nil
	})
	if err := app.Startup(context.Background()); err != nil {
		b.Fatal(err)
	}
	defer app.Shutdown()

	params := map[string]any{"first_name": "John", "last_name": "Doe"}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := app.SendRequest("natsapi.development.bench.parallel", params, time.Second)
			if err != nil {
				b.Error(err)
			}
		}
	})
}
