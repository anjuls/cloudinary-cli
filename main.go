// Package main provides the signal-aware entrypoint for the Cloudinary CLI.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/anjuls/cloudinary-cli/internal/cli"
)

// main builds a signal-aware context, constructs the CLI application, and
// exits with the result of Execute. Interrupt and SIGTERM are captured so
// ongoing uploads can abort cleanly via context cancellation.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(cli.NewApp().Execute(ctx, os.Args[1:]))
}
