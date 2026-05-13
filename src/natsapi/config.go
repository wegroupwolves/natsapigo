package natsapi

import "time"

type ConnectConfig struct {
	Servers             []string
	Name                string
	AllowReconnect      bool
	MaxReconnectAttempts int
	ReconnectWait       time.Duration
	ConnectTimeout      time.Duration
	PingInterval        time.Duration
	MaxPingsOutstanding int
	NKeysSeed           string
	UserCredentials     string
}

type SubscribeConfig struct {
	Queue string
}

type Config struct {
	Connect   ConnectConfig
	Subscribe SubscribeConfig
}

func DefaultConfig() Config {
	return Config{
		Connect: ConnectConfig{
			Servers:             []string{"nats://127.0.0.1:4222"},
			AllowReconnect:      true,
			MaxReconnectAttempts: 60,
			ReconnectWait:       2 * time.Second,
			ConnectTimeout:      2 * time.Second,
			PingInterval:        2 * time.Minute,
			MaxPingsOutstanding: 2,
		},
		Subscribe: SubscribeConfig{
			Queue: "",
		},
	}
}
