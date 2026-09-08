package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"github.com/Methamorphe/go-agent/internal/app"
	"github.com/Methamorphe/go-agent/internal/config"
	"github.com/Methamorphe/go-agent/internal/control"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/logging"
	"github.com/Methamorphe/go-agent/internal/tui"
	"github.com/Methamorphe/go-agent/internal/workspace"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	flags := flag.NewFlagSet("go-agent-tui", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	var dataDir string
	var controlAddress string
	var logLevel string
	var pprofAddress string
	var agentID string
	var mode string
	var theme string
	var uiConfigPath string
	var runtimeDaemon bool

	flags.StringVar(&dataDir, "data-dir", "", "runtime data directory")
	flags.StringVar(&controlAddress, "control-address", "", "unix socket or Windows named pipe")
	flags.StringVar(&logLevel, "log-level", "", "debug|info|warn|error")
	flags.StringVar(&pprofAddress, "pprof-address", "", "explicit loopback address for runtime profiling")
	flags.StringVar(&agentID, "agent", "", "Agent Process to attach; latest root when omitted")
	flags.StringVar(&mode, "mode", "ACT", "ASK|PLAN|ACT|REVIEW|OBSERVE")
	flags.StringVar(&theme, "theme", "", "dark|light; overrides the UI profile")
	flags.StringVar(&uiConfigPath, "ui-config", "", "JSON TUI customization profile")
	flags.BoolVar(&runtimeDaemon, "runtime-daemon", false, "internal: run the detached durable runtime")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go-agent-tui [--agent <id>] [--mode ACT] [--theme dark] [--ui-config profile.json]")
		return 2
	}

	visited := map[string]bool{}
	flags.Visit(func(value *flag.Flag) { visited[value.Name] = true })
	overrides := config.Overrides{}
	if visited["data-dir"] {
		overrides.DataDir = &dataDir
	}
	if visited["control-address"] {
		overrides.ControlAddress = &controlAddress
	}
	if visited["log-level"] {
		overrides.LogLevel = &logLevel
	}
	if visited["pprof-address"] {
		overrides.PprofAddress = &pprofAddress
	}

	cfg, err := config.Load("", overrides)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if runtimeDaemon {
		logger, err := logging.New(os.Stderr, cfg.LogLevel)
		if err != nil {
			fmt.Fprintln(os.Stderr, "logging configuration error:", err)
			return 2
		}
		if err := app.New(logger, cfg).Run(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "runtime error:", err)
			return 1
		}
		return 0
	}

	workspaceMode := workspace.Mode(strings.ToUpper(strings.TrimSpace(mode)))
	if !workspaceMode.Valid() {
		fmt.Fprintln(os.Stderr, "--mode must be ASK, PLAN, ACT, REVIEW, or OBSERVE")
		return 2
	}

	uiConfig := tui.DefaultConfig()
	uiConfig.AgentID = id.AgentID(strings.TrimSpace(agentID))
	uiConfig.Mode = workspaceMode
	customization, err := tui.LoadCustomization(uiConfigPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "UI configuration error:", err)
		return 2
	}
	uiConfig, err = tui.ApplyCustomization(uiConfig, customization)
	if err != nil {
		fmt.Fprintln(os.Stderr, "UI configuration error:", err)
		return 2
	}
	if visited["theme"] {
		switch strings.ToLower(strings.TrimSpace(theme)) {
		case "dark":
			uiConfig.Theme = tui.DarkTheme()
		case "light":
			uiConfig.Theme = tui.LightTheme()
		default:
			fmt.Fprintln(os.Stderr, "--theme must be dark or light")
			return 2
		}
	}

	if err := ensureRuntime(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "runtime startup error:", err)
		return 1
	}

	controlClient := control.NewClient(cfg.ControlAddress, cfg.MaxFrameBytes, id.NewGenerator())
	client := tui.NewControlClient(controlClient)
	model := tui.NewModel(ctx, client, uiConfig)
	program := tea.NewProgram(model, tea.WithContext(ctx), tea.WithFPS(uiConfig.FPS))
	if _, err := program.Run(); err != nil {
		if ctx.Err() != nil {
			return 0
		}
		fmt.Fprintln(os.Stderr, "tui error:", err)
		return 1
	}
	return 0
}
