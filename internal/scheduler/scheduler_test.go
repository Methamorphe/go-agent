package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}
func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	f.mu.Unlock()
}

func testProfile(provider, model string, locality Locality, quality float64, latency time.Duration) ModelProfile {
	return ModelProfile{
		ProviderID:                 ProviderID(provider),
		ModelID:                    ModelID(model),
		ContextWindow:              32768,
		MaxOutputTokens:            4096,
		Capabilities:               []Capability{CapabilityStreaming, CapabilityToolCalling, CapabilityStructuredOutput},
		Locality:                   locality,
		DataPolicy:                 DataPolicy{AllowsSensitive: locality == LocalityLocal, TrainingOptOut: true},
		InputCostMicrosPerMillion:  1000,
		OutputCostMicrosPerMillion: 2000,
		LatencyP95:                 latency,
		Reliability:                0.99,
		Quality:                    map[TaskKind]float64{TaskCodeReview: quality, TaskExtraction: quality, TaskConversation: quality},
		MaxConcurrency:             4,
		ProfileVersion:             1,
	}
}

func testTask() CognitiveTask {
	return CognitiveTask{
		ID: TaskID("task-1"), AgentID: id.AgentID("agent-1"), RootAgentID: id.AgentID("root-1"),
		Kind: TaskCodeReview, EstimatedInputTokens: 4000, ReservedOutputTokens: 1000,
		Objective: ObjectiveBalanced, Budget: Resources{MoneyMicros: 100000, Tokens: 10000},
		Privacy: PrivacyStandard, Risk: RiskNormal,
	}
}

func routerWith(t *testing.T, clock Clock, profiles ...ModelProfile) *Router {
	t.Helper()
	registry := NewRegistry(256)
	for _, profile := range profiles {
		if err := registry.Register(profile); err != nil {
			t.Fatal(err)
		}
	}
	return NewRouter(registry, NewTelemetry(256, clock), clock)
}

func TestSCH001PrivacyHardFilter(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0).UTC()}
	cloud := testProfile("cloud", "best", LocalityCloud, 0.99, time.Second)
	local := testProfile("local", "private", LocalityLocal, 0.70, 2*time.Second)
	router := routerWith(t, clock, cloud, local)
	task := testTask()
	task.Privacy = PrivacyLocalOnly

	decision, err := router.Route(task, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Selected != local.Ref() {
		t.Fatalf("selected %v, want local", decision.Selected)
	}
	if len(decision.Rejected) != 1 || decision.Rejected[0].Code != RejectPrivacy {
		t.Fatalf("unexpected rejections: %#v", decision.Rejected)
	}
}

func TestSCH002ContextHardFilter(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0).UTC()}
	small := testProfile("p", "small", LocalityCloud, 1, time.Millisecond)
	small.ContextWindow = 4096
	large := testProfile("p", "large", LocalityCloud, .5, 5*time.Second)
	large.ContextWindow = 65536
	router := routerWith(t, clock, small, large)
	task := testTask()
	task.EstimatedInputTokens = 8000

	decision, err := router.Route(task, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Selected.ModelID != "large" {
		t.Fatalf("selected %v", decision.Selected)
	}
}

func TestSCH003ConcurrentBudgetReservationPreventsOverspend(t *testing.T) {
	ledger := NewBudgetLedger()
	root := id.AgentID("root")
	if err := ledger.SetLimit(root, Resources{MoneyMicros: 1000, Tokens: 1000}); err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int64
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := ledger.Reserve(root, Resources{MoneyMicros: 100, Tokens: 100}); err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 10 {
		t.Fatalf("reservations=%d want=10", successes.Load())
	}
	snap := ledger.Snapshot(root)
	if snap.Reserved != (Resources{MoneyMicros: 1000, Tokens: 1000}) || snap.Available != (Resources{}) {
		t.Fatalf("bad snapshot: %#v", snap)
	}
}

