package server

import (
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/httplog/v2"
	"github.com/spf13/cobra"
	"github.com/uvasoftware/scanii-cli/assets"
	"github.com/uvasoftware/scanii-cli/internal/engine"
	"github.com/uvasoftware/scanii-cli/internal/identifiers"
	"github.com/uvasoftware/scanii-cli/internal/terminal"
)

// Flags holds configuration for the mock server.
type Flags struct {
	Address      string
	Engine       string
	Key          string
	Secret       string
	Data         string
	ReadyChan    chan bool
	CallBackWait time.Duration
}

// RunServer starts the mock Scanii server. This function blocks.
func RunServer(flags *Flags) {
	if flags.Key == "" {
		terminal.Info("No API key provided, generating one...")
		flags.Key = fmt.Sprintf("akk_%s", identifiers.GenerateShort())
	}

	if flags.Secret == "" {
		terminal.Info("No API secret provided, generating one...")
		flags.Secret = fmt.Sprintf("aks_%s", identifiers.GenerateSecure())
	}

	if flags.Data == "" {
		// Ensure the system temp directory exists — minimal Docker images
		// (e.g. scratch, distroless) may not include /tmp.
		if err := os.MkdirAll(os.TempDir(), 0755); err != nil {
			panic(err)
		}
		dir, err := os.MkdirTemp("", "scanii-cli")
		if err != nil {
			panic(err)
		}
		flags.Data = dir
	} else {
		if _, err := os.Stat(flags.Data); errors.Is(err, os.ErrNotExist) {
			err := os.MkdirAll(flags.Data, 0755)
			if err != nil {
				panic(err)
			}
		}
	}

	mux := http.NewServeMux()

	eng, err := engine.New()
	if err != nil {
		slog.Error("could not create engine")
		os.Exit(2)
	}

	Setup(mux, eng, flags.Key, flags.Secret, flags.Data, "http://"+flags.Address)

	// wrap the mux with request logging middleware
	logger := httplog.NewLogger("sc", httplog.Options{
		LogLevel:         slog.LevelDebug,
		Concise:          true,
		RequestHeaders:   true,
		MessageFieldName: "message",
		TimeFieldFormat:  time.DateTime,
	})
	handler := httplog.RequestLogger(logger)(mux)

	srv := &http.Server{
		Addr:         flags.Address,
		Handler:      handler,
		IdleTimeout:  30 * time.Second,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		ErrorLog:     slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
	}
	slog.Debug("storage directory", "path", flags.Data)

	terminal.Title("Scanii local server starting")
	terminal.KeyValue("API Key:", flags.Key)
	terminal.KeyValue("API Secret:", flags.Secret)
	terminal.KeyValue("Engine Rules:", fmt.Sprintf("%d", eng.RuleCount()))
	//goland:noinspection HttpUrlsUsage
	terminal.KeyValue("Address:", fmt.Sprintf("http://%s", flags.Address))
	//goland:noinspection HttpUrlsUsage
	fmt.Println()
	terminal.Info(fmt.Sprintf("Sample usage: curl -u %s:%s http://%s/v2.2/ping", flags.Key, flags.Secret, flags.Address))
	terminal.Section("We also provide fake sample files you can use to trigger findings:")
	terminal.Info(fmt.Sprintf("∙ content.image.nsfw.nudity: http://%s/static/samples/image.jpg", flags.Address))
	terminal.Info(fmt.Sprintf("∙ content.en.language.nsfw.0: http://%s/static/samples/language.txt", flags.Address))
	terminal.Info(fmt.Sprintf("∙ content.malicious.local-test-file: http://%s/static/samples/malware", flags.Address))
	fmt.Println()
	terminal.Warn("This server is for testing purposes only files aren't really analyzed.")

	listen, err := net.Listen("tcp", flags.Address)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}

	if flags.ReadyChan != nil {
		flags.ReadyChan <- true
	}

	err = srv.Serve(listen)
	if err != nil {
		slog.Error("server error", "error", err)
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(3)
	}
}

// serverEICAR decodes the embedded base64 EICAR payload and returns it
// as text/plain. Keeping the signature off-disk prevents AV engines
// from deleting it between builds.
func serverEICAR(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(assets.DecodedEICAR()))
}

// Command returns the server cobra command.
func Command() *cobra.Command {
	serverF := Flags{}
	serverCmd := &cobra.Command{
		Use: "server",
		Run: func(cmd *cobra.Command, args []string) {
			RunServer(&serverF)
		},
		Short: "Start a mock server suitable for testing purposes",
	}

	serverCmd.PersistentFlags().StringVarP(&serverF.Address, "address", "a", "0.0.0.0:4000", "Address to listen on")
	serverCmd.PersistentFlags().StringVarP(&serverF.Engine, "engine", "e", "", "Optional engine config to load")
	serverCmd.PersistentFlags().DurationVarP(&serverF.CallBackWait, "callback-wait", "w", 100*time.Millisecond, "Amount of time a callback should wait before firing")
	serverCmd.PersistentFlags().StringVarP(&serverF.Data, "data", "d", "", "Result storage path, defaults to a temp directory")
	serverCmd.PersistentFlags().StringVarP(&serverF.Key, "key", "k", "key", "API key to use, if not provided will be dynamically generated")
	serverCmd.PersistentFlags().StringVarP(&serverF.Secret, "secret", "s", "secret", "API secret to use, if not provided will be dynamically generated")

	return serverCmd
}
