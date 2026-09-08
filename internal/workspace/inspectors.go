package workspace

import (
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
)

type TransactionSummary struct {
	ID                 id.TransactionID `json:"id"`
	AgentID            id.AgentID       `json:"agent_id"`
	WorldID            id.WorldID       `json:"world_id"`
	State              string           `json:"state"`
	Version            uint64           `json:"version"`
	VerificationStatus string           `json:"verification_status,omitempty"`
	EffectCount        int              `json:"effect_count"`
	UncertainEffects   int              `json:"uncertain_effects"`
	ReconcileReason    string           `json:"reconcile_reason,omitempty"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

type ForkBranchSummary struct {
	ForkID           id.ForkID  `json:"fork_id"`
	AgentID          id.AgentID `json:"agent_id"`
	WorldID          id.WorldID `json:"world_id"`
	State            string     `json:"state"`
	SpentMoneyMicros int64      `json:"spent_money_micros"`
	SpentTokens      int64      `json:"spent_tokens"`
	Evaluation       string     `json:"evaluation,omitempty"`
}

type ForkSummary struct {
	GroupID         id.ForkGroupID      `json:"group_id"`
	SourceAgentID   id.AgentID          `json:"source_agent_id"`
	State           string              `json:"state"`
	WinnerForkID    id.ForkID           `json:"winner_fork_id,omitempty"`
	SelectionReason string              `json:"selection_reason,omitempty"`
	Branches        []ForkBranchSummary `json:"branches"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

type ContextFaultSummary struct {
	FaultID    id.ContextFaultID `json:"fault_id"`
	AgentID    id.AgentID        `json:"agent_id"`
	Kind       string            `json:"kind"`
	Reference  string            `json:"reference"`
	State      string            `json:"state"`
	Resolution string            `json:"resolution,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type TeamSummary struct {
	TeamID      id.TeamID  `json:"team_id"`
	RootAgentID id.AgentID `json:"root_agent_id"`
	LeadAgentID id.AgentID `json:"lead_agent_id"`
	State       string     `json:"state"`
	Objective   string     `json:"objective"`
	MemberCount int        `json:"member_count"`
	Round       int        `json:"round"`
	MaxRounds   int        `json:"max_rounds"`
	Escalated   bool       `json:"escalated"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type ImprovementSummary struct {
	ArtifactID string    `json:"artifact_id"`
	Kind       string    `json:"kind"`
	Version    uint64    `json:"version"`
	Status     string    `json:"status"`
	ScopeLevel string    `json:"scope_level"`
	ScopeKey   string    `json:"scope_key,omitempty"`
	Active     bool      `json:"active"`
	Killed     bool      `json:"killed"`
	CreatedAt  time.Time `json:"created_at"`
}

type Inspector struct {
	Transactions  []TransactionSummary  `json:"transactions"`
	Forks         []ForkSummary         `json:"forks"`
	ContextFaults []ContextFaultSummary `json:"context_faults"`
	Teams         []TeamSummary         `json:"teams"`
	Improvements  []ImprovementSummary  `json:"improvements"`
}
