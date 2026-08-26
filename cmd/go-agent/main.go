package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Methamorphe/go-agent/internal/app"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	logger := slog.New(
		slog.NewTextHandler(os.Stderr, nil),
	)

	application := app.New(logger)

	if err := application.Run(ctx); err != nil {
		logger.Error(
			"application stopped with an error",
			"error", err,
		)

		return 1
	}

	return 0
}
