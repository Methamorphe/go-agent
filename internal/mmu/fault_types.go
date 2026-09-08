package mmu

import (
	"context"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

type CognitiveRef string

const (
	RefContext    = "ctx"
	RefBelief     = "belief"
	RefEvidence   = "evidence"
	RefObject     = "object"
	RefEvent      = "event"
	RefCheckpoint = "checkpoint"
	RefAgent      = "agent"
)

func ParseCognitiveRef(value string) (CognitiveRef, error) {
	value = strings.TrimSpace(value)
	parts := strings.SplitN(value, "://", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "", errs.New(errs.CodeInvalidArgument, "mmu.ref.parse", "invalid cognitive reference")
	}
	switch parts[0] {
	case RefContext, RefBelief, RefEvidence, RefObject, RefEvent, RefCheckpoint, RefAgent:
	default:
		return "", errs.New(errs.CodeInvalidArgument, "mmu.ref.parse", "unsupported cognitive reference scheme")
	}
	if strings.ContainsAny(parts[1], " \t\r\n") {
		return "", errs.New(errs.CodeInvalidArgument, "mmu.ref.parse", "cognitive reference contains whitespace")
	}
	return CognitiveRef(parts[0] + "://" + parts[1]), nil
}

func (r CognitiveRef) String() string { return string(r) }
func (r CognitiveRef) Scheme() string {
	parts := strings.SplitN(string(r), "://", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[0]
}
func (r CognitiveRef) Key() string {
	parts := strings.SplitN(string(r), "://", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

type FaultKind string

const (
	FaultReference      FaultKind = "ReferenceFault"
	FaultRecall         FaultKind = "RecallFault"
	FaultEvidence       FaultKind = "EvidenceFault"
	FaultFreshness      FaultKind = "FreshnessFault"
	FaultDependency     FaultKind = "DependencyFault"
	FaultRepresentation FaultKind = "RepresentationFault"
)

type FaultPurpose string

const (
	PurposeContinueReasoning    FaultPurpose = "continue_reasoning"
	PurposeVerifyClaim          FaultPurpose = "verify_claim"
	PurposePrepareAction        FaultPurpose = "prepare_action"
	PurposeResolveContradiction FaultPurpose = "resolve_contradiction"
	PurposeInspectHistory       FaultPurpose = "inspect_history"
	PurposeSatisfyDependency    FaultPurpose = "satisfy_dependency"
)

type FaultUrgency string

const (
	UrgencyNormal   FaultUrgency = "normal"
	UrgencyHigh     FaultUrgency = "high"
	UrgencyCritical FaultUrgency = "critical"
)

type FaultState string

const (
	FaultDetected        FaultState = "DETECTED"
	FaultResolved        FaultState = "RESOLVED"
	FaultDenied          FaultState = "DENIED"
	FaultUnresolved      FaultState = "UNRESOLVED"
	FaultNeedsProjection FaultState = "NEEDS_COMPACTION_OR_PROJECTION"
	FaultBudgetExceeded  FaultState = "BUDGET_EXCEEDED"
)

type FaultBudget struct {
	MaxFaultsPerTurn       int           `json:"max_faults_per_turn"`
	MaxFaultsPerTaskWindow int           `json:"max_faults_per_task_window"`
	MaxMaterializedTokens  int           `json:"max_materialized_tokens"`
	MaxResolutionTime      time.Duration `json:"max_resolution_time"`
	MaxRepeatedSetHits     int           `json:"max_repeated_set_hits"`
}

func DefaultFaultBudget() FaultBudget {
	return FaultBudget{MaxFaultsPerTurn: 8, MaxFaultsPerTaskWindow: 32, MaxMaterializedTokens: 12_000, MaxResolutionTime: 10 * time.Second, MaxRepeatedSetHits: 3}
}

type ContextFaultRequest struct {
	ID            id.ContextFaultID `json:"id,omitempty"`
	AgentID       id.AgentID        `json:"agent_id"`
	InvocationID  id.InvocationID   `json:"invocation_id"`
	TaskWindowID  string            `json:"task_window_id,omitempty"`
	ProgressEpoch uint64            `json:"progress_epoch,omitempty"`
	Kind          FaultKind         `json:"kind"`
	Reference     *CognitiveRef     `json:"reference,omitempty"`
	Query         *RecallQuery      `json:"query,omitempty"`
	Purpose       FaultPurpose      `json:"purpose"`
	RequiredBy    []CognitiveRef    `json:"required_by,omitempty"`
	MaxTokens     int               `json:"max_tokens,omitempty"`
	Urgency       FaultUrgency      `json:"urgency,omitempty"`
	AllowedScopes []Scope           `json:"allowed_scopes"`
}

type FaultResolutionPlan struct {
	PageIDs      []id.ContextPageID `json:"page_ids,omitempty"`
	ResolvedRefs []CognitiveRef     `json:"resolved_refs,omitempty"`
	Dependencies []CognitiveRef     `json:"dependencies,omitempty"`
}

type CognitiveResolver interface {
	ResolveCognitiveRef(context.Context, ContextFaultRequest, CognitiveRef) (FaultResolutionPlan, error)
}

type FaultResolution struct {
	FaultID      id.ContextFaultID  `json:"fault_id"`
	State        FaultState          `json:"state"`
	PageIDs      []id.ContextPageID `json:"page_ids,omitempty"`
	ResolvedRefs []CognitiveRef     `json:"resolved_refs,omitempty"`
	LeaseIDs     []id.LeaseID       `json:"lease_ids,omitempty"`
	Tokens       int                `json:"tokens"`
	Reason       string             `json:"reason,omitempty"`
}

type FaultRecord struct {
	Request      ContextFaultRequest `json:"request"`
	State        FaultState          `json:"state"`
	PageIDs      []id.ContextPageID  `json:"page_ids,omitempty"`
	ResolvedRefs []CognitiveRef      `json:"resolved_refs,omitempty"`
	LeaseIDs     []id.LeaseID        `json:"lease_ids,omitempty"`
	Tokens       int                 `json:"tokens"`
	Reason       string              `json:"reason,omitempty"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

type FaultJournal interface {
	PutContextFault(context.Context, FaultRecord) error
	PendingResolvedContextFaults(context.Context, id.AgentID) ([]FaultRecord, error)
	MarkContextFaultsManifested(context.Context, []id.ContextFaultID, id.InvocationID) error
}

type ManifestFault struct {
	ID            id.ContextFaultID  `json:"id"`
	Kind          FaultKind          `json:"kind"`
	Requested     string             `json:"requested,omitempty"`
	Purpose       FaultPurpose       `json:"purpose"`
	ResolvedPages []id.ContextPageID `json:"resolved_pages,omitempty"`
	ResolvedRefs  []CognitiveRef     `json:"resolved_refs,omitempty"`
	Tokens        int                `json:"tokens"`
	Lease         string             `json:"lease"`
}
