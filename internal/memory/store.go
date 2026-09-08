package memory

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/objectstore"
)

var (
	ErrNotFound       = errors.New("epistemic memory object not found")
	ErrConflict       = errors.New("epistemic memory conflict")
	ErrPolicyDenied   = errors.New("epistemic memory write denied by policy")
	ErrInvalidState   = errors.New("invalid epistemic memory state")
	ErrPropagationCap = errors.New("truth-maintenance propagation budget exhausted")
)

type Repository interface {
	PutEvidence(context.Context, Evidence) error
	Evidence(context.Context, id.EvidenceID) (Evidence, error)
	EvidenceBySource(context.Context, string) ([]Evidence, error)
	InvalidateEvidence(context.Context, EvidenceInvalidation) error
	EvidenceInvalidation(context.Context, id.EvidenceID) (*EvidenceInvalidation, error)
	PutSourceVersion(context.Context, SourceVersion) error
	SourceVersions(context.Context, string, int) ([]SourceVersion, error)

	CreateBelief(context.Context, Belief, []EvidenceLink, []BeliefEdge) error
	Belief(context.Context, id.BeliefID) (Belief, error)
	BeliefEvidence(context.Context, id.BeliefID) ([]EvidenceLink, error)
	EvidenceDependents(context.Context, id.EvidenceID) ([]EvidenceLink, error)
	BeliefIncoming(context.Context, id.BeliefID) ([]BeliefEdge, error)
	BeliefOutgoing(context.Context, id.BeliefID) ([]BeliefEdge, error)
	PutBeliefEdge(context.Context, BeliefEdge) error
	TransitionBelief(context.Context, id.BeliefID, BeliefStatus, BeliefStatus, ConfidenceProfile, *time.Time, string, id.PropagationID, time.Time) (Belief, error)
	SearchBeliefs(context.Context, SearchQuery) ([]SearchCandidate, error)
	StatusHistory(context.Context, id.BeliefID) ([]StatusTransition, error)
	RecordBeliefUse(context.Context, UsageRecord) error
	BeliefUses(context.Context, id.BeliefID, int) ([]UsageRecord, error)

	CreatePropagationJob(context.Context, PropagationJob, []PropagationItem) error
	PropagationJob(context.Context, id.PropagationID) (PropagationJob, error)
	PendingPropagationItems(context.Context, id.PropagationID, int) ([]PropagationItem, error)
	EnqueuePropagationItems(context.Context, []PropagationItem) error
	MarkPropagationItemProcessed(context.Context, id.PropagationID, PropagationNodeKind, string, time.Time) error
	UpdatePropagationJob(context.Context, PropagationJob) error
}

type IDGenerator interface {
	Evidence() (id.EvidenceID, error)
	Belief() (id.BeliefID, error)
	Propagation() (id.PropagationID, error)
}

type ObjectStore interface {
	Put(context.Context, io.Reader) (objectstore.Meta, error)
}

type WriteAction string

const (
	WritePropose    WriteAction = "memory.propose"
	WritePublish    WriteAction = "memory.publish"
	WriteInvalidate WriteAction = "memory.invalidate"
	WritePin        WriteAction = "memory.pin"
)

type WritePolicy interface {
	AuthorizeMemoryWrite(context.Context, id.AgentID, WriteAction, Scope) bool
}

type WritePolicyFunc func(context.Context, id.AgentID, WriteAction, Scope) bool

func (f WritePolicyFunc) AuthorizeMemoryWrite(ctx context.Context, actor id.AgentID, action WriteAction, scope Scope) bool {
	return f(ctx, actor, action, scope)
}

type LocalOnlyPolicy struct{}

func (LocalOnlyPolicy) AuthorizeMemoryWrite(_ context.Context, actor id.AgentID, action WriteAction, scope Scope) bool {
	if actor == "" || !scope.Valid() {
		return false
	}
	if scope.Kind == ScopeProject || scope.Kind == ScopeUser {
		return false
	}
	switch action {
	case WritePropose, WritePublish, WriteInvalidate, WritePin:
		return true
	default:
		return false
	}
}

type AllowAllPolicy struct{}

func (AllowAllPolicy) AuthorizeMemoryWrite(_ context.Context, actor id.AgentID, action WriteAction, scope Scope) bool {
	return actor != "" && scope.Valid() && action != ""
}
