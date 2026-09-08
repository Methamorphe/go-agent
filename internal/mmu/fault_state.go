package mmu

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

type faultTurnKey struct { agent id.AgentID; invocation id.InvocationID }
type faultWindow struct {
	count int
	tokens int
	progress uint64
	fingerprints map[string]int
	updatedAt time.Time
}
type faultRuntime struct {
	mu sync.Mutex
	resolvers map[string]CognitiveResolver
	turns map[faultTurnKey]*faultWindow
	tasks map[string]*faultWindow
}
var faultRuntimes sync.Map

func faultRuntimeFor(m *Manager) *faultRuntime {
	if current, ok := faultRuntimes.Load(m); ok { return current.(*faultRuntime) }
	fresh := &faultRuntime{resolvers: map[string]CognitiveResolver{}, turns: map[faultTurnKey]*faultWindow{}, tasks: map[string]*faultWindow{}}
	actual, _ := faultRuntimes.LoadOrStore(m, fresh)
	return actual.(*faultRuntime)
}

func normalizeFaultBudget(b FaultBudget) FaultBudget {
	d := DefaultFaultBudget()
	if b.MaxFaultsPerTurn <= 0 { b.MaxFaultsPerTurn = d.MaxFaultsPerTurn }
	if b.MaxFaultsPerTaskWindow <= 0 { b.MaxFaultsPerTaskWindow = d.MaxFaultsPerTaskWindow }
	if b.MaxMaterializedTokens <= 0 { b.MaxMaterializedTokens = d.MaxMaterializedTokens }
	if b.MaxResolutionTime <= 0 { b.MaxResolutionTime = d.MaxResolutionTime }
	if b.MaxRepeatedSetHits <= 0 { b.MaxRepeatedSetHits = d.MaxRepeatedSetHits }
	return b
}

func (m *Manager) RegisterResolver(scheme string, resolver CognitiveResolver) error {
	scheme = strings.TrimSpace(strings.ToLower(scheme))
	if resolver == nil { return errs.New(errs.CodeInvalidArgument, "mmu.fault.register_resolver", "resolver is required") }
	switch scheme {
	case RefBelief, RefEvidence, RefObject, RefEvent, RefCheckpoint, RefAgent:
	default: return errs.New(errs.CodeInvalidArgument, "mmu.fault.register_resolver", "invalid resolver scheme")
	}
	rt := faultRuntimeFor(m); rt.mu.Lock(); rt.resolvers[scheme] = resolver; rt.mu.Unlock()
	return nil
}

func (m *Manager) reserveFault(request ContextFaultRequest, budget FaultBudget, now time.Time) error {
	rt := faultRuntimeFor(m); rt.mu.Lock(); defer rt.mu.Unlock()
	turnKey := faultTurnKey{agent: request.AgentID, invocation: request.InvocationID}
	turn := rt.turns[turnKey]
	if turn == nil { turn = &faultWindow{progress: request.ProgressEpoch, fingerprints: map[string]int{}}; rt.turns[turnKey] = turn }
	if turn.progress != request.ProgressEpoch { turn.count, turn.tokens, turn.fingerprints, turn.progress = 0, 0, map[string]int{}, request.ProgressEpoch }
	if turn.count >= budget.MaxFaultsPerTurn { return errs.New(errs.CodeResourceExhausted, "mmu.fault.budget", "maximum faults per invocation exceeded") }
	taskID := request.TaskWindowID; if taskID == "" { taskID = request.AgentID.String() }
	task := rt.tasks[taskID]
	if task == nil { task = &faultWindow{progress: request.ProgressEpoch, fingerprints: map[string]int{}}; rt.tasks[taskID] = task }
	if task.progress != request.ProgressEpoch { task.count, task.tokens, task.fingerprints, task.progress = 0, 0, map[string]int{}, request.ProgressEpoch }
	if task.count >= budget.MaxFaultsPerTaskWindow { return errs.New(errs.CodeResourceExhausted, "mmu.fault.budget", "maximum faults per task window exceeded") }
	fingerprint := faultFingerprint(request, nil)
	if turn.fingerprints[fingerprint] >= budget.MaxRepeatedSetHits || task.fingerprints[fingerprint] >= budget.MaxRepeatedSetHits { return errs.New(errs.CodeResourceExhausted, "mmu.fault.storm", "repeated context fault detected without progress") }
	turn.count++; turn.fingerprints[fingerprint]++; turn.updatedAt = now
	task.count++; task.fingerprints[fingerprint]++; task.updatedAt = now
	return nil
}

func (m *Manager) commitFaultTokens(request ContextFaultRequest, budget FaultBudget, tokens int, pages []id.ContextPageID) error {
	rt := faultRuntimeFor(m); rt.mu.Lock(); defer rt.mu.Unlock()
	turn := rt.turns[faultTurnKey{agent: request.AgentID, invocation: request.InvocationID}]
	taskID := request.TaskWindowID; if taskID == "" { taskID = request.AgentID.String() }
	task := rt.tasks[taskID]
	if turn == nil || task == nil { return errs.New(errs.CodeInternal, "mmu.fault.budget", "fault budget window disappeared") }
	if turn.tokens+tokens > budget.MaxMaterializedTokens || task.tokens+tokens > budget.MaxMaterializedTokens { return errs.New(errs.CodeResourceExhausted, "mmu.fault.budget", "materialized fault token budget exceeded") }
	fingerprint := faultFingerprint(request, pages)
	if turn.fingerprints[fingerprint] >= budget.MaxRepeatedSetHits || task.fingerprints[fingerprint] >= budget.MaxRepeatedSetHits { return errs.New(errs.CodeResourceExhausted, "mmu.fault.storm", "same resolved page set is thrashing") }
	turn.tokens += tokens; turn.fingerprints[fingerprint]++; turn.updatedAt = m.clock.Now().UTC()
	task.tokens += tokens; task.fingerprints[fingerprint]++; task.updatedAt = turn.updatedAt
	return nil
}

func faultFingerprint(request ContextFaultRequest, pages []id.ContextPageID) string {
	parts := []string{request.AgentID.String(), string(request.Kind), string(request.Purpose)}
	if request.Reference != nil { parts = append(parts, request.Reference.String()) }
	if request.Query != nil { parts = append(parts, strings.ToLower(strings.Join(strings.Fields(request.Query.Text), " "))) }
	ids := append([]id.ContextPageID(nil), pages...); sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, pageID := range ids { parts = append(parts, pageID.String()) }
	return strings.Join(parts, "|")
}
