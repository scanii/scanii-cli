package testutil

import (
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"time"

	"github.com/uvasoftware/scanii-cli/internal/commands/profile"
	"github.com/uvasoftware/scanii-cli/internal/commands/server"
	"github.com/uvasoftware/scanii-cli/internal/log"
)

const (
	Key    = "key"
	Secret = "secret"
)

// Server holds the test server configuration.
type Server struct {
	Profile  *profile.Profile
	Endpoint string
}

// StartServer starts a mock Scanii server on a random port and returns
// a Server with the profile and endpoint configured for testing.
func StartServer() *Server {
	handler := log.NewConsoleLogHandler(os.Stdout, &log.Options{Level: slog.LevelDebug, AddSource: true})
	slog.SetDefault(slog.New(handler))

	endpoint := fmt.Sprintf("localhost:%d", 20_000+rand.Intn(1000)) //nolint:gosec
	ready := make(chan bool)
	go func() {
		// panic rather than let <-ready block forever on a server that never started
		if err := server.RunServer(&server.Flags{
			Key:       Key,
			Secret:    Secret,
			Address:   endpoint,
			ReadyChan: ready,
		}); err != nil {
			panic(err)
		}
	}()
	<-ready

	return &Server{
		Profile: &profile.Profile{
			CreatedAt:   time.Now(),
			Credentials: Key + ":" + Secret,
			Endpoint:    endpoint,
		},
		Endpoint: endpoint,
	}
}
