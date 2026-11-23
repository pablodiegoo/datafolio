package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/pablodiegoo/datafolio/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	os.Exit(cli.Run(ctx, os.Args[1:], cli.DefaultOptions()))
}
