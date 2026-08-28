package orchestration

import (
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/objectstore"
)

type CapabilityGrant struct {
	Domain string `json:"domain"`
	Scope  string `json:"scope"`
}

type Budget struct {
	Tokens      int64 `json:"tokens"`
	ToolCalls   int64 `json:"tool_calls"`
	StorageBytes int64 `json:"storage_bytes"`
	WorldMillis int64 `json:"world_millis"`
	MoneyMicros int64 `json:"money_micros"`
}

func (b Budget) Valid() bool {
	return b.Tokens >= 0 && b.ToolCalls >= 0 && b.StorageBytes >= 0 && b.WorldMillis >= 0 && b.MoneyMicros >= 0
}

func (b Budget) IsZero() bool {
	return b == (Budget{})
}

func (b Budget) Fits(available Budget) bool {
	return b.Tokens <= available.Tokens && b.ToolCalls <= available.ToolCalls && b.StorageBytes <= available.StorageBytes && b.WorldMillis <= available.WorldMillis && b.MoneyMicros <= available.MoneyMicros
}

func (b Budget) Add(other Budget) Budget {
	return Budget{Tokens: b.Tokens + other.Tokens, ToolCalls: b.ToolCalls + other.ToolCalls, StorageBytes: b.StorageBytes + other.StorageBytes, WorldMillis: b.WorldMillis + other.WorldMillis, MoneyMicros: b.MoneyMicros + other.MoneyMicros}
}

func (b Budget) Sub(other Budget) Budget {
	return Budget{Tokens: b.Tokens - other.Tokens, ToolCalls: b.ToolCalls - other.ToolCalls, StorageBytes: b.StorageBytes - other.StorageBytes, WorldMillis: b.WorldMillis - other.WorldMillis, MoneyMicros: b.MoneyMicros - other.MoneyMicros}
}

type RootPolicy struct {
	Authority          []CapabilityGrant `json:"authority"`
	Budget             Budget            `json:"budget"`
	MaxTotalDescendants int               `json:"max_total_descendants"`
	MaxChildrenPerNode int                `json:"max_children_per_node"`
	MaxDepth           uint32             `json:"max_depth"`
	MaxParallelism     int                `json:"max_parallelism"`
	MailboxMessages    int                `json:"mailbox_messages"`
	MailboxBytes       int64              `json:"mailbox_bytes"`
}

type ResultContract struct {
	EvidenceRequired bool `json:"evidence_required"`
	MaxInlineBytes   int  `json:"max_inline_bytes"`
}

type SpawnSpec struct {
	ParentAgentID id.AgentID       `json:"parent_agent_id"`
	TaskIntent    string           `json:"task_intent"`
	Authority     []CapabilityGrant `json:"authority"`
	Budget        Budget            `json:"budget"`
	Result        ResultContract    `json:"result"`
	Deadline      *time.Time        `json:"deadline,omitempty"`
	ReuseCompleted bool             `json:"reuse_completed"`
	IndependentVerification bool    `json:"independent_verification"`
}

type SpawnStatus string

const (
	SpawnReserved SpawnStatus = "RESERVED"
	SpawnCreated  SpawnStatus = "CREATED"
	SpawnRejected SpawnStatus = "REJECTED"
	SpawnSettled  SpawnStatus = "SETTLED"
)

type SpawnRecord struct {
	ID            id.SpawnID             `json:"id"`
	ParentAgentID id.AgentID             `json:"parent_agent_id"`
	ChildAgentID  id.AgentID             `json:"child_agent_id,omitempty"`
	RootAgentID   id.AgentID             `json:"root_agent_id"`
	Depth         uint32                 `json:"depth"`
	TaskIntent    string                 `json:"task_intent"`
	TaskKey       string                 `json:"task_key"`
	Authority     []CapabilityGrant      `json:"authority"`
	ReservationID id.BudgetReservationID `json:"reservation_id"`
	Reserved      Budget                 `json:"reserved"`
	Status        SpawnStatus            `json:"status"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

type Account struct {
	AgentID   id.AgentID        `json:"agent_id"`
	RootID    id.AgentID        `json:"root_id"`
	Authority []CapabilityGrant `json:"authority"`
	Available Budget            `json:"available"`
	Reserved  Budget            `json:"reserved"`
	Policy    RootPolicy        `json:"policy"`
}

type MessageType string

const (
	MessageRequest MessageType = "request"
	MessageResponse MessageType = "response"
	MessageEvidence MessageType = "evidence"
	MessageStatus MessageType = "status"
	MessageResult MessageType = "result"
	MessageCancelRequest MessageType = "cancel_request"
)

type AgentMessage struct {
	ID            id.MessageID     `json:"id"`
	From          id.AgentID       `json:"from"`
	To            id.AgentID       `json:"to"`
	Type          MessageType      `json:"type"`
	PayloadRef    objectstore.Ref  `json:"payload_ref,omitempty"`
	Inline        string           `json:"inline,omitempty"`
	CorrelationID id.CorrelationID `json:"correlation_id,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
}

type ChildResult struct {
	ChildAgentID id.AgentID      `json:"child_agent_id"`
	Summary      string          `json:"summary"`
	ArtifactRef  objectstore.Ref `json:"artifact_ref,omitempty"`
	EvidenceRefs []string        `json:"evidence_refs,omitempty"`
	CompletedAt  time.Time       `json:"completed_at"`
}

type WaitMode string

const (
	WaitOne WaitMode = "one"
	WaitAll WaitMode = "all"
	WaitFirstSuccess WaitMode = "first_success"
	WaitQuorum WaitMode = "quorum"
)

type FailurePolicy string

const (
	FailureFailFast FailurePolicy = "fail_fast"
	FailureCollectAll FailurePolicy = "collect_all"
	FailureBestEffort FailurePolicy = "best_effort"
)

type WaitSpec struct {
	ID         id.WaitID      `json:"id"`
	ParentID   id.AgentID     `json:"parent_id"`
	Children   []id.AgentID   `json:"children"`
	Mode       WaitMode       `json:"mode"`
	Quorum     int            `json:"quorum,omitempty"`
	Failure    FailurePolicy  `json:"failure"`
	CreatedAt  time.Time      `json:"created_at"`
}

type Admission struct {
	AgentID id.AgentID `json:"agent_id"`
	RootID  id.AgentID `json:"root_id"`
	Priority int        `json:"priority"`
	EnqueuedAt time.Time `json:"enqueued_at"`
}