func TestSCH004FallbackRebuildsFromTaskNotProviderThread(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0).UTC()}
	a := testProfile("p1", "a", LocalityCloud, .9, time.Second)
	b := testProfile("p2", "b", LocalityCloud, .8, 2*time.Second)
	router := routerWith(t, clock, a, b)
	task := testTask()
	first, err := router.Route(task, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := router.Route(task, map[string]struct{}{first.Selected.Key(): {}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Selected == second.Selected {
		t.Fatal("fallback selected the failed model")
	}
	if second.TaskID != task.ID || task.AgentID != "agent-1" || task.RootAgentID != "root-1" {
		t.Fatal("task/process identity changed during fallback")
	}
}

func TestSCH005CircuitBreakerStopsStorm(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0).UTC()}
	primary := testProfile("p1", "primary", LocalityCloud, .95, time.Second)
	fallback := testProfile("p2", "fallback", LocalityCloud, .7, 2*time.Second)
	router := routerWith(t, clock, primary, fallback)
	for range 3 {
		router.Telemetry().Observe(primary.Ref(), ObservationTransientFailure, time.Second)
	}
	if got := router.Telemetry().Snapshot(primary.Ref()).State; got != HealthUnavailable {
		t.Fatalf("health=%s", got)
	}
	decision, err := router.Route(testTask(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Selected != fallback.Ref() {
		t.Fatalf("selected %v", decision.Selected)
	}
	clock.Advance(31 * time.Second)
	if got := router.Telemetry().Snapshot(primary.Ref()).State; got != HealthRecovering {
		t.Fatalf("health after cooldown=%s", got)
	}
	router.Telemetry().Observe(primary.Ref(), ObservationSuccess, time.Second)
	if got := router.Telemetry().Snapshot(primary.Ref()).State; got != HealthHealthy {
		t.Fatalf("health after probe=%s", got)
	}
}

func TestSCH006HighRiskQualityFloor(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0).UTC()}
	cheap := testProfile("p", "cheap", LocalityCloud, .69, time.Millisecond)
	strong := testProfile("p", "strong", LocalityCloud, .85, 3*time.Second)
	router := routerWith(t, clock, cheap, strong)
	task := testTask()
	task.Risk = RiskHigh
	decision, err := router.Route(task, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Selected != strong.Ref() {
		t.Fatalf("selected %v", decision.Selected)
	}
}

func TestSCH007QueuePressureChangesRoute(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0).UTC()}
	fast := testProfile("local", "fast", LocalityLocal, .8, time.Second)
	fast.MaxConcurrency = 1
	slow := testProfile("local", "idle", LocalityLocal, .8, 2*time.Second)
	slow.MaxConcurrency = 1
	router := routerWith(t, clock, fast, slow)
	task := testTask()
	task.Objective = ObjectiveLatencyFirst
	task.Privacy = PrivacyLocalOnly
	first, err := router.Route(task, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Selected != fast.Ref() {
		t.Fatalf("initial selected %v", first.Selected)
	}
	router.Telemetry().SetQueueDepth(fast.Ref(), 10)
	second, err := router.Route(task, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.Selected != slow.Ref() {
		t.Fatalf("under pressure selected %v", second.Selected)
	}
}

func TestSCH008DecisionDeterministic(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0).UTC()}
	router := routerWith(t, clock,
		testProfile("b", "m", LocalityCloud, .8, 2*time.Second),
		testProfile("a", "m", LocalityCloud, .8, 2*time.Second),
	)
	task := testTask()
	a, err := router.Route(task, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := router.Route(task, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID || a.Selected != b.Selected || a.Score != b.Score {
		t.Fatalf("non-deterministic decisions: %#v %#v", a, b)
	}
}

func TestSCH009HedgeCancelsLoser(t *testing.T) {
	cancelled := make(chan struct{})
	primary := func(ctx context.Context) (string, error) {
		<-ctx.Done()
		close(cancelled)
		return "", ctx.Err()
	}
	secondary := func(context.Context) (string, error) { return "winner", nil }
	got, err := Hedge(context.Background(), time.Millisecond, primary, secondary)
	if err != nil || got != "winner" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("hedge loser was not cancelled")
	}
}

func TestSCH010RootFairness(t *testing.T) {
	slots := NewSlots(SlotConfig{Global: 4, PerRoot: 2, DefaultPerProvider: 4})
	a1, err := slots.Acquire("root-a", "p", 4)
	if err != nil {
		t.Fatal(err)
	}
	defer a1.Release()
	a2, err := slots.Acquire("root-a", "p", 4)
	if err != nil {
		t.Fatal(err)
	}
	defer a2.Release()
	if _, err := slots.Acquire("root-a", "p", 4); !errors.Is(err, ErrSlotsExhausted) {
		t.Fatalf("third root-a acquire err=%v", err)
	}
	b, err := slots.Acquire("root-b", "p", 4)
	if err != nil {
		t.Fatalf("root-b should retain fair capacity: %v", err)
	}
	b.Release()
}

func TestSCH011ShadowCannotOverrideHardConstraint(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0).UTC()}
	cloud := testProfile("cloud", "unsafe", LocalityCloud, .99, time.Millisecond)
	local := testProfile("local", "safe", LocalityLocal, .6, 2*time.Second)
	router := routerWith(t, clock, cloud, local)
	task := testTask()
	task.Privacy = PrivacyLocalOnly
	shadow, err := router.ShadowAdvisory(task, cloud.Ref())
	if err != nil {
		t.Fatal(err)
	}
	if shadow.Eligible || shadow.Live.Selected != local.Ref() {
		t.Fatalf("shadow bypassed hard constraints: %#v", shadow)
	}
}

