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

	"github.com/Methamorphe/go-agent/internal/config"
	"github.com/Methamorphe/go-agent/internal/control"
	"github.com/Methamorphe/go-agent/internal/id"
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
	var agentID string
	var mode string
	var theme string
	var uiConfigPath string
	flags.StringVar(&dataDir, "data-dir", "", "runtime data directory")
	flags.StringVar(&controlAddress, "control-address", "", "unix socket or Windows named pipe")
	flags.StringVar(&agentID, "agent", "", "Agent Process to attach; latest root when omitted")
	flags.StringVar(&mode, "mode", "ACT", "ASK|PLAN|ACT|REVIEW|OBSERVE")
	flags.StringVar(&theme, "theme", "", "dark|light; overrides the UI profile")
	flags.StringVar(&uiConfigPath, "ui-config", "", "JSON TUI customization profile")
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
	cfg, err := config.Load("", overrides)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		return 2
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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
