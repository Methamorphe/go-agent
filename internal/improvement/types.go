package improvement

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type ArtifactID string
type EvaluationID string

type ArtifactKind string

const (
	KindPrompt        ArtifactKind = "prompt"
	KindSkill         ArtifactKind = "skill"
	KindAgentProfile  ArtifactKind = "agent_profile"
	KindRoutingPolicy ArtifactKind = "routing_policy"
	KindContextPolicy ArtifactKind = "context_policy"
	KindMemoryPolicy  ArtifactKind = "memory_policy"
	KindEvaluator     ArtifactKind = "evaluator"
)

func (k ArtifactKind) Valid() bool {
	switch k {
	case KindPrompt, KindSkill, KindAgentProfile, KindRoutingPolicy, KindContextPolicy, KindMemoryPolicy, KindEvaluator:
		return true
	default:
		return false
	}
}

type ArtifactStatus string

const (
	StatusDraft      ArtifactStatus = "DRAFT"
	StatusCandidate  ArtifactStatus = "CANDIDATE"
	StatusEvaluating ArtifactStatus = "EVALUATING"
	StatusShadow     ArtifactStatus = "SHADOW"
	StatusCanary     ArtifactStatus = "CANARY"
	StatusPromoted   ArtifactStatus = "PROMOTED"
	StatusDeprecated ArtifactStatus = "DEPRECATED"
	StatusRolledBack ArtifactStatus = "ROLLED_BACK"
	StatusRejected   ArtifactStatus = "REJECTED"
)

type ScopeLevel string

const (
	ScopeGlobal      ScopeLevel = "global"
	ScopeUser        ScopeLevel = "user"
	ScopeProject     ScopeLevel = "project"
	ScopeTaskClass   ScopeLevel = "task_class"
	ScopeModelFamily ScopeLevel = "model_family"
	ScopeWorldType   ScopeLevel = "world_type"
)

type Scope struct {
	Level ScopeLevel `json:"level"`
	Key   string     `json:"key,omitempty"`
}

func (s Scope) Valid() bool {
	switch s.Level {
	case ScopeGlobal:
		return s.Key == ""
	case ScopeUser, ScopeProject, ScopeTaskClass, ScopeModelFamily, ScopeWorldType:
		return strings.TrimSpace(s.Key) != ""
	default:
		return false
	}
}
func (s Scope) Equal(other Scope) bool { return s.Level == other.Level && s.Key == other.Key }

type Hypothesis struct {
	Observation     string   `json:"observation"`
	ProposedChange  string   `json:"proposed_change"`
	ExpectedBenefit string   `json:"expected_benefit"`
	ExpectedRisks   []string `json:"expected_risks,omitempty"`
	Scope           string   `json:"scope"`
}

func (h Hypothesis) Valid() bool {
	return strings.TrimSpace(h.Observation) != "" && strings.TrimSpace(h.ProposedChange) != "" && strings.TrimSpace(h.ExpectedBenefit) != "" && strings.TrimSpace(h.Scope) != ""
}

