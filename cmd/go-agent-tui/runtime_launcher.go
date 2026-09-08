package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Methamorphe/go-agent/internal/config"
	"github.com/Methamorphe/go-agent/internal/control"
)

const (
	runtimeProbeTimeout   = 200 * time.Millisecond
	runtimeStartupTimeout = 10 * time.Second
	runtimeProbeInterval  = 50 * time.Millisecond
)

func ensureRuntime(ctx context.Context, cfg config.Config) error {
	if runtimeReachable(ctx, cfg.ControlAddress) {
		return nil
	}

	if err := startDetachedRuntime(cfg); err != nil {
		return err
	}

	startupCtx, cancel := context.WithTimeout(ctx, runtimeStartupTimeout)
	defer cancel()

	ticker := time.NewTicker(runtimeProbeInterval)
	defer ticker.Stop()

	for {
		if runtimeReachable(startupCtx, cfg.ControlAddress) {
			return nil
		}

		select {
		case <-startupCtx.Done():
			return fmt.Errorf(
				"runtime did not become ready within %s; inspect %s: %w",
				runtimeStartupTimeout,
				filepath.Join(cfg.DataDir, "runtime.log"),
				startupCtx.Err(),
			)
		case <-ticker.C:
		}
	}
}

func runtimeReachable(parent context.Context, address string) bool {
	ctx, cancel := context.WithTimeout(parent, runtimeProbeTimeout)
	defer cancel()
	return control.Probe(ctx, address) == nil
}

func startDetachedRuntime(cfg config.Config) error {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("create runtime data directory: %w", err)
	}

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve TUI executable: %w", err)
	}

	logPath := filepath.Join(cfg.DataDir, "runtime.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open runtime log: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(executable, runtimeDaemonArgs(cfg)...)
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = os.Environ()
	prepareDetachedProcess(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start detached runtime: %w", err)
	}
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("release detached runtime process: %w", err)
	}
	return nil
}

func runtimeDaemonArgs(cfg config.Config) []string {
	args := []string{
		"--data-dir", cfg.DataDir,
		"--control-address", cfg.ControlAddress,
		"--log-level", cfg.LogLevel,
	}
	if cfg.PprofAddress != "" {
		args = append(args, "--pprof-address", cfg.PprofAddress)
	}
	return append(args, "--runtime-daemon")
}
