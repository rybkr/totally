package cli

import (
	"io"

	"github.com/spf13/cobra"
)

func NewRootCommand(stdout io.Writer, stderr io.Writer) *cobra.Command {
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
	cmd.AddCommand(newFilesCommand(stdout))
	return cmd
}
