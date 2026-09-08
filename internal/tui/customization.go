package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Customization is presentation-only configuration. It cannot add control
// protocol methods, bypass authority, or mutate durable Agent state by itself.
type Customization struct {
	Theme             string            `json:"theme,omitempty"`
	RefreshIntervalMS int               `json:"refresh_interval_ms,omitempty"`
	InspectorEvery    int               `json:"inspector_every,omitempty"`
	HistoryPageSize   int               `json:"history_page_size,omitempty"`
	TreeLimit         int               `json:"tree_limit,omitempty"`
	MaxCachedBlocks   int               `json:"max_cached_blocks,omitempty"`
	FPS               int               `json:"fps,omitempty"`
	Keymap            map[string]string `json:"keymap,omitempty"`
	Commands          map[string]string `json:"commands,omitempty"`
}

func LoadCustomization(path string) (Customization, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Customization{}, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return Customization{}, fmt.Errorf("read TUI customization: %w", err)
	}
	var value Customization
	decoderErr := json.Unmarshal(body, &value)
	if decoderErr != nil {
		return Customization{}, fmt.Errorf("decode TUI customization: %w", decoderErr)
	}
	return value, nil
}

func ApplyCustomization(cfg Config, value Customization) (Config, error) {
	switch strings.ToLower(strings.TrimSpace(value.Theme)) {
	case "":
	case "dark":
		cfg.Theme = DarkTheme()
	case "light":
		cfg.Theme = LightTheme()
	default:
		return Config{}, fmt.Errorf("unsupported TUI theme %q", value.Theme)
	}
	if value.RefreshIntervalMS > 0 {
		cfg.RefreshInterval = time.Duration(value.RefreshIntervalMS) * time.Millisecond
	}
	if value.InspectorEvery > 0 {
		cfg.InspectorEvery = value.InspectorEvery
	}
	if value.HistoryPageSize > 0 {
		cfg.HistoryPageSize = value.HistoryPageSize
	}
	if value.TreeLimit > 0 {
		cfg.TreeLimit = value.TreeLimit
	}
	if value.MaxCachedBlocks > 0 {
		cfg.MaxCachedBlocks = value.MaxCachedBlocks
	}
	if value.FPS > 0 {
		cfg.FPS = value.FPS
	}
	if value.Keymap != nil {
		cfg.Keymap = copyMappings(value.Keymap)
	}
	if value.Commands != nil {
		cfg.CommandAliases = copyMappings(value.Commands)
	}
	return cfg.normalized(), nil
}

func copyMappings(source map[string]string) map[string]string {
	out := make(map[string]string, len(source))
	for key, value := range source {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			out[key] = value
		}
	}
	return out
}

func (c Config) normalizeKey(key string) string {
	if mapped := strings.TrimSpace(c.Keymap[key]); mapped != "" {
		return mapped
	}
	return key
}

func (c Config) expandCommand(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return command
	}
	alias := strings.TrimSpace(c.CommandAliases[strings.ToLower(fields[0])])
	if alias == "" {
		return command
	}
	remainder := strings.TrimSpace(strings.TrimPrefix(command, fields[0]))
	if remainder == "" {
		return alias
	}
	return alias + " " + remainder
}
