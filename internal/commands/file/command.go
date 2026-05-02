package file

import (
	"context"

	"github.com/spf13/cobra"
)

// Command returns the files cobra command with all subcommands.
func Command(ctx context.Context, profile *string) *cobra.Command {
	var metadata string

	parent := cobra.Command{
		Use:   "files",
		Short: "API operations for the files resource",
		Long:  `Files API operations. Detailed API documentation can be found here: https://uvasoftware.github.io/openapi/v22/#/Files`,
	}

	parent.PersistentFlags().StringVarP(&metadata, "metadata", "m", "", "Metadata in the format key=value,key2=value2 to be associated with the request")

	parent.AddCommand(processCommand(ctx, profile, &metadata))
	parent.AddCommand(asyncCommand(ctx, profile, &metadata))
	parent.AddCommand(fetchCommand(ctx, profile, &metadata))
	parent.AddCommand(retrieveCommand(ctx, profile))
	parent.AddCommand(traceCommand(ctx, profile))

	return &parent
}
