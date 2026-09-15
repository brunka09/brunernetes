// Command brunerctl operates a single-node Brunernetes factory.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/brunka09/brunernetes/internal/cli"
)

// version is stamped at build time with -ldflags.
var version = "dev"

func main() {
	cli.Version = version

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cli.NewRootCommand().ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
