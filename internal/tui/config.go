package tui

import (
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/workspace"
)

const (
	DefaultRefreshInterval = 250 * time.Millisecond
	DefaultInspectorEvery  = 4
	DefaultCacheBlocks     = 512
	MaximumCacheBlocks     = 2048
	DefaultFPS             = 30
	MinimumFPS             = 10
	MaximumFPS             = 60
)

type ThemeName string

const (
	ThemeDark  ThemeName = "dark"
	ThemeLight ThemeName = "light"
)

type Theme struct {
	Name       ThemeName `json:"name"`
	Background string    `json:"background"`
	Surface    string    `json:"surface"`
	SurfaceAlt string    `json:"surface_alt"`
	Text       string    `json:"text"`
	Muted      string    `json:"muted"`
	Accent     string    `json:"accent"`
	Success    string    `json:"success"`
	Warning    string    `json:"warning"`
	Danger     string    `json:"danger"`
	Border     string    `json:"border"`
	Focus      string    `json:"focus"`
}

func DarkTheme() Theme {
	return Theme{
		Name: ThemeDark,
		Background: "#0B0F14", Surface: "#121821", SurfaceAlt: "#18212C",
		Text: "#E6EDF3", Muted: "#8B98A5", Accent: "#7AA2F7", Success: "#9ECE6A",
		Warning: "#E0AF68", Danger: "#F7768E", Border: "#263241", Focus: "#BB9AF7",
	}
}

func LightTheme() Theme {
	return Theme{
		Name: ThemeLight,
		Background: "#F7F8FA", Surface: "#FFFFFF", SurfaceAlt: "#EEF1F5",
		Text: "#1F2328", Muted: "#66707A", Accent: "#2F6FEB", Success: "#1A7F37",
		Warning: "#9A6700", Danger: "#CF222E", Border: "#D0D7DE", Focus: "#8250DF",
	}
}

type Config struct {
	AgentID         id.AgentID
	Mode            workspace.Mode
	Theme           Theme
	RefreshInterval time.Duration
	InspectorEvery  int
	HistoryPageSize int
	TreeLimit       int
	MaxCachedBlocks int
	FPS             int
	Keymap          map[string]string
	CommandAliases  map[string]string
}

func DefaultConfig() Config {
	return Config{
		Mode:            workspace.ModeAct,
		Theme:           DarkTheme(),
		RefreshInterval: DefaultRefreshInterval,
		InspectorEvery:  DefaultInspectorEvery,
		HistoryPageSize: workspace.DefaultPageSize,
		TreeLimit:       workspace.DefaultTreeLimit,
		MaxCachedBlocks: DefaultCacheBlocks,
		FPS:             DefaultFPS,
		Keymap:          map[string]string{},
		CommandAliases:  map[string]string{},
	}
}

func (c Config) normalized() Config {
	if !c.Mode.Valid() {
		c.Mode = workspace.ModeAct
	}
	if c.Theme.Name != ThemeDark && c.Theme.Name != ThemeLight {
		c.Theme = DarkTheme()
	}
	if c.RefreshInterval <= 0 {
		c.RefreshInterval = DefaultRefreshInterval
	}
	if c.InspectorEvery <= 0 {
		c.InspectorEvery = DefaultInspectorEvery
	}
	if c.HistoryPageSize <= 0 || c.HistoryPageSize > workspace.MaxPageSize {
		c.HistoryPageSize = workspace.DefaultPageSize
	}
	if c.TreeLimit <= 0 || c.TreeLimit > workspace.MaxTreeLimit {
		c.TreeLimit = workspace.DefaultTreeLimit
	}
	if c.MaxCachedBlocks <= 0 {
		c.MaxCachedBlocks = DefaultCacheBlocks
	}
	if c.MaxCachedBlocks > MaximumCacheBlocks {
		c.MaxCachedBlocks = MaximumCacheBlocks
	}
	if c.FPS < MinimumFPS || c.FPS > MaximumFPS {
		c.FPS = DefaultFPS
	}
	if c.Keymap == nil {
		c.Keymap = map[string]string{}
	}
	if c.CommandAliases == nil {
		c.CommandAliases = map[string]string{}
	}
	return c
}
