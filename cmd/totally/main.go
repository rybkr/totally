package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rybkr/totally/internal/cli"
)

func main() {
	cmd := cli.NewRootCommand(os.Stdout, os.Stderr)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "totally:", err)
		os.Exit(1)
	}
}
