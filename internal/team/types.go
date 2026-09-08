package team

import (
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/orchestration"
)

type TeamState string

const (
	TeamProposed        TeamState = "PROPOSED"
	TeamAdmitted        TeamState = "ADMITTED"
	TeamActive          TeamState = "ACTIVE"
	TeamFormationFailed TeamState = "FORMATION_FAILED"
	TeamCompleted       TeamState = "COMPLETED"
	TeamEscalated       TeamState = "ESCALATED"
	TeamRejected        TeamState = "REJECTED"
)

type SpecialistProfile struct {
	Role                    string                         `json:"role"`
	TaskIntent              string                         `json:"task_intent"`
	Authority               []orchestration.CapabilityGrant `json:"authority"`
	Budget                  orchestration.Budget           `json:"budget"`
	Result                  orchestration.ResultContract   `json:"result"`
	ModelObjective          string                         `json:"model_objective,omitempty"`
	IndependentVerification bool                           `json:"independent_verification,omitempty"`
}

type Limits struct {
	MaxMembers     int           `json:"max_members"`
	MaxParallelism int           `json:"max_parallelism"`
	MaxRounds      int           `json:"max_rounds"`
	MaxTurns       int           `json:"max_turns"`
	Deadline       *time.Time    `json:"deadline,omitempty"`
}

type Proposal struct {
	ID          id.TeamID           `json:"id"`
	RootAgentID id.AgentID          `json:"root_agent_id"`
	LeadAgentID id.AgentID          `json:"lead_agent_id"`
	Objective   string              `json:"objective"`
	Specialists []SpecialistProfile `json:"specialists"`
	Limits      Limits              `json:"limits"`
	CreatedAt   time.Time           `json:"created_at"`
}

type AdmissionDecision struct {
	Admitted bool     `json:"admitted"`
	Reason   string   `json:"reason,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

type Member struct {
	Role    string     `json:"role"`
	AgentID id.AgentID `json:"agent_id"`
	SpawnID id.SpawnID `json:"spawn_id"`
}

type Team struct {
	Proposal  Proposal          `json:"proposal"`
	State     TeamState         `json:"state"`
	Admission AdmissionDecision `json:"admission"`
	Members   []Member          `json:"members,omitempty"`
	Failure   string            `json:"failure,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type NegotiationState string

const (
	NegotiationOpen      NegotiationState = "OPEN"
	NegotiationConverged NegotiationState = "CONVERGED"
	NegotiationEscalated NegotiationState = "ESCALATED"
	NegotiationExpired   NegotiationState = "EXPIRED"
)

type TurnKind string

const (
	TurnClaim          TurnKind = "claim"
	TurnChallenge      TurnKind = "challenge"
	TurnEvidence       TurnKind = "evidence"
	TurnCounterexample TurnKind = "counterexample"
	TurnRevision       TurnKind = "revision"
	TurnConcede        TurnKind = "concede"
	TurnEscalation     TurnKind = "escalation"
)

type Turn struct {
	Sequence     uint64     `json:"sequence"`
	Round        int        `json:"round"`
	ActorAgentID id.AgentID `json:"actor_agent_id"`
	Kind         TurnKind   `json:"kind"`
	Statement    string     `json:"statement,omitempty"`
	EvidenceRefs []string   `json:"evidence_refs,omitempty"`
	ReplyTo      uint64     `json:"reply_to,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Negotiation struct {
	ID           id.NegotiationID   `json:"id"`
	TeamID       id.TeamID          `json:"team_id"`
	Subject      string             `json:"subject"`
	Participants []id.AgentID       `json:"participants"`
	State        NegotiationState   `json:"state"`
	Round        int                `json:"round"`
	MaxRounds    int                `json:"max_rounds"`
	MaxTurns     int                `json:"max_turns"`
	Deadline     *time.Time         `json:"deadline,omitempty"`
	Version      uint64             `json:"version"`
	Resolution   string             `json:"resolution,omitempty"`
	EscalatedTo  id.AgentID         `json:"escalated_to,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
}
