package fork

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/scheduler"
	"github.com/Methamorphe/go-agent/internal/world"
)

var (
	ErrNotFound          = errors.New("cognitive fork object not found")
	ErrConflict          = errors.New("cognitive fork conflict")
	ErrInvalidState      = errors.New("invalid cognitive fork state")
	ErrBudgetExhausted   = errors.New("cognitive fork budget exhausted")
	ErrNoEligibleWinner  = errors.New("no eligible cognitive fork winner")
	ErrPromotionRequired = errors.New("cognitive fork promotion requires evaluated winner")
)

type Checkpoint struct {
	ID                 id.CheckpointID    `json:"id"`
	SourceAgentID      id.AgentID         `json:"source_agent_id"`
	RootAgentID        id.AgentID         `json:"root_agent_id"`
	ProcessVersion     uint64             `json:"process_version"`
	LedgerSequence     uint64             `json:"ledger_sequence"`
	LastEventID        id.EventID         `json:"last_event_id"`
	WorldRef           world.BranchRef    `json:"world_ref"`
	ContextFrontier    []id.ContextPageID `json:"context_frontier,omitempty"`
	ProcessStateJSON   json.RawMessage    `json:"process_state_json"`
	StateHash          string             `json:"state_hash"`
	CreatedAt          time.Time          `json:"created_at"`
	RetainUntil        *time.Time         `json:"retain_until,omitempty"`
}

type GroupState string

const (
	GroupOpen      GroupState = "OPEN"
	GroupEvaluated GroupState = "EVALUATED"
	GroupPromoting GroupState = "PROMOTING"
	GroupPromoted  GroupState = "PROMOTED"
	GroupDiscarded GroupState = "DISCARDED"
)

type Group struct {
	ID            id.ForkGroupID  `json:"id"`
	CheckpointID  id.CheckpointID `json:"checkpoint_id"`
	SourceAgentID id.AgentID      `json:"source_agent_id"`
	State         GroupState      `json:"state"`
	WinnerForkID  id.ForkID       `json:"winner_fork_id,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type BranchState string

const (
	BranchOpen      BranchState = "OPEN"
	BranchEvaluated BranchState = "EVALUATED"
	BranchSelected  BranchState = "SELECTED"
	BranchPromoted  BranchState = "PROMOTED"
	BranchDiscarded BranchState = "DISCARDED"
)

type Branch struct {
	ID               id.ForkID          `json:"id"`
	GroupID          id.ForkGroupID     `json:"group_id"`
	AgentID          id.AgentID         `json:"agent_id"`
	WorldID          id.WorldID         `json:"world_id"`
	WorldRef         world.BranchRef     `json:"world_ref"`
	BudgetLimit      scheduler.Resources `json:"budget_limit"`
	BudgetSpent      scheduler.Resources `json:"budget_spent"`
	State            BranchState         `json:"state"`
	Evaluation       *Evaluation         `json:"evaluation,omitempty"`
	CognitiveOverlay []OverlayEntry      `json:"cognitive_overlay,omitempty"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

type OverlayKind string

const (
	OverlayContext  OverlayKind = "context"
	OverlayMemory   OverlayKind = "memory"
	OverlayDecision OverlayKind = "decision"
)

type OverlayEntry struct {
	Kind       OverlayKind     `json:"kind"`
	Key        string          `json:"key"`
	Value      json.RawMessage `json:"value"`
	Promotable bool            `json:"promotable"`
	CreatedAt  time.Time       `json:"created_at"`
}

type Metrics struct {
	Correctness float64 `json:"correctness"`
	Quality     float64 `json:"quality"`
	LatencyMS   int64   `json:"latency_ms"`
	MoneyMicros int64   `json:"money_micros"`
	Tokens      int64   `json:"tokens"`
}

type Objective struct {
	CorrectnessWeight float64 `json:"correctness_weight"`
	QualityWeight     float64 `json:"quality_weight"`
	LatencyWeight     float64 `json:"latency_weight"`
	MoneyWeight       float64 `json:"money_weight"`
	TokenWeight       float64 `json:"token_weight"`
	MinCorrectness    float64 `json:"min_correctness"`
}

func (o Objective) normalized() Objective {
	if o == (Objective{}) { return Objective{CorrectnessWeight: 0.65, QualityWeight: 0.35, MinCorrectness: 1} }
	return o
}

func (o Objective) Validate() error {
	o = o.normalized()
	if o.MinCorrectness < 0 || o.MinCorrectness > 1 { return errors.New("min correctness must be within [0,1]") }
	for _, w := range []float64{o.CorrectnessWeight, o.QualityWeight, o.LatencyWeight, o.MoneyWeight, o.TokenWeight} {
		if w < 0 { return errors.New("objective weights must be non-negative") }
	}
	return nil
}

type Evaluation struct {
	Metrics     Metrics   `json:"metrics"`
	Score       float64   `json:"score"`
	Eligible    bool      `json:"eligible"`
	Reason      string    `json:"reason,omitempty"`
	EvidenceRef string    `json:"evidence_ref,omitempty"`
	EvaluatedAt time.Time `json:"evaluated_at"`
}

func Evaluate(metrics Metrics, objective Objective, at time.Time) Evaluation {
	o := objective.normalized()
	eligible := metrics.Correctness >= o.MinCorrectness
	score := metrics.Correctness*o.CorrectnessWeight + metrics.Quality*o.QualityWeight -
		(float64(metrics.LatencyMS)/1000.0)*o.LatencyWeight -
		(float64(metrics.MoneyMicros)/1_000_000.0)*o.MoneyWeight -
		(float64(metrics.Tokens)/1000.0)*o.TokenWeight
	reason := ""
	if !eligible { reason = fmt.Sprintf("correctness %.4f below floor %.4f", metrics.Correctness, o.MinCorrectness) }
	return Evaluation{Metrics: metrics, Score: score, Eligible: eligible, Reason: reason, EvaluatedAt: at.UTC()}
}

func SelectWinner(branches []Branch) (id.ForkID, error) {
	eligible := make([]Branch, 0, len(branches))
	for _, b := range branches { if b.Evaluation != nil && b.Evaluation.Eligible { eligible = append(eligible, b) } }
	if len(eligible) == 0 { return "", ErrNoEligibleWinner }
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].Evaluation.Score == eligible[j].Evaluation.Score { return strings.Compare(eligible[i].ID.String(), eligible[j].ID.String()) < 0 }
		return eligible[i].Evaluation.Score > eligible[j].Evaluation.Score
	})
	return eligible[0].ID, nil
}

func NamespaceAction(forkID id.ForkID, actionID id.ActionID) id.ActionID {
	if forkID == "" || actionID == "" { return actionID }
	return id.ActionID("fork:" + forkID.String() + ":" + actionID.String())
}
