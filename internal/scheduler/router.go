package scheduler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"
)

type Weights struct{ Quality, Cost, Latency, FailureRisk, Load float64 }

func weightsFor(objective Objective) Weights {
	switch objective {
	case ObjectiveQualityFirst:
		return Weights{.65, .08, .08, .16, .03}
	case ObjectiveCostFirst:
		return Weights{.25, .5, .08, .12, .05}
	case ObjectiveLatencyFirst:
		return Weights{.25, .08, .5, .12, .05}
	default:
		return Weights{.42, .22, .18, .14, .04}
	}
}

type Router struct {
	registry      *Registry
	telemetry     *Telemetry
	clock         Clock
	policyVersion uint32
}

func NewRouter(registry *Registry, telemetry *Telemetry, clock Clock) *Router {
	if registry == nil {
		registry = NewRegistry(DefaultMaxProfiles)
	}
	if clock == nil {
		clock = systemClock{}
	}
	if telemetry == nil {
		telemetry = NewTelemetry(DefaultMaxProfiles, clock)
	}
	return &Router{registry: registry, telemetry: telemetry, clock: clock, policyVersion: DefaultPolicyVersion}
}
func (r *Router) Registry() *Registry   { return r.registry }
func (r *Router) Telemetry() *Telemetry { return r.telemetry }
func (r *Router) Route(task CognitiveTask, excluded map[string]struct{}) (RoutingDecision, error) {
	if err := task.Validate(); err != nil {
		return RoutingDecision{}, err
	}
	if excluded == nil {
		excluded = map[string]struct{}{}
	}
	profiles := r.registry.Snapshot()
	candidates := make([]Candidate, 0, len(profiles))
	rejected := make([]CandidateRejection, 0)
	for _, p := range profiles {
		if rej := r.eligible(task, p, excluded); rej != nil {
			rejected = append(rejected, *rej)
			continue
		}
		tel := r.telemetry.Snapshot(p.Ref())
		cost := estimateCost(task, p)
		latency := estimateLatency(p, tel)
		score := scoreCandidate(task, p, tel, cost, latency)
		candidates = append(candidates, Candidate{Model: p.Ref(), EstimatedCost: cost, EstimatedLatency: latency, Score: score})
	}
	if len(candidates) == 0 {
		sortRejections(rejected)
		return RoutingDecision{}, &NoEligibleError{Rejected: rejected}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if math.Abs(candidates[i].Score.Total-candidates[j].Score.Total) > 1e-12 {
			return candidates[i].Score.Total > candidates[j].Score.Total
		}
		return candidates[i].Model.Key() < candidates[j].Model.Key()
	})
	sortRejections(rejected)
	selected := candidates[0]
	created := r.clock.Now().UTC()
	did := decisionID(task, candidates, rejected, r.policyVersion)
	return RoutingDecision{ID: did, TaskID: task.ID, Candidates: candidates, Rejected: rejected, Selected: selected.Model, EstimatedCost: selected.EstimatedCost, EstimatedLatency: selected.EstimatedLatency, Score: selected.Score, PolicyVersion: r.policyVersion, CreatedAt: created}, nil
}
func (r *Router) eligible(task CognitiveTask, p ModelProfile, excluded map[string]struct{}) *CandidateRejection {
	reject := func(code RejectionCode, detail string) *CandidateRejection {
		return &CandidateRejection{Model: p.Ref(), Code: code, Detail: detail}
	}
	if _, ok := excluded[p.Key()]; ok {
		return reject(RejectExcluded, "candidate excluded by bounded fallback history")
	}
	if p.Experimental && !task.AllowExperimental {
		return reject(RejectExperimental, "experimental profile not allowed")
	}
	requiredContext := task.EstimatedInputTokens + task.ReservedOutputTokens
	if task.Requirements.MinContext > requiredContext {
		requiredContext = task.Requirements.MinContext
	}
	if p.ContextWindow < requiredContext {
		return reject(RejectContext, fmt.Sprintf("needs %d tokens, profile has %d", requiredContext, p.ContextWindow))
	}
	if p.MaxOutputTokens < task.ReservedOutputTokens {
		return reject(RejectContext, "max output below reservation")
	}
	for _, cap := range task.Requirements.RequiredCapabilities {
		if !p.Has(cap) {
			return reject(RejectCapability, "missing capability "+string(cap))
		}
	}
	if task.Objective == ObjectiveLocalOnly || task.Privacy == PrivacyLocalOnly {
		if p.Locality != LocalityLocal {
			return reject(RejectPrivacy, "local-only task")
		}
	}
	if task.Privacy == PrivacySensitive && !p.DataPolicy.AllowsSensitive {
		return reject(RejectPrivacy, "sensitive data not allowed")
	}
	if len(task.Requirements.AllowedProviders) > 0 && !containsProvider(task.Requirements.AllowedProviders, p.ProviderID) {
		return reject(RejectPolicy, "provider not allow-listed")
	}
	if containsProvider(task.Requirements.DeniedProviders, p.ProviderID) {
		return reject(RejectPolicy, "provider denied")
	}
	if len(task.Requirements.AllowedModels) > 0 && !containsModel(task.Requirements.AllowedModels, p.ModelID) {
		return reject(RejectPolicy, "model not allow-listed")
	}
	if containsModel(task.Requirements.DeniedModels, p.ModelID) {
		return reject(RejectPolicy, "model denied")
	}
	tel := r.telemetry.Snapshot(p.Ref())
	if tel.State == HealthUnavailable || tel.State == HealthRateLimited {
		return reject(RejectHealth, "provider circuit is "+string(tel.State))
	}
	quality := p.QualityFor(task.Kind)
	minQuality := task.Requirements.MinQuality
	if task.Risk == RiskHigh && minQuality < 0.7 {
		minQuality = 0.7
	}
	if quality < minQuality {
		return reject(RejectQuality, fmt.Sprintf("quality %.3f below floor %.3f", quality, minQuality))
	}
	minReliability := task.Requirements.MinReliability
	if task.Risk == RiskHigh && minReliability < 0.9 {
		minReliability = 0.9
	}
	if p.Reliability < minReliability {
		return reject(RejectReliability, fmt.Sprintf("reliability %.3f below floor %.3f", p.Reliability, minReliability))
	}
	cost := estimateCost(task, p)
	if (p.InputCostMicrosPerMillion < 0 || p.OutputCostMicrosPerMillion < 0) && task.Budget.MoneyMicros >= 0 {
		return reject(RejectBudget, "unknown price cannot satisfy finite budget")
	}
	if !cost.Fits(task.Budget) {
		return reject(RejectBudget, "estimated request exceeds task budget")
	}
	if task.Deadline != nil && r.clock.Now().Add(estimateLatency(p, tel)).After(task.Deadline.UTC()) {
		return reject(RejectDeadline, "estimated completion exceeds deadline")
	}
	return nil
}
func estimateCost(task CognitiveTask, p ModelProfile) Resources {
	money := int64(0)
	if p.InputCostMicrosPerMillion >= 0 && p.OutputCostMicrosPerMillion >= 0 {
		money = (int64(task.EstimatedInputTokens)*p.InputCostMicrosPerMillion + int64(task.ReservedOutputTokens)*p.OutputCostMicrosPerMillion + 999999) / 1000000
	}
	return Resources{MoneyMicros: money, Tokens: int64(task.EstimatedInputTokens + task.ReservedOutputTokens)}
}
func estimateLatency(p ModelProfile, t TelemetrySnapshot) time.Duration {
	base := p.LatencyP95
	if t.LatencyEWMA > 0 {
		base = t.LatencyEWMA
	}
	if base <= 0 {
		base = time.Second
	}
	capacity := p.MaxConcurrency
	if capacity <= 0 {
		capacity = 1
	}
	mult := 1 + float64(t.QueueDepth)/float64(capacity)
	if t.State == HealthDegraded {
		mult *= 1.5
	}
	if t.State == HealthRecovering {
		mult *= 1.25
	}
	return time.Duration(float64(base) * mult)
}
func scoreCandidate(task CognitiveTask, p ModelProfile, t TelemetrySnapshot, cost Resources, latency time.Duration) ScoreBreakdown {
	w := weightsFor(task.Objective)
	q := clamp01(p.QualityFor(task.Kind))
	costRatio := 0.0
	if task.Budget.MoneyMicros > 0 {
		costRatio = clamp01(float64(cost.MoneyMicros) / float64(task.Budget.MoneyMicros))
	}
	latencyNorm := clamp01(float64(latency) / float64(30*time.Second))
	risk := clamp01(1 - p.Reliability + t.ErrorEWMA*0.5)
	load := clamp01(float64(t.QueueDepth) / float64(maxInt(1, p.MaxConcurrency*2)))
	total := w.Quality*q - w.Cost*costRatio - w.Latency*latencyNorm - w.FailureRisk*risk - w.Load*load
	return ScoreBreakdown{Quality: q, Cost: costRatio, Latency: latencyNorm, FailureRisk: risk, Load: load, Total: total}
}
func decisionID(task CognitiveTask, candidates []Candidate, rejected []CandidateRejection, version uint32) DecisionID {
	h := sha256.New()
	taskJSON, _ := json.Marshal(task)
	_, _ = h.Write(taskJSON)
	fmt.Fprintf(h, "|policy:%d", version)
	for _, c := range candidates {
		fmt.Fprintf(h, "|%s@%d:%.12f", c.Model.Key(), c.Model.Version, c.Score.Total)
	}
	for _, x := range rejected {
		fmt.Fprintf(h, "|rej:%s@%d:%s", x.Model.Key(), x.Model.Version, x.Code)
	}
	sum := h.Sum(nil)
	return DecisionID("rd-" + hex.EncodeToString(sum[:12]))
}
func sortRejections(in []CandidateRejection) {
	sort.Slice(in, func(i, j int) bool {
		if in[i].Model.Key() != in[j].Model.Key() {
			return in[i].Model.Key() < in[j].Model.Key()
		}
		return in[i].Code < in[j].Code
	})
}
func containsProvider(xs []ProviderID, v ProviderID) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func containsModel(xs []ModelID, v ModelID) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// AdvisoryResult records what a shadow/learned router proposed without allowing
// that proposal to weaken the deterministic hard-constraint path.
type AdvisoryResult struct {
	Live        RoutingDecision `json:"live"`
	Proposed    ModelRef        `json:"proposed"`
	Eligible    bool            `json:"eligible"`
	WouldSelect bool            `json:"would_select"`
	Reason      string          `json:"reason,omitempty"`
}

func (r *Router) ShadowAdvisory(task CognitiveTask, proposed ModelRef) (AdvisoryResult, error) {
	live, err := r.Route(task, nil)
	if err != nil {
		return AdvisoryResult{}, err
	}
	result := AdvisoryResult{Live: live, Proposed: proposed}
	for _, candidate := range live.Candidates {
		if candidate.Model == proposed {
			result.Eligible = true
			result.WouldSelect = live.Selected == proposed
			return result, nil
		}
	}
	result.Reason = "proposal is outside the hard-eligible candidate set"
	return result, nil
}

func EstimateResources(task CognitiveTask, profile ModelProfile) Resources {
	return estimateCost(task, profile)
}
