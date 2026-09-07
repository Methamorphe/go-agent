package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/scheduler"
)

type SchedulerBackendConfig struct {
	ProviderID string `json:"provider_id"`
	Type       string `json:"type"`
	BaseURL    string `json:"base_url,omitempty"`
	APIKeyEnv  string `json:"api_key_env,omitempty"`
}

type SchedulerProfileConfig struct {
	ModelID                    string                         `json:"model_id"`
	ProviderID                 string                         `json:"provider_id"`
	ContextWindow              int                            `json:"context_window"`
	MaxOutputTokens            int                            `json:"max_output_tokens"`
	Capabilities               []scheduler.Capability         `json:"capabilities,omitempty"`
	Locality                   scheduler.Locality             `json:"locality"`
	AllowsSensitive            bool                           `json:"allows_sensitive"`
	TrainingOptOut             bool                           `json:"training_opt_out"`
	InputCostMicrosPerMillion  int64                          `json:"input_cost_micros_per_million"`
	OutputCostMicrosPerMillion int64                          `json:"output_cost_micros_per_million"`
	LatencyP95MS               int                            `json:"latency_p95_ms"`
	Reliability                float64                        `json:"reliability"`
	Quality                    map[scheduler.TaskKind]float64 `json:"quality,omitempty"`
	MaxConcurrency             int                            `json:"max_concurrency"`
	Experimental               bool                           `json:"experimental,omitempty"`
	ProfileVersion             uint32                         `json:"profile_version"`
}

type SchedulerConfig struct {
	Enabled            bool                      `json:"enabled"`
	MaxProfiles        int                       `json:"max_profiles"`
	GlobalSlots        int                       `json:"global_slots"`
	PerRootSlots       int                       `json:"per_root_slots"`
	PerProviderSlots   int                       `json:"per_provider_slots"`
	DefaultRootBudget  scheduler.Resources       `json:"default_root_budget"`
	Backends           []SchedulerBackendConfig  `json:"backends,omitempty"`
	Profiles           []SchedulerProfileConfig  `json:"profiles,omitempty"`
}

func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		Enabled:          false,
		MaxProfiles:      scheduler.DefaultMaxProfiles,
		GlobalSlots:      8,
		PerRootSlots:     4,
		PerProviderSlots: 8,
	}
}

func (c SchedulerConfig) Validate() error {
	if c.MaxProfiles <= 0 || c.MaxProfiles > 4096 {
		return fmt.Errorf("scheduler.max_profiles must be between 1 and 4096")
	}
	if c.GlobalSlots <= 0 || c.GlobalSlots > 4096 {
		return fmt.Errorf("scheduler.global_slots must be between 1 and 4096")
	}
	if c.PerRootSlots <= 0 || c.PerRootSlots > c.GlobalSlots {
		return fmt.Errorf("scheduler.per_root_slots must be between 1 and global_slots")
	}
	if c.PerProviderSlots <= 0 || c.PerProviderSlots > c.GlobalSlots {
		return fmt.Errorf("scheduler.per_provider_slots must be between 1 and global_slots")
	}
	if !c.DefaultRootBudget.Valid() {
		return fmt.Errorf("scheduler.default_root_budget contains negative values")
	}
	if !c.Enabled {
		return nil
	}
	if c.DefaultRootBudget.Tokens <= 0 {
		return fmt.Errorf("scheduler.default_root_budget.tokens must be positive when enabled")
	}
	if len(c.Backends) == 0 || len(c.Profiles) == 0 {
		return fmt.Errorf("scheduler requires at least one backend and one profile when enabled")
	}
	if len(c.Profiles) > c.MaxProfiles {
		return fmt.Errorf("scheduler profile count exceeds max_profiles")
	}

	backends := make(map[string]struct{}, len(c.Backends))
	for i, backend := range c.Backends {
		id := strings.TrimSpace(backend.ProviderID)
		if id == "" {
			return fmt.Errorf("scheduler.backends[%d].provider_id is required", i)
		}
		if _, exists := backends[id]; exists {
			return fmt.Errorf("scheduler backend %q is duplicated", id)
		}
		switch backend.Type {
		case "openai", "openai-compatible":
		default:
			return fmt.Errorf("scheduler backend %q has unsupported type %q", id, backend.Type)
		}
		backends[id] = struct{}{}
	}

	profiles := make(map[string]struct{}, len(c.Profiles))
	for i, raw := range c.Profiles {
		if _, ok := backends[strings.TrimSpace(raw.ProviderID)]; !ok {
			return fmt.Errorf("scheduler.profiles[%d] references unknown provider %q", i, raw.ProviderID)
		}
		profile := raw.ModelProfile()
		if err := profile.Validate(); err != nil {
			return fmt.Errorf("scheduler.profiles[%d]: %w", i, err)
		}
		key := profile.Key()
		if _, exists := profiles[key]; exists {
			return fmt.Errorf("scheduler profile %q is duplicated", key)
		}
		profiles[key] = struct{}{}
	}
	return nil
}

func (c SchedulerProfileConfig) ModelProfile() scheduler.ModelProfile {
	return scheduler.ModelProfile{
		ModelID:                    scheduler.ModelID(strings.TrimSpace(c.ModelID)),
		ProviderID:                 scheduler.ProviderID(strings.TrimSpace(c.ProviderID)),
		ContextWindow:              c.ContextWindow,
		MaxOutputTokens:            c.MaxOutputTokens,
		Capabilities:               append([]scheduler.Capability(nil), c.Capabilities...),
		Locality:                   c.Locality,
		DataPolicy:                 scheduler.DataPolicy{AllowsSensitive: c.AllowsSensitive, TrainingOptOut: c.TrainingOptOut},
		InputCostMicrosPerMillion:  c.InputCostMicrosPerMillion,
		OutputCostMicrosPerMillion: c.OutputCostMicrosPerMillion,
		LatencyP95:                 time.Duration(c.LatencyP95MS) * time.Millisecond,
		Reliability:                c.Reliability,
		Quality:                    cloneQuality(c.Quality),
		MaxConcurrency:             c.MaxConcurrency,
		Experimental:               c.Experimental,
		ProfileVersion:             c.ProfileVersion,
	}
}

func cloneQuality(in map[scheduler.TaskKind]float64) map[scheduler.TaskKind]float64 {
	if in == nil {
		return nil
	}
	out := make(map[scheduler.TaskKind]float64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (c SchedulerConfig) Backend(providerID scheduler.ProviderID) (SchedulerBackendConfig, error) {
	for _, backend := range c.Backends {
		if strings.TrimSpace(backend.ProviderID) == string(providerID) {
			return backend, nil
		}
	}
	return SchedulerBackendConfig{}, errors.New("scheduler backend not found")
}
