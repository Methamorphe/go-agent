package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/config"
)

func TestRunStopsWhenContextIsCancelled(t *testing.T) {
	logger := slog.New(
		slog.NewTextHandler(
			io.Discard,
			nil,
		),
	)

	cfg := config.Defaults()
	cfg.DataDir = t.TempDir()

	if err := cfg.Normalize(); err != nil {
		t.Fatalf(
			"Normalize() returned an error: %v",
			err,
		)
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf(
			"Validate() returned an error: %v",
			err,
		)
	}

	application := New(
		logger,
		cfg,
	)

	ctx, cancel :=
		context.WithCancel(
			context.Background(),
		)

	done := make(
		chan error,
		1,
	)

	go func() {
		done <- application.Run(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf(
				"Run() returned an error: %v",
				err,
			)
		}

	case <-time.After(3 * time.Second):
		t.Fatal(
			"Run() did not stop after context cancellation",
		)
	}
}
