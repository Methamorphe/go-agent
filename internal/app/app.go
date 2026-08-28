package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Methamorphe/go-agent/internal/agent"
	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/config"
	"github.com/Methamorphe/go-agent/internal/control"
	"github.com/Methamorphe/go-agent/internal/diag"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/mmu"
	"github.com/Methamorphe/go-agent/internal/objectstore"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/storage/sqlite"
	"github.com/Methamorphe/go-agent/internal/supervisor"
)

type App struct {
	logger *slog.Logger
	cfg    config.Config
	clock  clock.Clock
}

func New(logger *slog.Logger, cfg config.Config) *App {
	return &App{logger: logger, cfg: cfg, clock: clock.NewSystemClock()}
}

func NewWithClock(logger *slog.Logger, cfg config.Config, source clock.Clock) *App {
	return &App{logger: logger, cfg: cfg, clock: source}
}

func (a *App) Run(ctx context.Context) error {
	if err := os.MkdirAll(a.cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	store, err := sqlite.Open(ctx, sqlite.Config{
		Path:         a.cfg.DatabasePath(),
		BusyTimeout:  a.cfg.SQLiteBusyTimeout(),
		MaxOpenConns: a.cfg.SQLiteMaxOpenConns,
		Clock:        a.clock,
	})
	if err != nil {
		if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			a.logger.Info("runtime startup cancelled", "reason", ctx.Err())
			return nil
		}
		return err
	}
	defer func() {
		if err := store.Close(); err != nil {
			a.logger.Warn("database close failed", "error", err)
		}
	}()

	objects, err := objectstore.New(a.cfg.ObjectStorePath())
	if err != nil {
		return err
	}
	if _, err := objects.CleanupTemps(a.clock.Now(), 24*time.Hour); err != nil {
		a.logger.Warn("temporary object cleanup failed", "error", err)
	}

	ids := id.NewGenerator()
	runtimeID, err := ids.RuntimeInstance()
	if err != nil {
		return fmt.Errorf("generate runtime instance id: %w", err)
	}

	processes := agentprocess.NewService(store, ids, a.clock)
	memory, err := mmu.New(store, objects, ids, a.clock, mmu.DefaultConfig())
	if err != nil {
		return fmt.Errorf("initialize cognitive MMU: %w", err)
	}
	agentRunner := agent.NewG4(a.logger, processes, objects, ids, runtimeID, memory)
	processSupervisor := supervisor.New(a.logger, processes, a.clock, runtimeID)

	// Recovery completes before the daemon accepts control requests.
	if err := processSupervisor.Recover(ctx); err != nil {
		if ctx.Err() != nil {
			a.logger.Info("runtime recovery cancelled", "reason", ctx.Err())
			return nil
		}
		return fmt.Errorf("recover durable processes: %w", err)
	}

	profiler, err := diag.StartPprof(a.cfg.PprofAddress, a.logger)
	if err != nil {
		return err
	}
	if profiler != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := profiler.Close(shutdownCtx); err != nil {
				a.logger.Warn("pprof shutdown failed", "error", err)
			}
		}()
	}

	listener, err := control.Listen(a.cfg.ControlAddress)
	if err != nil {
		return err
	}

	baseHandler := newControlHandler(store, processes, a.clock.Now)
	handler := newG2ControlHandler(baseHandler, agentRunner)
	server := control.NewServer(a.logger, handler, a.cfg.MaxFrameBytes, a.cfg.MaxControlConnections)

	runtimeCtx, cancelRuntime := context.WithCancel(ctx)
	defer cancelRuntime()

	serveErr := make(chan error, 1)
	supervisorErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(runtimeCtx, listener) }()
	go func() { supervisorErr <- processSupervisor.Run(runtimeCtx) }()

	a.logger.Info(
		"runtime started",
		"runtime_id", runtimeID,
		"data_dir", a.cfg.DataDir,
		"database", a.cfg.DatabasePath(),
		"control_address", a.cfg.ControlAddress,
	)

	select {
	case <-ctx.Done():
		cancelRuntime()
		if err := <-serveErr; err != nil {
			return fmt.Errorf("control server shutdown: %w", err)
		}
		if err := <-supervisorErr; err != nil {
			return fmt.Errorf("supervisor shutdown: %w", err)
		}
		if err := a.checkpointStore(store); err != nil {
			a.logger.Warn("WAL checkpoint during shutdown failed", "error", err)
		}
		a.logger.Info("runtime stopped", "reason", ctx.Err())
		return nil

	case err := <-serveErr:
		cancelRuntime()
		supervisorRunErr := <-supervisorErr
		if err != nil {
			return fmt.Errorf("control server stopped: %w", err)
		}
		if supervisorRunErr != nil {
			return fmt.Errorf("supervisor stopped while control server exited: %w", supervisorRunErr)
		}
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("control server stopped unexpectedly")

	case err := <-supervisorErr:
		cancelRuntime()
		serverRunErr := <-serveErr
		if err != nil {
			return fmt.Errorf("supervisor stopped: %w", err)
		}
		if serverRunErr != nil {
			return fmt.Errorf("control server stopped while supervisor exited: %w", serverRunErr)
		}
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("supervisor stopped unexpectedly")
	}
}

func (a *App) checkpointStore(store interface{ Checkpoint(context.Context) error }) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return store.Checkpoint(shutdownCtx)
}
