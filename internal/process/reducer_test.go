package process

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
)

func TestTerminalProcessCannotRunAgain(t *testing.T) {
	state := testReadyState(t)

	state = mustApply(t, state, testEvent(
		t,
		state,
		EventAgentActivated,
		AgentActivatedPayload{
			LeaseID:           "lse_test",
			RuntimeInstanceID: "run_test",
		},
	))

	state = mustApply(t, state, testEvent(
		t,
		state,
		EventAgentCompleted,
		AgentCompletedPayload{},
	))

	_, err := Apply(state, testEvent(
		t,
		state,
		EventAgentActivated,
		AgentActivatedPayload{
			LeaseID:           "lse_second",
			RuntimeInstanceID: "run_test",
		},
	))
	if err == nil {
		t.Fatal("terminal process was reactivated")
	}
}

func TestTimestampCannotReorderProcess(t *testing.T) {
	state := testReadyState(t)
	event := testEvent(t, state, EventAgentSuspended, AgentSuspendedPayload{})
	event.Timestamp = state.UpdatedAt.Add(-24 * time.Hour)

	next, err := Apply(state, event)
	if err != nil {
		t.Fatal(err)
	}

	if next.Version != state.Version+1 {
		t.Fatalf("version = %d, want %d", next.Version, state.Version+1)
	}

	if next.UpdatedAt != event.Timestamp.UTC() {
		t.Fatalf("updated_at = %s, want event timestamp %s", next.UpdatedAt, event.Timestamp.UTC())
	}
}

func TestHistoricalAgentCreatedPayloadUpcasts(t *testing.T) {
	parent := id.AgentID("agt_parent")
	body, err := json.Marshal(agentCreatedPayloadV0{ParentAgentID: &parent})
	if err != nil {
		t.Fatal(err)
	}

	event := ledger.Event{
		ID:             "evt_legacy",
		AgentID:        "agt_child",
		RootAgentID:    "agt_root",
		ProcessVersion: 1,
		Type:           EventAgentCreated,
		SchemaVersion:  0,
		Timestamp:      testTime(),
		CorrelationID:  "cor_test",
		Actor: ledger.ActorRef{
			Kind: ledger.ActorSystem,
		},
		Payload: body,
	}

	state, err := Apply(State{}, event)
	if err != nil {
		t.Fatal(err)
	}

	if state.LineageDepth != 1 {
		t.Fatalf("depth = %d, want 1", state.LineageDepth)
	}
	if state.ParentAgentID == nil || *state.ParentAgentID != parent {
		t.Fatalf("parent = %v, want %s", state.ParentAgentID, parent)
	}
}

func TestStaleWakeIsRejected(t *testing.T) {
	state := testReadyState(t)
	state = mustApply(t, state, testEvent(
		t,
		state,
		EventAgentActivated,
		AgentActivatedPayload{
			LeaseID:           "lse_test",
			RuntimeInstanceID: "run_test",
		},
	))
	state = mustApply(t, state, testEvent(
		t,
		state,
		EventAgentSleepStarted,
		AgentSleepStartedPayload{
			SleepID:    "slp_current",
			WakeAt:     testTime().Add(time.Hour),
			Generation: 2,
		},
	))

	cases := []struct {
		name       string
		sleepID    id.SleepID
		generation uint64
	}{
		{name: "wrong sleep id", sleepID: "slp_stale", generation: 2},
		{name: "wrong generation", sleepID: "slp_current", generation: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Apply(state, testEvent(
				t,
				state,
				EventAgentWoken,
				AgentWokenPayload{
					SleepID:    tc.sleepID,
					Generation: tc.generation,
				},
			))
			if err == nil {
				t.Fatal("stale wake was accepted")
			}
		})
	}
}

func TestProcessVersionMustBeContiguous(t *testing.T) {
	state := testReadyState(t)
	event := testEvent(t, state, EventAgentSuspended, AgentSuspendedPayload{})
	event.ProcessVersion = state.Version + 2

	if _, err := Apply(state, event); err == nil {
		t.Fatal("version gap was accepted")
	}
}

func TestUnknownWaitingReasonIsRejected(t *testing.T) {
	state := testReadyState(t)
	state = mustApply(t, state, testEvent(
		t,
		state,
		EventAgentActivated,
		AgentActivatedPayload{
			LeaseID:           "lse_test",
			RuntimeInstanceID: "run_test",
		},
	))

	_, err := Apply(state, testEvent(
		t,
		state,
		EventAgentWaitStarted,
		AgentWaitStartedPayload{Reason: WaitingReason("WAITING_UNKNOWN")},
	))
	if err == nil {
		t.Fatal("unknown waiting reason was accepted")
	}
}

