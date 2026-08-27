package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Methamorphe/go-agent/internal/app"
	"github.com/Methamorphe/go-agent/internal/config"
	"github.com/Methamorphe/go-agent/internal/logging"
)

func main() {
	os.Exit(
		run(os.Args[1:]),
	)
}

func run(args []string) int {
	flags := flag.NewFlagSet(
		"go-agent",
		flag.ContinueOnError,
	)

	flags.SetOutput(os.Stderr)

	var configPath string
	var dataDir string
	var logLevel string
	var controlAddress string
	var pprofAddress string

	flags.StringVar(
		&configPath,
		"config",
		"",
		"path to JSON config file",
	)

	flags.StringVar(
		&dataDir,
		"data-dir",
		"",
		"runtime data directory",
	)

	flags.StringVar(
		&logLevel,
		"log-level",
		"",
		"debug|info|warn|error",
	)

	flags.StringVar(
		&controlAddress,
		"control-address",
		"",
		"unix socket or Windows named pipe",
	)

	flags.StringVar(
		&pprofAddress,
		"pprof-address",
		"",
		"explicit loopback address, e.g. 127.0.0.1:6060",
	)

	if err := flags.Parse(args); err != nil {
		return 2
	}

	if flags.NArg() != 0 {
		fmt.Fprintln(
			os.Stderr,
			"unexpected positional arguments",
		)

		return 2
	}

	visited := map[string]bool{}

	flags.Visit(
		func(flag *flag.Flag) {
			visited[flag.Name] = true
		},
	)

	overrides := config.Overrides{}

	if visited["data-dir"] {
		overrides.DataDir = &dataDir
	}

	if visited["log-level"] {
		overrides.LogLevel = &logLevel
	}

	if visited["control-address"] {
		overrides.ControlAddress =
			&controlAddress
	}

	if visited["pprof-address"] {
		overrides.PprofAddress =
			&pprofAddress
	}

	cfg, err := config.Load(
		configPath,
		overrides,
	)

	if err != nil {
		fallback := slog.New(
			slog.NewTextHandler(
				os.Stderr,
				nil,
			),
		)

		fallback.Error(
			"configuration error",
			"error",
			err,
		)

		return 2
	}

	logger, err := logging.New(
		os.Stderr,
		cfg.LogLevel,
	)

	if err != nil {
		fmt.Fprintln(
			os.Stderr,
			err,
		)

		return 2
	}

	ctx, stop :=
		signal.NotifyContext(
			context.Background(),
			os.Interrupt,
			syscall.SIGTERM,
		)

	defer stop()

	application := app.New(
		logger,
		cfg,
	)

	if err := application.Run(ctx); err != nil {
		logger.Error(
			"application stopped with an error",
			"error",
			err,
		)

		return 1
	}

	return 0
}
