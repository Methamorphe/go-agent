package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/mmu"
	"github.com/Methamorphe/go-agent/internal/objectstore"
)

func TestG10ResolvedFaultIsClaimedByNextPersistedManifest(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, Config{Path: filepath.Join(t.TempDir(), "g10.db"), MaxOpenConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	request := mmu.ContextFaultRequest{ID: "flt_test", AgentID: "agt_test", InvocationID: "inv_source", Kind: mmu.FaultReference, Purpose: mmu.PurposeContinueReasoning, AllowedScopes: []mmu.Scope{mmu.ScopeProject}}
	if err := store.PutContextFault(ctx, mmu.FaultRecord{Request: request, State: mmu.FaultDetected, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.PutContextFault(ctx, mmu.FaultRecord{Request: request, State: mmu.FaultResolved, PageIDs: []id.ContextPageID{"ctx_test"}, Tokens: 42, CreatedAt: now, UpdatedAt: now.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}

	var eventCount int
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM context_fault_events WHERE fault_id = ?`, request.ID.String()).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 2 {
		t.Fatalf("fault lifecycle event count=%d, want 2", eventCount)
	}
	var firstState, lastState string
	if err := store.db.QueryRowContext(ctx, `SELECT state FROM context_fault_events WHERE fault_id = ? ORDER BY sequence LIMIT 1`, request.ID.String()).Scan(&firstState); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT state FROM context_fault_events WHERE fault_id = ? ORDER BY sequence DESC LIMIT 1`, request.ID.String()).Scan(&lastState); err != nil {
		t.Fatal(err)
	}
	if firstState != string(mmu.FaultDetected) || lastState != string(mmu.FaultResolved) {
		t.Fatalf("lifecycle states=%s..%s", firstState, lastState)
	}

	pending, err := store.PendingResolvedContextFaults(ctx, "agt_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Request.ID != "flt_test" {
		t.Fatalf("pending=%+v", pending)
	}
	if err := store.PutContextManifest(ctx, "inv_next", "agt_test", objectstore.Ref("sha256:manifest"), now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	pending, err = store.PendingResolvedContextFaults(ctx, "agt_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("fault was not claimed by next manifest: %+v", pending)
	}
}
