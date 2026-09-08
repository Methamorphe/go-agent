package workspace

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
)

type fakeProjectionStore struct {
	states    map[id.AgentID]agentprocess.State
	tree      []agentprocess.State
	events    map[id.AgentID][]ledger.Event
	inspector Inspector
	cursor    uint64
}

func (f *fakeProjectionStore) Current(_ context.Context, agentID id.AgentID) (agentprocess.State, error) {
	state, ok := f.states[agentID]
	if !ok {
		return agentprocess.State{}, fmt.Errorf("not found")
	}
	return state, nil
}

func (f *fakeProjectionStore) LatestRoot(context.Context) (agentprocess.State, error) {
	for _, state := range f.tree {
		if state.ParentAgentID == nil {
			return state, nil
		}
	}
	return agentprocess.State{}, fmt.Errorf("not found")
}

func (f *fakeProjectionStore) ProcessTree(_ context.Context, _ id.AgentID, limit int) ([]agentprocess.State, error) {
	if limit > len(f.tree) {
		limit = len(f.tree)
	}
	return append([]agentprocess.State(nil), f.tree[:limit]...), nil
}

func (f *fakeProjectionStore) WorkspaceEventsBefore(_ context.Context, agentID id.AgentID, before uint64, limit int) ([]ledger.Event, error) {
	all := f.events[agentID]
	filtered := make([]ledger.Event, 0, len(all))
	for _, event := range all {
		if before == 0 || event.ProcessVersion < before {
			filtered = append(filtered, event)
		}
	}
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	return filtered, nil
}

func (f *fakeProjectionStore) SearchWorkspaceEvents(_ context.Context, rootID id.AgentID, query string, limit int) ([]ledger.Event, error) {
	result := make([]ledger.Event, 0, limit)
	for _, events := range f.events {
		for _, event := range events {
			if event.RootAgentID == rootID && (query == "model" || query == string(event.Type)) {
				result = append(result, event)
				if len(result) == limit {
					return result, nil
				}
			}
		}
	}
	return result, nil
}

func (f *fakeProjectionStore) WorkspaceInspector(context.Context, id.AgentID) (Inspector, error) {
	return f.inspector, nil
}

func (f *fakeProjectionStore) WorkspaceCursor(context.Context, id.AgentID) (uint64, error) {
	return f.cursor, nil
}

func (f *fakeProjectionStore) ProcessTreeAfter(_ context.Context, _ id.AgentID, after uint64, limit int) ([]agentprocess.State, error) {
	if after >= f.cursor {
		return nil, nil
	}
	if limit > len(f.tree) {
		limit = len(f.tree)
	}
	return append([]agentprocess.State(nil), f.tree[:limit]...), nil
}

func (f *fakeProjectionStore) WorkspaceEventsAfter(_ context.Context, agentID id.AgentID, after uint64, limit int) ([]ledger.Event, error) {
	result := make([]ledger.Event, 0, limit)
	for _, event := range f.events[agentID] {
		if event.LedgerSequence > after {
			result = append(result, event)
			if len(result) == limit {
				break
			}
		}
	}
	return result, nil
}

