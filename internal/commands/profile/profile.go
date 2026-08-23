package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/uvasoftware/scanii-cli/internal/buildinfo"
	"github.com/uvasoftware/scanii-cli/internal/client"
	"github.com/uvasoftware/scanii-cli/internal/terminal"
)

// Command returns the profile cobra command.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Configure local settings",
		Long:  "Manage multiple CLI profiles with different API credentials and endpoints",
	}

	cmd.AddCommand(createCommand())
	cmd.AddCommand(listCommand())
	cmd.AddCommand(deleteCommand())

	return cmd
}

func createCommand() *cobra.Command {
	var endpoint, credentials string

	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create or update a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}

			config := &Profile{}

			if endpoint == "" && credentials == "" {
				terminal.Section("Configuring profile: " + name)

				// prompt for credentials
				terminal.Info("Please enter the credentials you would like to use in the format key:secret")
				credentials = terminal.ReadLine("> ")
				if strings.TrimSpace(credentials) == "" {
					return fmt.Errorf("credentials cannot be empty")
				}
				if !strings.Contains(credentials, ":") {
					return fmt.Errorf("credentials must be in the format key:secret")
				}
				config.Credentials = credentials

				// prompt for endpoint
				availableEndpoints := []string{
					"api-us1.scanii.com",
					"api-eu1.scanii.com",
					"api-eu2.scanii.com",
					"api-ap1.scanii.com",
					"api-ap2.scanii.com",
					"api-ca1.scanii.com",
					"localhost:4000",
				}

				terminal.Info("Please select an endpoint from the options below:")
				terminal.List(availableEndpoints)

				selection := terminal.ReadLine("> ")
				id, err := strconv.Atoi(strings.TrimSpace(selection))
				if err != nil || id < 1 || id > len(availableEndpoints) {
					return fmt.Errorf("invalid endpoint selected: %s", selection)
				}
				config.Endpoint = availableEndpoints[id-1]
			} else {
				// non-interactive mode: default to "default" if no name provided
				if name == "" {
					name = DefaultProfileName
				}
				if endpoint != "" {
					config.Endpoint = endpoint
				}
				if credentials != "" {
					config.Credentials = credentials
				}
			}

			err := save(name, config)
			if err != nil {
				return err
			}
			terminal.Success(fmt.Sprintf("Profile %q saved", name))
			return nil
		},
	}

	cmd.Flags().StringVarP(&endpoint, "endpoint", "e", "", "Endpoint to use, see https://docs.scanii.com/article/161-endpoints-and-regions")
	cmd.Flags().StringVarP(&credentials, "credentials", "c", "", "API credentials to use in the format key:secret")

	return cmd
}

func listCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list [name]",
		Short: "List all profiles or show details for a specific profile",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				name := args[0]
				config, err := Load(name)
				if err != nil {
					return fmt.Errorf("profile %q not found: %w", name, err)
				}

				terminal.Section("Profile: " + name)
				terminal.KeyValue("Endpoint:", config.Endpoint)
				terminal.KeyValue("Credentials:", config.Credentials)
				terminal.KeyValue("Created At:", config.CreatedAt.Local().Format(time.RFC1123))
				return nil
			}

			dir, err := ConfigDir()
			if err != nil {
				return err
			}

			entries, err := os.ReadDir(dir)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println("No profiles found")
					return nil
				}
				return err
			}

			found := false
			terminal.Section("Available profiles")
			var profiles []string
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
					continue
				}
				name := strings.TrimSuffix(entry.Name(), ".json")
				config, err := Load(name)
				if err != nil {
					terminal.Warn(fmt.Sprintf("%s (error: %s)", name, err))
					continue
				}
				profiles = append(profiles, fmt.Sprintf("%s (endpoint=%s key=%s)", terminal.ToString(terminal.Bold, name), config.Endpoint, config.APIKey()))
				found = true
			}

			terminal.List(profiles)

			if !found {
				fmt.Println("No profiles found")
			}
			return nil
		},
	}
}

func deleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			p, err := ConfigPath(name)
			if err != nil {
				return err
			}

			if _, err := os.Stat(p); os.IsNotExist(err) {
				return fmt.Errorf("profile %q not found", name)
			}

			err = os.Remove(p)
			if err != nil {
				return err
			}

			terminal.Success(fmt.Sprintf("Profile %q deleted", name))
			return nil
		},
	}
}