func TestCancelRequestedProcessCannotCompleteSuccessfully(t *testing.T) {
	state := testReadyState(t)
	state = mustApply(t, state, testEvent(
		t,
		state,
		EventAgentActivated,
		AgentActivatedPayload{
			LeaseID:           "lse_test",
			RuntimeInstanceID: "run_test",
		},
	))
	state = mustApply(t, state, testEvent(
		t,
		state,
		EventCancelRequested,
		CancelRequestedPayload{Scope: "process"},
	))

	_, err := Apply(state, testEvent(
		t,
		state,
		EventAgentCompleted,
		AgentCompletedPayload{},
	))
	if err == nil {
		t.Fatal("cancel-requested process completed successfully")
	}
}

func BenchmarkApplyReadyToSuspended(b *testing.B) {
	state := benchmarkReadyState(b)
	event := benchmarkEvent(
		b,
		state,
		EventAgentSuspended,
		AgentSuspendedPayload{Reason: "benchmark"},
	)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if _, err := Apply(state, event); err != nil {
			b.Fatal(err)
		}
	}
}

func testReadyState(t *testing.T) State {
	t.Helper()
	return buildReadyState(t)
}

func benchmarkReadyState(b *testing.B) State {
	b.Helper()
	return buildReadyState(b)
}

type testFataler interface {
	Helper()
	Fatal(args ...any)
}

func buildReadyState(tb testFataler) State {
	tb.Helper()
	now := testTime()
	intent := Intent{
		ID:            "int_test",
		SchemaVersion: IntentSchemaVersion,
		Goal:          "test durable process",
		CreatedAt:     now,
	}

	created := makeTestEvent(
		tb,
		"evt_1",
		"agt_test",
		"agt_test",
		1,
		EventAgentCreated,
		AgentCreatedPayload{},
		now,
	)
	state, err := Apply(State{}, created)
	if err != nil {
		tb.Fatal(err)
	}

	bound := makeTestEvent(
		tb,
		"evt_2",
		state.AgentID,
		state.RootAgentID,
		2,
		EventRootIntentBound,
		RootIntentBoundPayload{Intent: intent},
		now.Add(time.Second),
	)
	state, err = Apply(state, bound)
	if err != nil {
		tb.Fatal(err)
	}

	readied := makeTestEvent(
		tb,
		"evt_3",
		state.AgentID,
		state.RootAgentID,
		3,
		EventAgentReadied,
		AgentReadiedPayload{Reason: "test"},
		now.Add(2*time.Second),
	)
	state, err = Apply(state, readied)
	if err != nil {
		tb.Fatal(err)
	}

	return state
}

func testEvent(t *testing.T, state State, eventType ledger.EventType, payload any) ledger.Event {
	t.Helper()
	return makeTestEvent(
		t,
		id.EventID(fmt.Sprintf("evt_%d", state.Version+1)),
		state.AgentID,
		state.RootAgentID,
		state.Version+1,
		eventType,
		payload,
		state.UpdatedAt.Add(time.Second),
	)
}

func benchmarkEvent(b *testing.B, state State, eventType ledger.EventType, payload any) ledger.Event {
	b.Helper()
	return makeTestEvent(
		b,
		id.EventID(fmt.Sprintf("evt_%d", state.Version+1)),
		state.AgentID,
		state.RootAgentID,
		state.Version+1,
		eventType,
		payload,
		state.UpdatedAt.Add(time.Second),
	)
}

func makeTestEvent(
	tb testFataler,
	eventID id.EventID,
	agentID id.AgentID,
	rootID id.AgentID,
	version uint64,
	eventType ledger.EventType,
	payload any,
	timestamp time.Time,
) ledger.Event {
	tb.Helper()
	body, err := encodePayload(payload)
	if err != nil {
		tb.Fatal(err)
	}

	return ledger.Event{
		ID:             eventID,
		AgentID:        agentID,
		RootAgentID:    rootID,
		ProcessVersion: version,
		Type:           eventType,
		SchemaVersion:  EventSchemaVersion,
		Timestamp:      timestamp.UTC(),
		CorrelationID:  "cor_test",
		Actor: ledger.ActorRef{
			Kind: ledger.ActorSystem,
		},
		Payload: body,
	}
}

func mustApply(t *testing.T, state State, event ledger.Event) State {
	t.Helper()
	next, err := Apply(state, event)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func testTime() time.Time {
	return time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
}
