package main

import (
	"context"

	"github.com/google/gops/agent"
	"github.com/spf13/cobra"
	"github.com/uvasoftware/scanii-cli/internal/buildinfo"
	"github.com/uvasoftware/scanii-cli/internal/commands/account"
	"github.com/uvasoftware/scanii-cli/internal/commands/authtoken"
	"github.com/uvasoftware/scanii-cli/internal/commands/file"
	"github.com/uvasoftware/scanii-cli/internal/commands/ping"
	"github.com/uvasoftware/scanii-cli/internal/commands/profile"
	"github.com/uvasoftware/scanii-cli/internal/commands/server"
	"github.com/uvasoftware/scanii-cli/internal/log"
	"github.com/uvasoftware/scanii-cli/internal/terminal"

	"log/slog"
	"os"
	"runtime/debug"
)

var (
	verbose    bool
	profileArg string
)

func main() {

	cobra.EnableCommandSorting = false

	rootCmd := &cobra.Command{
		Use:     "sc",
		Version: buildinfo.Version(),
		Short:   "Scanii CLI",
		Long:    "A CLI to help you integrate Scanii (https://www.scanii.com) with your application",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// the console handler is installed either way, so that the info-level
			// lines a normal run emits — request ids, most usefully — are styled
			// like the rest of the CLI rather than by slog's default handler
			level := slog.LevelInfo
			if verbose {
				level = slog.LevelDebug
			}
			handler := log.NewConsoleLogHandler(os.Stdout, &log.Options{Level: level, AddSource: verbose, Color: terminal.IsTTY()})
			slog.SetDefault(slog.New(handler))
			slog.Debug("running in debug mode")

			cmd.SilenceUsage = true
			cmd.SilenceErrors = true

			if err := agent.Listen(agent.Options{
				ShutdownCleanup: true,
			}); err != nil {
				panic(err)
			}

		},
	}

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Display app and runtime information",
		Run: func(_ *cobra.Command, _ []string) {
			bi, _ := debug.ReadBuildInfo()
			terminal.Section("Scanii CLI (https://www.scanii.com)")
			terminal.KeyValue("Version:", buildinfo.Version())
			terminal.KeyValue("Date:", buildinfo.Date())
			terminal.KeyValue("Go Version:", bi.GoVersion)
			terminal.Section("Build settings")
			for _, e := range bi.Settings {
				terminal.KeyValue(e.Key+":", e.Value)
			}
		},
	}

	ctx := context.Background()

	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringVarP(&profileArg, "profile", "p", "default", "profile to use")
	rootCmd.AddCommand(profile.Command())
	rootCmd.AddCommand(account.Command(ctx, &profileArg))
	rootCmd.AddCommand(file.Command(ctx, &profileArg))
	rootCmd.AddCommand(authtoken.Command(ctx, &profileArg))
	rootCmd.AddCommand(ping.Command(ctx, &profileArg))
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(server.Command())

	err := rootCmd.Execute()
	if err != nil {
		terminal.Error(err.Error())
		os.Exit(1)
	}
}
