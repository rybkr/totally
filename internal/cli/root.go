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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return loadGlobalOptions(cmd, &opts)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.CompletionOptions.DisableDefaultCmd = true
	addGlobalFlags(cmd, &opts)
	cmd.AddCommand(newFilesCommand(stdout, &opts))
	cmd.AddCommand(newShowCommand(stdout, &opts))
	cmd.AddCommand(newSessionsCommand(stdout, &opts))
	cmd.AddCommand(newPricesCommand(stdout, &opts))
	return cmd
}
