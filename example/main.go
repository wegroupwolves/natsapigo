package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/wegroupwolves/natsapigo/src/natsapi"
)

// --- Domain types ---

type GreetParams struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Birth     string `json:"birth"`
}

type GreetResult struct {
	Message string `json:"message"`
}

type StatusResult struct {
	Status string `json:"status"`
}

type CountResult struct {
	Count int `json:"count"`
}

type ActivityEvent struct {
	UserID string `json:"user_id"`
	Action string `json:"action"`
}

// --- Handlers ---

func handleHealth(_ *natsapi.NatsAPI, _ struct{}) (StatusResult, error) {
	return StatusResult{Status: "OK"}, nil
}

func handleGreet(_ *natsapi.NatsAPI, p GreetParams) (GreetResult, error) {
	if p.FirstName == "" || p.LastName == "" {
		return GreetResult{}, natsapi.NewJsonRPCException("INVALID_PARAMETERS_RECEIVED", natsapi.ErrorDetail{Message: "first_name and last_name are required"})
	}
	return GreetResult{Message: fmt.Sprintf("Hello %s %s!", p.FirstName, p.LastName)}, nil
}

func handleCount(a *natsapi.NatsAPI, _ struct{}) (CountResult, error) {
	count, _ := a.State["user_count"].(int)
	return CountResult{Count: count}, nil
}

func handleActivity(_ *natsapi.NatsAPI, e ActivityEvent) error {
	slog.Info("Activity logged", "user_id", e.UserID, "action", e.Action)
	return nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg := natsapi.DefaultConfig()
	if url := os.Getenv("NATS_URL"); url != "" {
		cfg.Connect.Servers = []string{url}
	}

	app := natsapi.New("myservice",
		natsapi.WithConfig(cfg),
		natsapi.WithInfo("My Service", "1.0.0"),
		natsapi.WithAsyncAPIDocServer(":8090"),
		natsapi.WithOnStartup(func() error {
			slog.Info("App started")
			return nil
		}),
		natsapi.WithOnShutdown(func() error {
			slog.Info("App stopped")
			return nil
		}),
	)

	app.State["user_count"] = 42

	app.Request("health.RETRIEVE", handleHealth)

	usersRouter := natsapi.NewSubjectRouter("users")
	usersRouter.Request("greet.RETRIEVE", handleGreet)
	usersRouter.Request("count.RETRIEVE", handleCount)
	usersRouter.Publish("activity.LOG", handleActivity)
	app.IncludeRouter(usersRouter)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := app.Startup(ctx); err != nil {
		slog.Error("Failed to start", "err", err)
		os.Exit(1)
	}

	slog.Info("Listening on myservice.>  — press Ctrl+C to stop")
	<-ctx.Done()

	if err := app.Shutdown(); err != nil {
		slog.Error("Shutdown error", "err", err)
	}
}
