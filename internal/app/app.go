package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/config"
	"github.com/Methamorphe/go-agent/internal/control"
	"github.com/Methamorphe/go-agent/internal/control/protocol"
	"github.com/Methamorphe/go-agent/internal/diag"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/objectstore"
	"github.com/Methamorphe/go-agent/internal/storage/sqlite"
)

type App struct {
	logger *slog.Logger
	cfg    config.Config
	clock  clock.Clock
}

func New(
	logger *slog.Logger,
	cfg config.Config,
) *App {
	return &App{
		logger: logger,
		cfg:    cfg,
		clock:  clock.NewSystemClock(),
	}
}

func NewWithClock(
	logger *slog.Logger,
	cfg config.Config,
	clock clock.Clock,
) *App {
	return &App{
		logger: logger,
		cfg:    cfg,
		clock:  clock,
	}
}

func (a *App) Run(
	ctx context.Context,
) error {
	if err := os.MkdirAll(
		a.cfg.DataDir,
		0o700,
	); err != nil {
		return fmt.Errorf(
			"create data directory: %w",
			err,
		)
	}

	store, err := sqlite.Open(
		ctx,
		sqlite.Config{
			Path:         a.cfg.DatabasePath(),
			BusyTimeout:  a.cfg.SQLiteBusyTimeout(),
			MaxOpenConns: a.cfg.SQLiteMaxOpenConns,
			Clock:        a.clock,
		},
	)

	if err != nil {
		if ctx.Err() != nil &&
			(errors.Is(err, context.Canceled) ||
				errors.Is(err, context.DeadlineExceeded)) {
			a.logger.Info(
				"runtime startup cancelled",
				"reason", ctx.Err(),
			)

			return nil
		}

		return err
	}

	defer store.Close()

	objects, err := objectstore.New(
		a.cfg.ObjectStorePath(),
	)

	if err != nil {
		return err
	}

	if _, err := objects.CleanupTemps(
		a.clock.Now(),
		24*time.Hour,
	); err != nil {
		a.logger.Warn(
			"temporary object cleanup failed",
			"error",
			err,
		)
	}

	profiler, err := diag.StartPprof(
		a.cfg.PprofAddress,
		a.logger,
	)

	if err != nil {
		return err
	}

	if profiler != nil {
		defer func() {
			shutdownCtx, cancel :=
				context.WithTimeout(
					context.Background(),
					2*time.Second,
				)

			defer cancel()

			if err := profiler.Close(
				shutdownCtx,
			); err != nil {
				a.logger.Warn(
					"pprof shutdown failed",
					"error",
					err,
				)
			}
		}()
	}

	listener, err := control.Listen(
		a.cfg.ControlAddress,
	)

	if err != nil {
		return err
	}

	handler := control.HandlerFunc(
		func(
			requestCtx context.Context,
			request protocol.Envelope,
		) (protocol.Envelope, error) {
			switch request.Type {
			case "ping":
				return response(
					request,
					map[string]any{
						"ok": true,
					},
				)

			case "health.get":
				if err := store.Ping(
					requestCtx,
				); err != nil {
					return protocol.Envelope{},
						err
				}

				return response(
					request,
					map[string]any{
						"ok": true,

						"runtime": diag.Snapshot(
							a.clock.Now(),
						),

						"storage": store.Stats(),
					},
				)

			default:
				return protocol.Envelope{},
					errs.New(
						errs.CodeNotFound,
						"control.handle",
						"unknown request type",
					)
			}
		},
	)

	server := control.NewServer(
		a.logger,
		handler,
		a.cfg.MaxFrameBytes,
		a.cfg.MaxControlConnections,
	)

	serveErr := make(
		chan error,
		1,
	)

	go func() {
		serveErr <- server.Serve(
			ctx,
			listener,
		)
	}()

	a.logger.Info(
		"runtime started",
		"data_dir",
		a.cfg.DataDir,
		"database",
		a.cfg.DatabasePath(),
		"control_address",
		a.cfg.ControlAddress,
	)

	select {
	case <-ctx.Done():
		if err := <-serveErr; err != nil {
			return err
		}

		shutdownCtx, cancel :=
			context.WithTimeout(
				context.Background(),
				3*time.Second,
			)

		defer cancel()

		if err := store.Checkpoint(
			shutdownCtx,
		); err != nil {
			a.logger.Warn(
				"WAL checkpoint during shutdown failed",
				"error",
				err,
			)
		}

		a.logger.Info(
			"runtime stopped",
			"reason",
			ctx.Err(),
		)

		return nil

	case err := <-serveErr:
		if err != nil {
			return err
		}

		if ctx.Err() != nil {
			return nil
		}

		return fmt.Errorf(
			"control server stopped unexpectedly",
		)
	}
}

func response(
	request protocol.Envelope,
	payload any,
) (protocol.Envelope, error) {
	body, err := json.Marshal(payload)

	if err != nil {
		return protocol.Envelope{},
			fmt.Errorf(
				"encode response payload: %w",
				err,
			)
	}

	return protocol.Envelope{
		Version:   protocol.Version,
		Kind:      protocol.KindResponse,
		Type:      request.Type,
		RequestID: request.RequestID,
		Payload:   body,
	}, nil
}
