package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/azohra/gopro-yank/internal/app"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(app.Main(ctx, os.Args[1:], version))
}
