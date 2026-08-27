package process

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
)

func TestTerminalProcessCannotRunAgain(
	t *testing.T,
) {
	state := testReadyState(t)

	running := testEvent(
		t,
		state,
		EventAgentActivated,
		AgentActivatedPayload{
			LeaseID: "lse_test",

			RuntimeInstanceID: "run_test",
		},
	)

	state = mustApply(
		t,
		state,
		running,
	)

	completed := testEvent(
		t,
		state,
		EventAgentCompleted,
		AgentCompletedPayload{},
	)

	state = mustApply(
		t,
		state,
		completed,
	)

	activate := testEvent(
		t,
		state,
		EventAgentActivated,
		AgentActivatedPayload{
			LeaseID: "lse_second",

			RuntimeInstanceID: "run_test",
		},
	)

	if _, err := Apply(
		state,
		activate,
	); err == nil {
		t.Fatal(
			"terminal process was reactivated",
		)
	}
}

func TestTimestampCannotReorderProcess(
	t *testing.T,
) {
	state := testReadyState(t)

	event := testEvent(
		t,
		state,
		EventAgentSuspended,
		AgentSuspendedPayload{},
	)

	event.Timestamp =
		state.UpdatedAt.Add(
			-24 * time.Hour,
		)

	next, err := Apply(
		state,
		event,
	)
	if err != nil {
		t.Fatal(err)
	}

	if next.Version !=
		state.Version+1 {
		t.Fatal(
			"process version did not define ordering",
		)
	}
}

func TestHistoricalAgentCreatedPayloadUpcasts(
	t *testing.T,
) {
	parent :=
		id.AgentID("agt_parent")

	body, err :=
		json.Marshal(
			agentCreatedPayloadV0{
				ParentAgentID: &parent,
			},
		)
	if err != nil {
		t.Fatal(err)
	}

	event := ledger.Event{
		ID: "evt_legacy",

		AgentID: "agt_child",

		RootAgentID: "agt_root",

		ProcessVersion: 1,

		Type: EventAgentCreated,

		SchemaVersion: 0,

		Timestamp: time.Date(
			2026,
			time.August,
			27,
			12,
			0,
			0,
			0,
			time.UTC,
		),

		CorrelationID: "cor_test",

		Actor: ledger.ActorRef{
			Kind: ledger.ActorSystem,
		},

		Payload: body,
	}

	state, err := Apply(
		State{},
		event,
	)
	if err != nil {
		t.Fatal(err)
	}

	if state.LineageDepth != 1 {
		t.Fatalf(
			"depth = %d, want 1",
			state.LineageDepth,
		)
	}
}
