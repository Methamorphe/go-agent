package live

import (
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
)

func TestBusIsBoundedAndExposesOffsets(t *testing.T) {
	bus := New(2, 5)
	agentID := id.AgentID("agt_test")
	if err := bus.Begin(agentID, time.Now()); err != nil {
		t.Fatal(err)
	}
	bus.Publish(agentID, "hello")
	bus.Publish(agentID, " world")
	snapshot, ok := bus.Snapshot(agentID)
	if !ok {
		t.Fatal("missing stream")
	}
	if snapshot.Data != "world" {
		t.Fatalf("data = %q, want world", snapshot.Data)
	}
	if snapshot.StartOffset != 6 || snapshot.EndOffset != 11 {
		t.Fatalf("offsets = %d..%d", snapshot.StartOffset, snapshot.EndOffset)
	}
}
