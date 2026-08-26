package app

import (
	"context"
	"log/slog"
)

type App struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) *App {
	return &App{
		logger: logger,
	}
}

func (a *App) Run(ctx context.Context) error {
	a.logger.Info("runtime starting")

	<-ctx.Done()

	a.logger.Info(
		"runtime stopping",
		"reason", ctx.Err(),
	)

	return nil
}
