package cli

import (
	"io"

	"github.com/spf13/cobra"
)

func NewRootCommand(stdout io.Writer, stderr io.Writer) *cobra.Command {
	var opts globalOptions

	cmd := &cobra.Command{
		Use:           "totally",
		Short:         "Analyze local agent session files",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	addGlobalFlags(cmd, &opts)
	cmd.AddCommand(newFilesCommand(stdout, &opts))
	return cmd
}