func TestSCH012NoEligibleStructuredFailure(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0).UTC()}
	cloud := testProfile("cloud", "only", LocalityCloud, .9, time.Second)
	router := routerWith(t, clock, cloud)
	task := testTask()
	task.Privacy = PrivacyLocalOnly
	_, err := router.Route(task, nil)
	if !errors.Is(err, ErrNoEligibleModel) {
		t.Fatalf("err=%v", err)
	}
	var structured *NoEligibleError
	if !errors.As(err, &structured) || len(structured.Rejected) != 1 || structured.Rejected[0].Code != RejectPrivacy {
		t.Fatalf("not structured: %#v", err)
	}
}

func TestSCH013ActualCostSettlesExactlyOnce(t *testing.T) {
	ledger := NewBudgetLedger()
	if err := ledger.SetLimit("root", Resources{MoneyMicros: 1000, Tokens: 1000}); err != nil {
		t.Fatal(err)
	}
	rid, err := ledger.Reserve("root", Resources{MoneyMicros: 800, Tokens: 800})
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Settle(rid, Resources{MoneyMicros: 300, Tokens: 400}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Settle(rid, Resources{MoneyMicros: 1, Tokens: 1}); !errors.Is(err, ErrReservationSettled) {
		t.Fatalf("second settle err=%v", err)
	}
	snap := ledger.Snapshot("root")
	if snap.Spent != (Resources{MoneyMicros: 300, Tokens: 400}) || snap.Reserved != (Resources{}) {
		t.Fatalf("snapshot=%#v", snap)
	}
}

func TestSCH014ModelSwitchPreservesAgentIdentity(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0).UTC()}
	a := testProfile("p1", "a", LocalityCloud, .9, time.Second)
	b := testProfile("p2", "b", LocalityCloud, .8, 2*time.Second)
	router := routerWith(t, clock, a, b)
	task := testTask()
	first, err := router.Route(task, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := router.Route(task, map[string]struct{}{first.Selected.Key(): {}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Selected == second.Selected {
		t.Fatal("model did not switch")
	}
	if task.AgentID != id.AgentID("agent-1") || task.RootAgentID != id.AgentID("root-1") || task.ID != TaskID("task-1") {
		t.Fatal("durable identity mutated")
	}
}

func TestRuntimePrepareReservesAndAccounts(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0).UTC()}
	profile := testProfile("p", "m", LocalityCloud, .8, time.Second)
	router := routerWith(t, clock, profile)
	budgets := NewBudgetLedger()
	if err := budgets.SetLimit("root-1", Resources{MoneyMicros: 100000, Tokens: 10000}); err != nil {
		t.Fatal(err)
	}
	runtime := NewRuntime(router, budgets, NewSlots(SlotConfig{Global: 2, PerRoot: 1, DefaultPerProvider: 1}))
	attempt, err := runtime.Prepare(testTask(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if budgets.Snapshot("root-1").Reserved == (Resources{}) {
		t.Fatal("missing reservation")
	}
	clock.Advance(time.Second)
	actual := Resources{MoneyMicros: attempt.Reserved.MoneyMicros / 2, Tokens: attempt.Reserved.Tokens / 2}
	if err := runtime.Finish(attempt, actual, ObservationSuccess); err != nil {
		t.Fatal(err)
	}
	if budgets.Snapshot("root-1").Spent != actual {
		t.Fatal("actual usage not settled")
	}
	metrics := runtime.Metrics.Snapshot()
	if metrics.Decisions != 1 || metrics.Completed != 1 {
		t.Fatalf("metrics=%#v", metrics)
	}
}

func TestSCH015HundredProfilesNoHistoryDependency(t *testing.T) {
	clock := &fakeClock{now: time.Unix(100, 0).UTC()}
	registry := NewRegistry(100)
	for i := range 100 {
		profile := testProfile(fmt.Sprintf("p%03d", i), "m", LocalityCloud, .5+float64(i)/1000, time.Duration(i+1)*time.Millisecond)
		if err := registry.Register(profile); err != nil {
			t.Fatal(err)
		}
	}
	router := NewRouter(registry, NewTelemetry(100, clock), clock)
	decision, err := router.Route(testTask(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.Candidates) != 100 {
		t.Fatalf("candidates=%d", len(decision.Candidates))
	}
}

func BenchmarkRoute100Profiles(b *testing.B) {
	clock := &fakeClock{now: time.Unix(100, 0).UTC()}
	registry := NewRegistry(100)
	for i := range 100 {
		_ = registry.Register(testProfile(fmt.Sprintf("p%03d", i), "m", LocalityCloud, .8, time.Second))
	}
	router := NewRouter(registry, NewTelemetry(100, clock), clock)
	task := testTask()
	b.ResetTimer()
	for range b.N {
		if _, err := router.Route(task, nil); err != nil {
			b.Fatal(err)
		}
	}
}
