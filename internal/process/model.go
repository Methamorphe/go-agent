package process

import (
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
)

const StateSchemaVersion uint32 = 1
const SnapshotSchemaVersion uint32 = 1
const IntentSchemaVersion uint32 = 1

type Status string

const (
	StatusNew       Status = "NEW"
	StatusReady     Status = "READY"
	StatusRunning   Status = "RUNNING"
	StatusWaiting   Status = "WAITING"
	StatusSleeping  Status = "SLEEPING"
	StatusSuspended Status = "SUSPENDED"
	StatusCompleted Status = "COMPLETED"
	StatusFailed    Status = "FAILED"
	StatusCancelled Status = "CANCELLED"
)

func (s Status) Terminal() bool {
	return s == StatusCompleted ||
		s == StatusFailed ||
		s == StatusCancelled
}

type WaitingReason string

const (
	WaitingModel          WaitingReason = "WAITING_MODEL"
	WaitingTool           WaitingReason = "WAITING_TOOL"
	WaitingChild          WaitingReason = "WAITING_CHILD"
	WaitingApproval       WaitingReason = "WAITING_APPROVAL"
	WaitingResource       WaitingReason = "WAITING_RESOURCE"
	WaitingReconciliation WaitingReason = "WAITING_RECONCILIATION"
)

type Intent struct {
	ID            id.IntentID `json:"id"`
	SchemaVersion uint32      `json:"schema_version"`
	Goal          string      `json:"goal"`
	CreatedAt     time.Time   `json:"created_at"`
}

type WaitState struct {
	Reason    WaitingReason `json:"reason"`
	Reference string        `json:"reference,omitempty"`
	Since     time.Time     `json:"since"`
}

type SleepState struct {
	ID         id.SleepID `json:"id"`
	WakeAt     time.Time  `json:"wake_at"`
	Generation uint64     `json:"generation"`
	CreatedAt  time.Time  `json:"created_at"`
}

type ExecutionLease struct {
	ID                id.LeaseID           `json:"id"`
	RuntimeInstanceID id.RuntimeInstanceID `json:"runtime_instance_id"`
	AcquiredAt        time.Time            `json:"acquired_at"`
	ProcessVersion    uint64               `json:"process_version"`
}

type CancelState struct {
	Requested bool       `json:"requested"`
	Scope     string     `json:"scope,omitempty"`
	EventID   id.EventID `json:"event_id,omitempty"`
}

type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type State struct {
	AgentID          id.AgentID      `json:"agent_id"`
	RootAgentID      id.AgentID      `json:"root_agent_id"`
	ParentAgentID    *id.AgentID     `json:"parent_agent_id,omitempty"`
	CreationEventID  id.EventID      `json:"creation_event_id"`
	CreatedAt        time.Time       `json:"created_at"`
	LineageDepth     uint32          `json:"lineage_depth"`
	Version          uint64          `json:"version"`
	Status           Status          `json:"status"`
	RootIntent       *Intent         `json:"root_intent,omitempty"`
	Wait             *WaitState      `json:"wait,omitempty"`
	Sleep            *SleepState     `json:"sleep,omitempty"`
	Lease            *ExecutionLease `json:"lease,omitempty"`
	Cancel           CancelState     `json:"cancel"`
	Failure          *Failure        `json:"failure,omitempty"`
	CompletionRef    string          `json:"completion_ref,omitempty"`
	LastCheckpointID id.CheckpointID `json:"last_checkpoint_id,omitempty"`
	LastEventID      id.EventID      `json:"last_event_id"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type Snapshot struct {
	ID                    id.SnapshotID `json:"id"`
	AgentID               id.AgentID    `json:"agent_id"`
	ThroughProcessVersion uint64        `json:"through_process_version"`
	ThroughLedgerSequence uint64        `json:"through_ledger_sequence"`
	SchemaVersion         uint32        `json:"schema_version"`
	CreatedAt             time.Time     `json:"created_at"`
	StateJSON             []byte        `json:"state_json"`
	SHA256                string        `json:"sha256"`
}

type Receipt struct {
	RequestID     id.RequestID `json:"request_id"`
	CommandType   string       `json:"command_type"`
	AgentID       id.AgentID   `json:"agent_id"`
	ResultVersion uint64       `json:"result_version"`
	ResultJSON    []byte       `json:"result_json"`
	LastEventID   id.EventID   `json:"last_event_id"`
	CreatedAt     time.Time    `json:"created_at"`
}

type SleepDue struct {
	AgentID    id.AgentID `json:"agent_id"`
	SleepID    id.SleepID `json:"sleep_id"`
	Generation uint64     `json:"generation"`
	WakeAt     time.Time  `json:"wake_at"`
	Version    uint64     `json:"version"`
}