func TestAttachIsBoundedAndCarriesCursor(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	rootID := id.AgentID("agt_root")
	childID := id.AgentID("agt_child")
	parent := rootID
	root := agentprocess.State{AgentID: rootID, RootAgentID: rootID, Status: agentprocess.StatusRunning, Version: 12, CreatedAt: now, UpdatedAt: now, RootIntent: &agentprocess.Intent{Goal: "ship G13"}}
	child := agentprocess.State{AgentID: childID, RootAgentID: rootID, ParentAgentID: &parent, LineageDepth: 1, Status: agentprocess.StatusWaiting, Version: 4, CreatedAt: now, UpdatedAt: now}
	store := &fakeProjectionStore{
		states: map[id.AgentID]agentprocess.State{rootID: root, childID: child},
		tree: []agentprocess.State{root, child},
		events: map[id.AgentID][]ledger.Event{rootID: syntheticEvents(rootID, rootID, 1, 150, now)},
		cursor: 150,
	}
	service := New(store, func() time.Time { return now })

	snapshot, err := service.Attach(context.Background(), AttachRequest{AgentID: rootID, Mode: ModeReview, HistoryLimit: 20, TreeLimit: 8})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ProtocolVersion != ProtocolVersion || snapshot.Cursor != 150 || snapshot.Mode != ModeReview {
		t.Fatalf("unexpected snapshot metadata: %#v", snapshot)
	}
	if got := len(snapshot.Viewport.Blocks); got != 20 {
		t.Fatalf("history viewport must be bounded to 20 blocks, got %d", got)
	}
	if !snapshot.Viewport.HasPrevious {
		t.Fatal("expected older history to remain available")
	}
	if got := len(snapshot.Tree); got != 2 {
		t.Fatalf("expected two process summaries, got %d", got)
	}
	if snapshot.Tree[0].Children != 1 {
		t.Fatalf("root child count mismatch: %d", snapshot.Tree[0].Children)
	}
}

func TestRefreshMarksClientBehindInsteadOfGrowingQueue(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	rootID := id.AgentID("agt_root")
	root := agentprocess.State{AgentID: rootID, RootAgentID: rootID, Status: agentprocess.StatusRunning, Version: 80, CreatedAt: now, UpdatedAt: now}
	store := &fakeProjectionStore{
		states: map[id.AgentID]agentprocess.State{rootID: root}, tree: []agentprocess.State{root},
		events: map[id.AgentID][]ledger.Event{rootID: syntheticEvents(rootID, rootID, 1, 80, now)}, cursor: 80,
	}
	service := New(store, func() time.Time { return now })
	refresh, err := service.Refresh(context.Background(), RefreshRequest{RootAgentID: rootID, FocusedAgentID: rootID, AfterSequence: 1, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !refresh.Behind {
		t.Fatal("expected bounded refresh to signal behind")
	}
	if len(refresh.Blocks) != 10 {
		t.Fatalf("refresh must return at most requested limit, got %d", len(refresh.Blocks))
	}
	if refresh.Cursor != 80 {
		t.Fatalf("unexpected cursor %d", refresh.Cursor)
	}
}

func TestEventProjectionNeverExposesHiddenReasoning(t *testing.T) {
	now := time.Now().UTC()
	event := ledger.Event{
		ID: "evt_1", AgentID: "agt_1", RootAgentID: "agt_1", LedgerSequence: 1, ProcessVersion: 1,
		Type: "ModelInvocationCompleted", Actor: ledger.ActorRef{Kind: ledger.ActorAgent}, Timestamp: now,
		Payload: []byte(`{"response_ref":"object://answer","finish_reason":"stop","private_chain_of_thought":"must-not-render"}`),
	}
	block := blockFromEvent(event)
	if block.Kind != BlockAssistantMessage {
		t.Fatalf("unexpected block kind %s", block.Kind)
	}
	if block.Preview == "must-not-render" {
		t.Fatal("hidden reasoning must never become projected preview")
	}
	if block.ObjectRef != "object://answer" {
		t.Fatalf("expected durable response ref, got %q", block.ObjectRef)
	}
}

func syntheticEvents(agentID, rootID id.AgentID, start, count int, now time.Time) []ledger.Event {
	events := make([]ledger.Event, 0, count)
	for index := 0; index < count; index++ {
		version := uint64(start + index)
		events = append(events, ledger.Event{
			ID: id.EventID(fmt.Sprintf("evt_%d", version)), AgentID: agentID, RootAgentID: rootID,
			LedgerSequence: version, ProcessVersion: version, Type: "ModelInvocationCompleted",
			Timestamp: now.Add(time.Duration(index) * time.Millisecond), Actor: ledger.ActorRef{Kind: ledger.ActorAgent},
			Payload: []byte(fmt.Sprintf(`{"response_ref":"object://%d","finish_reason":"stop","model":"m"}`, version)),
		})
	}
	return events
}
