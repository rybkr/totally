package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rybkr/totally/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd := cli.NewRootCommand(os.Stdout, os.Stderr)
	cmd.Version = buildVersion()
	cmd.SetVersionTemplate("totally {{.Version}}\n")
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "totally:", err)
		os.Exit(cli.ExitCode(err))
	}
}

func buildVersion() string {
	return fmt.Sprintf("%s (%s) built %s", version, commit, date)
}
