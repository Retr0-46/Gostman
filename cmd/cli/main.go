package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"gostman/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cmd := cli.NewRootCmd()
	cmd.SetArgs(cli.NormalizeArgs(os.Args[1:]))
	if err := cmd.ExecuteContext(ctx); err != nil {
		if !errors.Is(err, cli.ErrRunFailed) {
			fmt.Fprintln(os.Stderr, "Error:", err)
		}
		os.Exit(1)
	}
}
