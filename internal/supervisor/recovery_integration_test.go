package supervisor_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/storage/sqlite"
	"github.com/Methamorphe/go-agent/internal/supervisor"
)

func TestRecoverMovesPersistedRunningProcessBackToReady(t *testing.T) {
	ctx := context.Background()
	fakeClock := clock.NewFakeClock(time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC))

	store, err := sqlite.Open(ctx, sqlite.Config{
		Path:         filepath.Join(t.TempDir(), "runtime.db"),
		BusyTimeout:  5 * time.Second,
		MaxOpenConns: 8,
		Clock:        fakeClock,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	service := agentprocess.NewService(store, id.NewGenerator(), fakeClock)

	state, err := service.CreateRoot(
		ctx,
		"recover interrupted durable work",
		agentprocess.CommandMeta{
			RequestID:     "req_recovery_create",
			CorrelationID: "cor_recovery_create",
			Actor: ledger.ActorRef{
				Kind: ledger.ActorSystem,
				ID:   "integration-test",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	expected := state.Version
	state, err = service.Activate(
		ctx,
		state.AgentID,
		"run_interrupted",
		&expected,
		agentprocess.CommandMeta{
			RequestID:     "req_recovery_activate",
			CorrelationID: "cor_recovery_activate",
			Actor: ledger.ActorRef{
				Kind: ledger.ActorSupervisor,
				ID:   "run_interrupted",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if state.Status != agentprocess.StatusRunning || state.Version != 4 || state.Lease == nil {
		t.Fatalf("precondition failed: status=%s version=%d lease=%v", state.Status, state.Version, state.Lease)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	restarted := supervisor.New(logger, service, fakeClock, "run_restarted")

	if err := restarted.Recover(ctx); err != nil {
		t.Fatal(err)
	}

	current, err := service.Inspect(ctx, state.AgentID)
	if err != nil {
		t.Fatal(err)
	}

	if current.Status != agentprocess.StatusReady {
		t.Fatalf("status after recovery = %s, want READY", current.Status)
	}
	if current.Version != 5 {
		t.Fatalf("version after recovery = %d, want 5", current.Version)
	}
	if current.Lease != nil {
		t.Fatalf("stale execution lease survived recovery: %#v", current.Lease)
	}

	events, err := service.Events(ctx, state.AgentID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 5 {
		t.Fatalf("event count = %d, want 5", len(events))
	}

	last := events[len(events)-1]
	if last.Type != agentprocess.EventRecoveryActionApplied {
		t.Fatalf("last event = %s, want %s", last.Type, agentprocess.EventRecoveryActionApplied)
	}
	if last.Actor.Kind != ledger.ActorSupervisor || last.Actor.ID != "run_restarted" {
		t.Fatalf("recovery actor = %#v, want supervisor run_restarted", last.Actor)
	}
	if last.CausationID == nil || *last.CausationID != state.LastEventID {
		t.Fatalf("recovery causation = %v, want %s", last.CausationID, state.LastEventID)
	}

	reconstructed, err := service.Reconstruct(ctx, state.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if reconstructed.Status != current.Status ||
		reconstructed.Version != current.Version ||
		reconstructed.LastEventID != current.LastEventID {
		t.Fatalf("reconstructed recovery state differs: reconstructed=%#v current=%#v", reconstructed, current)
	}
}
