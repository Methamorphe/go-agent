package transaction

import (
	"encoding/json"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/world"
)

type State string

const (
	StateCreating            State = "CREATING"
	StateOpen                State = "OPEN"
	StateVerifying           State = "VERIFYING"
	StateReadyToCommit       State = "READY_TO_COMMIT"
	StateCommitting          State = "COMMITTING"
	StateRollingBack         State = "ROLLING_BACK"
	StateNeedsReconciliation State = "NEEDS_RECONCILIATION"
	StateCommitted           State = "COMMITTED"
	StateRolledBack          State = "ROLLED_BACK"
	StateFailed              State = "FAILED"
)

func (s State) Terminal() bool {
	return s == StateCommitted || s == StateRolledBack || s == StateFailed
}

type CommitPolicy string

const (
	CommitRequireVerification CommitPolicy = "require_verification"
)

type Transaction struct {
	ID               id.TransactionID     `json:"id"`
	AgentID          id.AgentID           `json:"agent_id"`
	BaseCheckpoint   id.CheckpointID      `json:"base_checkpoint"`
	WorldID          id.WorldID           `json:"world_id"`
	IsolatedWorldRef world.BranchRef       `json:"isolated_world_ref"`
	State            State                 `json:"state"`
	Version          uint64                `json:"version"`
	CommitPolicy     CommitPolicy          `json:"commit_policy"`
	PreparedPlan     *world.PromotionPlan  `json:"prepared_plan,omitempty"`
	ReconcileReason  string                `json:"reconcile_reason,omitempty"`
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
}

type EffectState string

const (
	EffectPrepared          EffectState = "PREPARED"
	EffectDeferred          EffectState = "DEFERRED"
	EffectDispatched        EffectState = "DISPATCHED"
	EffectCompleted         EffectState = "COMPLETED"
	EffectFailedBeforeEffect EffectState = "FAILED_BEFORE_EFFECT"
	EffectOutcomeUnknown    EffectState = "OUTCOME_UNKNOWN"
)

type OutcomeCertainty string

const (
	OutcomeNotApplicable OutcomeCertainty = "not_applicable"
	OutcomeKnownApplied  OutcomeCertainty = "known_applied"
	OutcomeKnownAbsent   OutcomeCertainty = "known_absent"
	OutcomeUnknown       OutcomeCertainty = "unknown"
)

type EffectRecord struct {
	ID               id.EffectRecordID `json:"id"`
	TransactionID    id.TransactionID  `json:"transaction_id"`
	ActionID         id.ActionID       `json:"action_id"`
	Kind             string            `json:"kind"`
	Effect           world.Effect      `json:"effect"`
	State            EffectState       `json:"state"`
	OutcomeCertainty OutcomeCertainty  `json:"outcome_certainty"`
	Error            string            `json:"error,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

type VerificationStatus string

const (
	VerificationRunning VerificationStatus = "RUNNING"
	VerificationPassed  VerificationStatus = "PASSED"
	VerificationFailed  VerificationStatus = "FAILED"
)

type CheckSpec struct {
	Name   string       `json:"name"`
	Action world.Action `json:"action"`
}

type CheckResult struct {
	Name       string             `json:"name"`
	Status     world.ResultStatus `json:"status"`
	ExitCode   *int               `json:"exit_code,omitempty"`
	DataSHA256 string             `json:"data_sha256,omitempty"`
	Error      string             `json:"error,omitempty"`
}

type Verification struct {
	ID            id.VerificationID `json:"id"`
	TransactionID id.TransactionID  `json:"transaction_id"`
	Checks        []CheckSpec        `json:"checks"`
	Results       []CheckResult      `json:"results"`
	Status        VerificationStatus `json:"status"`
	StartedAt     time.Time          `json:"started_at"`
	CompletedAt   *time.Time         `json:"completed_at,omitempty"`
}

type Event struct {
	Sequence           uint64          `json:"sequence"`
	TransactionID      id.TransactionID `json:"transaction_id"`
	TransactionVersion uint64          `json:"transaction_version"`
	Type               string          `json:"type"`
	Payload            json.RawMessage `json:"payload,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
}
