package testutil

import (
	"fmt"
	"log/slog"
	"net"
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

// freePort returns a loopback address the OS has just confirmed is free.
//
// The port used to be picked at random out of a thousand, which `go test ./...`
// collides on sooner or later: it runs one process per package, several of them
// start a server, and two of them drawing the same number fails the run with
// "address already in use". Asking the OS for a free port and handing it
// straight to the server closes all but a sliver of that window — RunServer
// needs the address before it binds, so the listener cannot simply be passed in.
func freePort() string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic("reserving a port: " + err.Error())
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		panic("releasing the reserved port: " + err.Error())
	}
	return fmt.Sprintf("localhost:%d", port)
}

// StartServer starts a mock Scanii server on a free port and returns
// a Server with the profile and endpoint configured for testing.
func StartServer() *Server {
	handler := log.NewConsoleLogHandler(os.Stdout, &log.Options{Level: slog.LevelDebug, AddSource: true})
	slog.SetDefault(slog.New(handler))

	endpoint := freePort()
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
