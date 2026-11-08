package server

import (
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
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
	Address   string
	Engine    string
	Key       string
	Secret    string
	Data      string
	ReadyChan chan bool
	// CallBackWait overrides the engine's callback delay. Nil means the engine
	// config decides, which is what keeps an unpassed flag from overriding a
	// value set in an --engine file.
	CallBackWait *time.Duration
}

// RunServer starts the mock Scanii server. This function blocks until the
// server stops, and returns an error if it could not be started.
func RunServer(flags *Flags) error {
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
		if err := os.MkdirAll(os.TempDir(), 0755); err != nil { // #nosec G301 -- the system temp directory, created only where a minimal image lacks one
			return fmt.Errorf("creating temp directory: %w", err)
		}
		dir, err := os.MkdirTemp("", "scanii-cli")
		if err != nil {
			return fmt.Errorf("creating storage directory: %w", err)
		}
		flags.Data = dir
	} else {
		if _, err := os.Stat(flags.Data); errors.Is(err, os.ErrNotExist) {
			if err := os.MkdirAll(flags.Data, 0755); err != nil { // #nosec G301 -- result storage the user chose with --data; the result files are 0600
				return fmt.Errorf("creating storage directory %s: %w", flags.Data, err)
			}
		}
	}

	mux := http.NewServeMux()

	eng, err := engine.New()
	if err != nil {
		return fmt.Errorf("creating engine: %w", err)
	}

	// a bad --engine config is worth failing on: starting anyway would serve the
	// built-in rules while looking like it had honored the flag
	if flags.Engine != "" {
		if err := loadEngineConfig(eng, flags.Engine); err != nil {
			return err
		}
		slog.Debug("loaded engine config", "path", flags.Engine, "rules", eng.RuleCount())
	}

	// applied after the config file so that an explicit flag wins over it
	if flags.CallBackWait != nil {
		eng.SetCallbackWait(*flags.CallBackWait)
	}

	storage, err := Setup(mux, eng, flags.Key, flags.Secret, flags.Data, "http://"+flags.Address)
	if err != nil {
		return err
	}
	defer func() { _ = storage.Close() }()

	// wrap the mux with request logging middleware
	logger := httplog.NewLogger("sc", httplog.Options{
		LogLevel:         slog.LevelDebug,
		Concise:          true,
		RequestHeaders:   true,
		MessageFieldName: "message",
		TimeFieldFormat:  time.DateTime,
	})
	handler := CORS(httplog.RequestLogger(logger)(mux))

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
	terminal.KeyValue("Callback Wait:", eng.CallbackWait().String())
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

	listeners, err := listen(flags.Address)
	if err != nil {
		return err
	}

	if flags.ReadyChan != nil {
		flags.ReadyChan <- true
	}

	errs := make(chan error, len(listeners))
	for _, l := range listeners {
		go func() { errs <- srv.Serve(l) }()
	}
	// the first listener to fail takes the others down with it
	err = <-errs
	_ = srv.Close()
	return fmt.Errorf("serving: %w", err)
}

// listen binds address. A "localhost" host binds both loopback addresses, so
// that clients which resolve localhost to ::1 first and do not fall back to
// IPv4 still connect; net.Listen alone would bind only one of them. The IPv6
// one is best effort, for hosts without IPv6 loopback.
func listen(address string) ([]net.Listener, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host != "localhost" {
		l, err := net.Listen("tcp", address)
		if err != nil {
			return nil, fmt.Errorf("listening on %s: %w", address, err)
		}
		return []net.Listener{l}, nil
	}

	v4, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		return nil, fmt.Errorf("listening on %s: %w", address, err)
	}
	// a port of 0 picks one on IPv4; IPv6 has to use the same
	port = strconv.Itoa(v4.Addr().(*net.TCPAddr).Port)
	v6, err := net.Listen("tcp", net.JoinHostPort("::1", port))
	if err != nil {
		slog.Debug("not listening on IPv6 loopback", "error", err)
		return []net.Listener{v4}, nil
	}
	return []net.Listener{v4, v6}, nil
}

// loadEngineConfig replaces the engine's built-in rules with the config file at
// path. Unknown fields are rejected by the decoder, so a file that is valid JSON
// but not an engine config is reported rather than silently ignored.
func loadEngineConfig(eng *engine.Engine, path string) error {
	file, err := os.Open(path) // #nosec G304 -- the --engine config the user named
	if err != nil {
		return fmt.Errorf("opening engine config: %w", err)
	}
	defer func() { _ = file.Close() }()

	if err := eng.LoadConfig(file); err != nil {
		return fmt.Errorf("loading engine config %s: %w", path, err)
	}

	return nil
}

// serverEICAR decodes the embedded base64 EICAR payload and returns it
// as text/plain. Keeping the signature off-disk prevents AV engines
// from deleting it between builds.
func serverEICAR(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(assets.DecodedEICAR()))
}

// defaultAddress is where the server listens unless --address or the
// addressEnv variable says otherwise: the IPv4 and IPv6 loopback addresses
// only, so that a server started for local testing is not reachable from the
// network.
const defaultAddress = "localhost:4000"

// addressEnv overrides defaultAddress. The container image sets it to
// 0.0.0.0:4000, so that `docker run ... server` keeps answering on its
// published port.
const addressEnv = "SC_SERVER_ADDRESS"

// Command returns the server cobra command.
func Command() *cobra.Command {
	serverF := Flags{}
	var callbackWait time.Duration
	serverCmd := &cobra.Command{
		Use: "server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// only override the engine config when the flag was actually passed
			if cmd.Flags().Changed("callback-wait") {
				serverF.CallBackWait = &callbackWait
			}
			return RunServer(&serverF)
		},
		Short: "Start a mock server suitable for testing purposes",
	}

	address := defaultAddress
	if v := os.Getenv(addressEnv); v != "" {
		address = v
	}
	serverCmd.PersistentFlags().StringVarP(&serverF.Address, "address", "a", address, "Address to listen on, defaults to $"+addressEnv+" when set")
	serverCmd.PersistentFlags().StringVarP(&serverF.Engine, "engine", "e", "", "Optional engine config to load")
	serverCmd.PersistentFlags().DurationVarP(&callbackWait, "callback-wait", "w", 100*time.Millisecond, "Amount of time a callback should wait before firing, overrides the engine config")
	serverCmd.PersistentFlags().StringVarP(&serverF.Data, "data", "d", "", "Result storage path, defaults to a temp directory")
	serverCmd.PersistentFlags().StringVarP(&serverF.Key, "key", "k", "key", "API key to use, pass an empty value to generate one")
	serverCmd.PersistentFlags().StringVarP(&serverF.Secret, "secret", "s", "secret", "API secret to use, pass an empty value to generate one")

	return serverCmd
}