const DefaultProfileName = "default"

// Profile represents a CLI profile's settings.
type Profile struct {
	Endpoint    string    `json:"endpoint"`
	CreatedAt   time.Time `json:"createdAt"`
	Version     *string   `json:"version"`
	Credentials string    `json:"credentials"`
}

// ApiKey returns the key portion of the credentials (before the colon).
func (c *Profile) APIKey() string {
	parts := strings.SplitN(c.Credentials, ":", 2)
	return parts[0]
}

// ApiSecret returns the secret portion of the credentials (after the colon).
func (c *Profile) APISecret() string {
	parts := strings.SplitN(c.Credentials, ":", 2)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// Option configures the client a profile builds.
type Option func(*clientConfig)

type clientConfig struct {
	maxIdleConns int
}

// WithMaxConcurrency sizes the connection pool to the number of requests the
// caller intends to have in flight at once.
//
// Without it the transport keeps Go's default of two idle connections per host,
// so a run that uploads dozens of files at a time closes all but two of its
// connections the moment they go idle and opens fresh ones — paying a DNS
// lookup, a TCP connect and a TLS handshake per file that the API never sees in
// its own timings.
func WithMaxConcurrency(n int) Option {
	return func(cfg *clientConfig) {
		if n < 1 {
			return
		}
		cfg.maxIdleConns = n
	}
}

func (c *Profile) Client(opts ...Option) (*client.Client, error) {
	dest := fmt.Sprintf("https://%s/v2.2", c.Endpoint)
	if strings.HasPrefix(c.Endpoint, "localhost") {
		dest = fmt.Sprintf("http://%s/v2.2", c.Endpoint)
	}

	// a command that makes one request has nothing to pool, so the default is
	// what the transport would have used anyway
	cfg := clientConfig{maxIdleConns: http.DefaultMaxIdleConnsPerHost}
	for _, o := range opts {
		o(&cfg)
	}

	return client.New(dest,
		client.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
			req.SetBasicAuth(c.APIKey(), c.APISecret())
			req.Header.Set("User-Agent", buildinfo.UserAgent())
			return nil
		}),
		// The Scanii API has a maximum processing time of 30 minutes per request.
		// We use transport-level timeouts instead of http.Client.Timeout so that
		// the upload transfer time is not counted against the deadline.
		client.WithHTTPClient(&http.Client{
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: 30 * time.Second}).DialContext,
				TLSHandshakeTimeout:   15 * time.Second,
				ResponseHeaderTimeout: 30 * time.Minute,
				// there is only ever one host in play, so the two limits are the
				// same number — capping the total below the per-host figure would
				// quietly undo it
				MaxIdleConns:        cfg.maxIdleConns,
				MaxIdleConnsPerHost: cfg.maxIdleConns,
			},
		}),
	)
}

// ConfigDir returns the base directory for storing profiles.
func ConfigDir() (string, error) {
	dirname, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dirname, ".config", "scanii-cli"), nil
}

// ConfigPath returns the full path to a profile's JSON file.
func ConfigPath(profileName string) (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, profileName+".json"), nil
}

// EnsureConfigDir creates the config directory if it does not exist.
func EnsureConfigDir() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}

// Load reads and parses a profile by name.
func Load(profileName string) (*Profile, error) {
	config := &Profile{}

	cp, err := ConfigPath(profileName)
	if err != nil {
		return nil, err
	}

	slog.Debug("loading config", "path", cp, "profile", profileName)

	fd, err := os.Open(cp)
	if err != nil {
		if os.IsNotExist(err) {
			if profileName == DefaultProfileName {
				return nil, fmt.Errorf("no default profile configured, create one with: sc profile create")
			}
			return nil, fmt.Errorf("profile %q not found, create one with: sc profile create %s", profileName, profileName)
		}
		return nil, err
	}
	defer fd.Close()

	contents, err := io.ReadAll(fd)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(contents, &config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// save writes a profile to disk, setting the CreatedAt timestamp.
func save(profileName string, config *Profile) error {
	config.CreatedAt = time.Now()

	if err := EnsureConfigDir(); err != nil {
		return err
	}

	cp, err := ConfigPath(profileName)
	if err != nil {
		return err
	}

	js, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	err = os.WriteFile(cp, js, 0600)
	if err != nil {
		return err
	}

	slog.Debug("config saved", "path", cp, "profile", profileName)
	return nil
}
