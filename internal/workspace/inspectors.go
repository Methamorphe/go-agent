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

// ContextRuntimeSummary is a bounded projection of the Cognitive MMU. It does
// not copy page bodies into the workspace; object/page references remain the
// canonical evidence boundary.
type ContextRuntimeSummary struct {
	AgentID            id.AgentID `json:"agent_id,omitempty"`
	PageCount          int        `json:"page_count"`
	EstimatedTokens    int64      `json:"estimated_tokens"`
	ActiveLeaseCount   int        `json:"active_lease_count"`
	LatestManifestRef  string     `json:"latest_manifest_ref,omitempty"`
	LatestManifestAt   time.Time  `json:"latest_manifest_at,omitempty"`
	UnresolvedFaults   int        `json:"unresolved_faults"`
}

// SchedulerSummary exposes the current root budget and the latest durable G6
// routing decision. A zero-value routing section means no scheduled invocation
// has yet been recorded for the focused workspace.
type SchedulerSummary struct {
	RootAgentID          id.AgentID `json:"root_agent_id,omitempty"`
	LimitMoneyMicros     int64      `json:"limit_money_micros"`
	LimitTokens          int64      `json:"limit_tokens"`
	SpentMoneyMicros     int64      `json:"spent_money_micros"`
	SpentTokens          int64      `json:"spent_tokens"`
	ReservedMoneyMicros  int64      `json:"reserved_money_micros"`
	ReservedTokens       int64      `json:"reserved_tokens"`
	LastDecisionID       string     `json:"last_decision_id,omitempty"`
	LastProvider         string     `json:"last_provider,omitempty"`
	LastModel            string     `json:"last_model,omitempty"`
	LastProfileVersion   uint32     `json:"last_profile_version,omitempty"`
	LastEstimatedMoney   int64      `json:"last_estimated_money_micros"`
	LastEstimatedTokens  int64      `json:"last_estimated_tokens"`
	LastDecisionAt       time.Time  `json:"last_decision_at,omitempty"`
}

// AuthoritySummary is deliberately explicit about what is and is not
// projected. G13 never invents a capability grant from UI state. The root task
// intent is durable and visible; live capability details may remain below the
// projection boundary unless the runtime has durably emitted them.
type AuthoritySummary struct {
	AgentID              id.AgentID `json:"agent_id,omitempty"`
	IntentID             id.IntentID `json:"intent_id,omitempty"`
	IntentVersion        uint32      `json:"intent_version,omitempty"`
	Goal                 string     `json:"goal,omitempty"`
	CapabilityProjection string     `json:"capability_projection"`
	LastSecurityEvent    string     `json:"last_security_event,omitempty"`
	LastSecurityAt       time.Time  `json:"last_security_at,omitempty"`
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
	Context       ContextRuntimeSummary `json:"context"`
	ContextFaults []ContextFaultSummary `json:"context_faults"`
	Scheduler     SchedulerSummary      `json:"scheduler"`
	Authority     AuthoritySummary      `json:"authority"`
	Teams         []TeamSummary         `json:"teams"`
	Improvements  []ImprovementSummary  `json:"improvements"`
}
