package ledger

import (
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
)

func TestEventValidateAllowsHistoricalSchemaZero(t *testing.T) {
	event := validTestEvent()
	event.SchemaVersion = 0

	if err := event.Validate(); err != nil {
		t.Fatalf("historical schema version 0 should remain replayable: %v", err)
	}
}

func TestEventRejectsOversizedInlinePayload(t *testing.T) {
	event := validTestEvent()
	event.Payload = make([]byte, MaxInlinePayloadBytes+1)

	err := event.Validate()
	if !errs.IsCode(err, errs.CodeResourceExhausted) {
		t.Fatalf("error = %v, want resource_exhausted", err)
	}
}

func TestEventRequiresProcessVersion(t *testing.T) {
	event := validTestEvent()
	event.ProcessVersion = 0

	err := event.Validate()
	if !errs.IsCode(err, errs.CodeInvalidArgument) {
		t.Fatalf("error = %v, want invalid_argument", err)
	}
}

func BenchmarkEventValidate(b *testing.B) {
	event := validTestEvent()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if err := event.Validate(); err != nil {
			b.Fatal(err)
		}
	}
}

func validTestEvent() Event {
	return Event{
		ID:             "evt_test",
		AgentID:        "agt_test",
		RootAgentID:    "agt_test",
		ProcessVersion: 1,
		Type:           "AgentCreated",
		SchemaVersion:  1,
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
		Actor: ActorRef{
			Kind: ActorSystem,
		},
		Payload: []byte(`{}`),
	}
}
