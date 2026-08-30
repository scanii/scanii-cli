package file

import (
	"context"

	"github.com/spf13/cobra"
)

// Command returns the files cobra command with all subcommands.
func Command(ctx context.Context, profile *string) *cobra.Command {
	var metadata string
	var perf bool

	parent := cobra.Command{
		Use:   "files",
		Short: "API operations for the files resource",
		Long:  `Files API operations. Detailed API documentation can be found here: https://uvasoftware.github.io/openapi/v22/#/Files`,
	}

	parent.PersistentFlags().StringVarP(&metadata, "metadata", "m", "", "Metadata in the format key=value,key2=value2 to be associated with the request")
	parent.PersistentFlags().BoolVar(&perf, "perf", false, "Print a timing breakdown of the API requests the command made")

	parent.AddCommand(processCommand(ctx, profile, &metadata, &perf))
	parent.AddCommand(asyncCommand(ctx, profile, &metadata, &perf))
	parent.AddCommand(fetchCommand(ctx, profile, &metadata, &perf))
	parent.AddCommand(retrieveCommand(ctx, profile, &perf))
	parent.AddCommand(deleteCommand(ctx, profile, &perf))
	parent.AddCommand(deleteTraceCommand(ctx, profile, &perf))
	parent.AddCommand(traceCommand(ctx, profile, &perf))

	return &parent
}
