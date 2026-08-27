package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
)

func TestReplayAndSnapshotTailEqualCurrentProjection(t *testing.T) {
	ctx, store, service, fakeClock := newProcessHarness(t)

	state := mustCreateRoot(t, ctx, service, "req_create_replay")
	state = mustActivate(t, ctx, service, state, "req_activate_replay")

	fakeClock.Advance(time.Second)
	state, err := service.Yield(
		ctx,
		state.AgentID,
		versionPtr(state.Version),
		"cooperative yield",
		commandMeta("req_yield_replay"),
	)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := service.Snapshot(ctx, state.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ThroughProcessVersion != state.Version {
		t.Fatalf("snapshot version = %d, want %d", snapshot.ThroughProcessVersion, state.Version)
	}

	fakeClock.Advance(time.Second)
	state, err = service.Suspend(
		ctx,
		state.AgentID,
		versionPtr(state.Version),
		"test suspension",
		commandMeta("req_suspend_replay"),
	)
	if err != nil {
		t.Fatal(err)
	}

	fakeClock.Advance(time.Second)
	state, err = service.Resume(
		ctx,
		state.AgentID,
		versionPtr(state.Version),
		"test resume",
		commandMeta("req_resume_replay"),
	)
	if err != nil {
		t.Fatal(err)
	}

	current, err := service.Inspect(ctx, state.AgentID)
	if err != nil {
		t.Fatal(err)
	}

	reconstructed, err := service.Reconstruct(ctx, state.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reconstructed, current) {
		t.Fatalf("snapshot+tail reconstruction differs from current projection\nreconstructed: %#v\ncurrent: %#v", reconstructed, current)
	}

	events, err := store.Events(ctx, state.AgentID, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	var replayed agentprocess.State
	for _, event := range events {
		replayed, err = agentprocess.Apply(replayed, event)
		if err != nil {
			t.Fatalf("full replay at version %d: %v", event.ProcessVersion, err)
		}
	}
	if !reflect.DeepEqual(replayed, current) {
		t.Fatalf("full replay differs from current projection\nreplayed: %#v\ncurrent: %#v", replayed, current)
	}
}

func TestDuplicateRequestIDReturnsStoredResultWithoutDuplicateEvent(t *testing.T) {
	ctx, store, service, _ := newProcessHarness(t)
	state := mustCreateRoot(t, ctx, service, "req_create_idempotent")

	meta := commandMeta("req_suspend_idempotent")
	first, err := service.Suspend(
		ctx,
		state.AgentID,
		versionPtr(state.Version),
		"first call",
		meta,
	)
	if err != nil {
		t.Fatal(err)
	}

	second, err := service.Suspend(
		ctx,
		state.AgentID,
		versionPtr(state.Version),
		"payload may differ on a retried transport request",
		meta,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(second, first) {
		t.Fatalf("idempotent retry returned a different result\nfirst: %#v\nsecond: %#v", first, second)
	}

	events, err := store.Events(ctx, state.AgentID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("event count = %d, want 4", len(events))
	}
}

func TestSameExpectedVersionAllowsOnlyOneTransition(t *testing.T) {
	ctx, store, service, _ := newProcessHarness(t)
	state := mustCreateRoot(t, ctx, service, "req_create_cas")
	expected := state.Version

	type result struct {
		state agentprocess.State
		err   error
	}

	results := make(chan result, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	for i, requestID := range []id.RequestID{"req_cas_a", "req_cas_b"} {
		i := i
		requestID := requestID
		go func() {
			defer wg.Done()
			next, err := service.Suspend(
				ctx,
				state.AgentID,
				&expected,
				[]string{"first contender", "second contender"}[i],
				commandMeta(requestID),
			)
			results <- result{state: next, err: err}
		}()
	}

	wg.Wait()
	close(results)

	var successes int
	var conflicts int
	for result := range results {
		switch {
		case result.err == nil:
			successes++
			if result.state.Version != expected+1 {
				t.Fatalf("winner version = %d, want %d", result.state.Version, expected+1)
			}
		case errs.IsCode(result.err, errs.CodeConflict):
			conflicts++
		default:
			t.Fatalf("unexpected transition error: %v", result.err)
		}
	}

	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}

	events, err := store.Events(ctx, state.AgentID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("event count = %d, want 4", len(events))
	}
}

func TestAppendRollsBackProjectionAndEventWhenReceiptConflicts(t *testing.T) {
	ctx, store, service, fakeClock := newProcessHarness(t)
	state := mustCreateRoot(t, ctx, service, "req_receipt_already_exists")

	fakeClock.Advance(time.Second)
	body, err := json.Marshal(agentprocess.AgentSuspendedPayload{Reason: "must roll back"})
	if err != nil {
		t.Fatal(err)
	}
	causation := state.LastEventID
	event := ledger.Event{
		ID:             "evt_atomic_rollback",
		AgentID:        state.AgentID,
		RootAgentID:    state.RootAgentID,
		ProcessVersion: state.Version + 1,
		Type:           agentprocess.EventAgentSuspended,
		SchemaVersion:  agentprocess.EventSchemaVersion,
		Timestamp:      fakeClock.Now(),
		CausationID:    &causation,
		CorrelationID:  "cor_atomic_rollback",
		Actor: ledger.ActorRef{
			Kind: ledger.ActorSystem,
			ID:   "test",
		},
		Payload: body,
	}

	next, err := agentprocess.Apply(state, event)
	if err != nil {
		t.Fatal(err)
	}
	resultJSON, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}

	err = store.Append(
		ctx,
		state.Version,
		[]ledger.Event{event},
		next,
		agentprocess.Receipt{
			RequestID:     "req_receipt_already_exists",
			CommandType:   "process.suspend",
			AgentID:       next.AgentID,
			ResultVersion: next.Version,
			ResultJSON:    resultJSON,
			LastEventID:   next.LastEventID,
			CreatedAt:     fakeClock.Now(),
		},
	)
	if !errs.IsCode(err, errs.CodeConflict) {
		t.Fatalf("append error = %v, want conflict", err)
	}

	current, err := store.Current(ctx, state.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Version != state.Version || current.Status != state.Status {
		t.Fatalf("projection changed despite rollback: version=%d status=%s", current.Version, current.Status)
	}

	events, err := store.Events(ctx, state.AgentID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3 after rollback", len(events))
	}
}

func TestChildLineageAndCausationAreDurable(t *testing.T) {
	ctx, store, service, _ := newProcessHarness(t)
	root := mustCreateRoot(t, ctx, service, "req_create_parent")

	child, err := service.CreateChild(
		ctx,
		root.AgentID,
		commandMeta("req_create_child"),
	)
	if err != nil {
		t.Fatal(err)
	}

	if child.RootAgentID != root.AgentID {
		t.Fatalf("child root = %s, want %s", child.RootAgentID, root.AgentID)
	}
	if child.ParentAgentID == nil || *child.ParentAgentID != root.AgentID {
		t.Fatalf("child parent = %v, want %s", child.ParentAgentID, root.AgentID)
	}
	if child.LineageDepth != 1 {
		t.Fatalf("child depth = %d, want 1", child.LineageDepth)
	}
	if !reflect.DeepEqual(child.RootIntent, root.RootIntent) {
		t.Fatalf("child root intent differs from parent root intent")
	}

	events, err := store.Events(ctx, child.AgentID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("child event count = %d, want 3", len(events))
	}
	if events[0].CausationID == nil || *events[0].CausationID != root.LastEventID {
		t.Fatalf("child creation causation = %v, want %s", events[0].CausationID, root.LastEventID)
	}
	if events[1].CausationID == nil || *events[1].CausationID != events[0].ID {
		t.Fatalf("child intent causation = %v, want %s", events[1].CausationID, events[0].ID)
	}
	if events[2].CausationID == nil || *events[2].CausationID != events[1].ID {
		t.Fatalf("child ready causation = %v, want %s", events[2].CausationID, events[1].ID)
	}
}

func TestStaleDueWakeCannotReactivateChangedProcess(t *testing.T) {
	ctx, _, service, fakeClock := newProcessHarness(t)
	state := mustCreateRoot(t, ctx, service, "req_create_stale_wake")
	state = mustActivate(t, ctx, service, state, "req_activate_stale_wake")

	wakeAt := fakeClock.Now().Add(time.Hour)
	state, err := service.Sleep(
		ctx,
		state.AgentID,
		versionPtr(state.Version),
		wakeAt,
		commandMeta("req_sleep_stale_wake"),
	)
	if err != nil {
		t.Fatal(err)
	}

	due, err := service.DueSleeps(ctx, wakeAt.Add(time.Second), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 {
		t.Fatalf("due sleeps = %d, want 1", len(due))
	}
	stale := due[0]

	state, err = service.Suspend(
		ctx,
		state.AgentID,
		versionPtr(state.Version),
		"state changed before stale scheduler result",
		commandMeta("req_suspend_stale_wake"),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.Wake(ctx, stale, commandMeta("req_stale_wake"))
	if !errs.IsCode(err, errs.CodeConflict) {
		t.Fatalf("stale wake error = %v, want conflict", err)
	}

	current, err := service.Inspect(ctx, state.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != agentprocess.StatusSuspended || current.Version != state.Version {
		t.Fatalf("stale wake changed process: status=%s version=%d", current.Status, current.Version)
	}

	due, err = service.DueSleeps(ctx, wakeAt.Add(2*time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("wake condition survived suspension: %d rows", len(due))
	}
}

func newProcessHarness(t *testing.T) (context.Context, *Store, *agentprocess.Service, *clock.FakeClock) {
	t.Helper()
	ctx := context.Background()
	fakeClock := clock.NewFakeClock(time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC))
	store, err := Open(ctx, Config{
		Path:         filepath.Join(t.TempDir(), "runtime.db"),
		BusyTimeout:  5 * time.Second,
		MaxOpenConns: 8,
		Clock:        fakeClock,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	service := agentprocess.NewService(store, id.NewGenerator(), fakeClock)
	return ctx, store, service, fakeClock
}

func mustCreateRoot(t *testing.T, ctx context.Context, service *agentprocess.Service, requestID id.RequestID) agentprocess.State {
	t.Helper()
	state, err := service.CreateRoot(ctx, "test durable root intent", commandMeta(requestID))
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func mustActivate(t *testing.T, ctx context.Context, service *agentprocess.Service, state agentprocess.State, requestID id.RequestID) agentprocess.State {
	t.Helper()
	next, err := service.Activate(
		ctx,
		state.AgentID,
		"run_test",
		versionPtr(state.Version),
		commandMeta(requestID),
	)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func commandMeta(requestID id.RequestID) agentprocess.CommandMeta {
	return agentprocess.CommandMeta{
		RequestID:     requestID,
		CorrelationID: id.CorrelationID("cor_" + requestID.String()),
		Actor: ledger.ActorRef{
			Kind: ledger.ActorSystem,
			ID:   "test",
		},
	}
}

func versionPtr(version uint64) *uint64 {
	return &version
}
