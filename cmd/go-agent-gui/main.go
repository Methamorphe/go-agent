package main

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/Methamorphe/go-agent/internal/config"
	"github.com/Methamorphe/go-agent/internal/control"
	"github.com/Methamorphe/go-agent/internal/gui"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/workspace"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	flags := flag.NewFlagSet("go-agent-gui", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	var dataDir string
	var controlAddress string
	var agentID string
	var mode string
	flags.StringVar(&dataDir, "data-dir", "", "runtime data directory")
	flags.StringVar(&controlAddress, "control-address", "", "unix socket or Windows named pipe")
	flags.StringVar(&agentID, "agent", "", "Agent Process to attach; latest root when omitted")
	flags.StringVar(&mode, "mode", "ACT", "ASK|PLAN|ACT|REVIEW|OBSERVE")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go-agent-gui [--agent <id>] [--mode ACT]")
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

	controlClient := control.NewClient(cfg.ControlAddress, cfg.MaxFrameBytes, id.NewGenerator())
	desktopService := gui.NewService(controlClient, gui.LaunchOptions{
		AgentID: id.AgentID(strings.TrimSpace(agentID)),
		Mode:    workspaceMode,
	})

	app := application.New(application.Options{
		Name:        "GO Agent",
		Description: "Durable agent workspace",
		Services: []application.Service{
			application.NewService(desktopService),
		},
		Assets: application.AssetOptions{
			Handler:        application.BundledAssetFileServer(assets),
			DisableLogging: true,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                          "workspace",
		Title:                         "GO Agent",
		Width:                         1440,
		Height:                        900,
		MinWidth:                      760,
		MinHeight:                     560,
		BackgroundColour:              application.NewRGB(246, 245, 242),
		DefaultContextMenuDisabled:    true,
		ZoomControlEnabled:            false,
	})

	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "desktop error:", err)
		return 1
	}
	return 0
}