type ArtifactVersion struct {
	ArtifactID           ArtifactID     `json:"artifact_id"`
	Version              uint32         `json:"version"`
	Kind                 ArtifactKind   `json:"kind"`
	ContentRef           string         `json:"content_ref"`
	ParentVersion        uint32         `json:"parent_version,omitempty"`
	OriginRef            string         `json:"origin_ref"`
	Hypothesis           *Hypothesis    `json:"hypothesis,omitempty"`
	Status               ArtifactStatus `json:"status"`
	Scope                Scope          `json:"scope"`
	RequiredCapabilities []string       `json:"required_capabilities,omitempty"`
	CanarySampleCap      uint64         `json:"canary_sample_cap,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
}

func (v ArtifactVersion) Key() string                 { return VersionKey(v.ArtifactID, v.Version) }
func VersionKey(id ArtifactID, version uint32) string { return fmt.Sprintf("%s@v%d", id, version) }

type Outcome struct {
	TaskRef                       string  `json:"task_ref"`
	BaselineSuccess               bool    `json:"baseline_success"`
	CandidateSuccess              bool    `json:"candidate_success"`
	QualityDelta                  float64 `json:"quality_delta,omitempty"`
	CostDelta                     float64 `json:"cost_delta,omitempty"`
	LatencyDelta                  float64 `json:"latency_delta,omitempty"`
	SecurityViolation             bool    `json:"security_violation,omitempty"`
	CriticalRegression            bool    `json:"critical_regression,omitempty"`
	DeterministicVerificationLost bool    `json:"deterministic_verification_lost,omitempty"`
	ReplayIncompatible            bool    `json:"replay_incompatible,omitempty"`
	IrreversibleEffectRegression  bool    `json:"irreversible_effect_regression,omitempty"`
}

type Evaluation struct {
	ID               EvaluationID `json:"id"`
	ArtifactID       ArtifactID   `json:"artifact_id"`
	BaselineVersion  uint32       `json:"baseline_version"`
	CandidateVersion uint32       `json:"candidate_version"`
	CorpusRef        string       `json:"corpus_ref"`
	EvaluatorRef     string       `json:"evaluator_ref"`
	Independent      bool         `json:"independent"`
	KnownBias        []string     `json:"known_bias,omitempty"`
	Outcomes         []Outcome    `json:"outcomes"`
	EvidenceRefs     []string     `json:"evidence_refs,omitempty"`
	CreatedAt        time.Time    `json:"created_at"`
}

func (e Evaluation) UniqueTasks() int {
	m := map[string]struct{}{}
	for _, o := range e.Outcomes {
		if o.TaskRef != "" {
			m[o.TaskRef] = struct{}{}
		}
	}
	return len(m)
}
func (e Evaluation) WinLossTie() (wins, losses, ties int) {
	for _, o := range e.Outcomes {
		switch {
		case o.CandidateSuccess && !o.BaselineSuccess:
			wins++
		case !o.CandidateSuccess && o.BaselineSuccess:
			losses++
		default:
			ties++
		}
	}
	return
}
func (e Evaluation) HardRegression() bool {
	for _, o := range e.Outcomes {
		if o.SecurityViolation || o.CriticalRegression || o.DeterministicVerificationLost || o.ReplayIncompatible || o.IrreversibleEffectRegression {
			return true
		}
	}
	return false
}

type PromotionRecord struct {
	ArtifactID       ArtifactID   `json:"artifact_id"`
	BaselineVersion  uint32       `json:"baseline_version"`
	CandidateVersion uint32       `json:"candidate_version"`
	EvaluationID     EvaluationID `json:"evaluation_id"`
	EvaluationRefs   []string     `json:"evaluation_refs,omitempty"`
	PromotedAt       time.Time    `json:"promoted_at"`
}

type InvocationManifest struct {
	InvocationID string                `json:"invocation_id"`
	Artifacts    map[ArtifactID]uint32 `json:"artifacts"`
	ManifestHash string                `json:"manifest_hash"`
	CapturedAt   time.Time             `json:"captured_at"`
}

type KillSwitch struct {
	All      bool                  `json:"all"`
	Families map[ArtifactKind]bool `json:"families,omitempty"`
	Versions map[string]bool       `json:"versions,omitempty"`
}

func (k KillSwitch) Blocks(v ArtifactVersion) bool {
	return k.All || k.Families[v.Kind] || k.Versions[v.Key()]
}

type EffectClass string

const (
	EffectReadOnly EffectClass = "read_only"
	EffectMutating EffectClass = "mutating"
)

func normalizeCapabilities(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
func hasRequired(required, granted []string) bool {
	set := map[string]struct{}{}
	for _, v := range granted {
		set[v] = struct{}{}
	}
	for _, v := range required {
		if _, ok := set[v]; !ok {
			return false
		}
	}
	return true
}
