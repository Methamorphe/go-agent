package world

import (
	"encoding/json"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
)

type EffectClass string

const (
	EffectPure          EffectClass = "Pure"
	EffectRead          EffectClass = "Read"
	EffectReversible    EffectClass = "Reversible"
	EffectCompensatable EffectClass = "Compensatable"
	EffectIrreversible  EffectClass = "Irreversible"
)

type Effect struct {
	Class      EffectClass `json:"class"`
	Idempotent bool        `json:"idempotent"`
	Retryable  bool        `json:"retryable"`
}

type Action struct {
	ID       id.ActionID   `json:"id"`
	AgentID  id.AgentID    `json:"agent_id"`
	Kind     string        `json:"kind"`
	Purpose  string        `json:"purpose"`
	Resource string        `json:"resource,omitempty"`
	Params   json.RawMessage `json:"params,omitempty"`
	Effect   Effect        `json:"effect"`
}

type ResultStatus string

const (
	ResultSucceeded ResultStatus = "succeeded"
	ResultFailed    ResultStatus = "failed"
	ResultDenied    ResultStatus = "denied"
)

type Result struct {
	Status   ResultStatus `json:"status"`
	Data     []byte       `json:"data,omitempty"`
	ExitCode *int         `json:"exit_code,omitempty"`
	Error    string       `json:"error,omitempty"`
}

type Profile struct {
	Name                     string `json:"name"`
	SupportsStreaming        bool   `json:"supports_streaming"`
	SupportsCancellation     bool   `json:"supports_cancellation"`
	SupportsProcessTreeKill  bool   `json:"supports_process_tree_kill"`
}

type Intent struct {
	Version            uint64   `json:"version"`
	Goal               string   `json:"goal"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	AllowedDomains     []string `json:"allowed_domains,omitempty"`
	ForbiddenDomains   []string `json:"forbidden_domains,omitempty"`
}

type Capability struct {
	Domain    string     `json:"domain"`
	Scope     string     `json:"scope"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type ActionProof struct {
	IntentVersion    uint64 `json:"intent_version"`
	Purpose          string `json:"purpose"`
	CapabilityDomain string `json:"capability_domain"`
	CapabilityScope  string `json:"capability_scope"`
}

type AuthorizationDecision struct {
	Allowed bool         `json:"allowed"`
	Code    string       `json:"code"`
	Proof   *ActionProof `json:"proof,omitempty"`
}

func CanonicalEffect(kind string) Effect {
	switch kind {
	case "fs.read_file", "fs.list_directory":
		return Effect{Class: EffectRead, Idempotent: true, Retryable: true}
	case "fs.write_file":
		return Effect{Class: EffectReversible, Idempotent: true, Retryable: true}
	case "process.exec":
		return Effect{Class: EffectCompensatable, Idempotent: false, Retryable: false}
	case "network.request":
		return Effect{Class: EffectIrreversible, Idempotent: false, Retryable: false}
	default:
		return Effect{Class: EffectPure, Idempotent: true, Retryable: true}
	}
}
