package live

import (
	"strings"
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

func TestBusLongStreamingStressRemainsBounded(t *testing.T) {
	const maxBytes = 64 << 10
	const chunks = 100_000
	chunk := strings.Repeat("x", 37)
	bus := New(2, maxBytes)
	agentID := id.AgentID("agt_stream_stress")
	if err := bus.Begin(agentID, time.Now()); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < chunks; i++ {
		bus.Publish(agentID, chunk)
	}

	snapshot, ok := bus.Snapshot(agentID)
	if !ok {
		t.Fatal("missing stream after stress publish")
	}
	wantEnd := int64(chunks * len(chunk))
	if snapshot.EndOffset != wantEnd {
		t.Fatalf("end offset = %d, want %d", snapshot.EndOffset, wantEnd)
	}
	if len(snapshot.Data) != maxBytes {
		t.Fatalf("retained bytes = %d, want bounded %d", len(snapshot.Data), maxBytes)
	}
	if snapshot.StartOffset != wantEnd-int64(maxBytes) {
		t.Fatalf("start offset = %d, want %d", snapshot.StartOffset, wantEnd-int64(maxBytes))
	}
}
