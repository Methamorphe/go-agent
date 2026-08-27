package api

import (
	"github.com/Methamorphe/go-agent/internal/agent"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/live"
)

const (
	TypeAgentRun  = "agent.run"
	TypeAgentLive = "agent.live"
)

const (
	MessageAgentRun  = TypeAgentRun
	MessageAgentLive = TypeAgentLive
)

type AgentRunRequest struct {
	AgentID   id.AgentID `json:"agent_id"`
	Provider  string     `json:"provider"`
	Model     string     `json:"model"`
	BaseURL   string     `json:"base_url,omitempty"`
	APIKeyEnv string     `json:"api_key_env,omitempty"`
	Workspace string     `json:"workspace"`
	MaxSteps  int        `json:"max_steps,omitempty"`
}

type AgentRunResponse struct {
	Result agent.RunResult `json:"result"`
}

type AgentLiveRequest struct {
	AgentID id.AgentID `json:"agent_id"`
}

type AgentLiveResponse struct {
	Stream live.Snapshot `json:"stream"`
}
