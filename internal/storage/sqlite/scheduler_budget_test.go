package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/scheduler"
)

func TestG6SchedulerBudgetSurvivesReopenWithActiveReservation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "runtime.db")
	cfg := Config{Path: path, BusyTimeout: 5 * time.Second, MaxOpenConns: 8, Clock: clock.NewFakeClock(time.Date(2026, 9, 7, 20, 0, 0, 0, time.UTC))}
	root := id.AgentID("agt_g6_root")
	limit := scheduler.Resources{MoneyMicros: 1_000, Tokens: 1_000}
	reserved := scheduler.Resources{MoneyMicros: 600, Tokens: 700}

	store, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetLimit(root, limit); err != nil {
		t.Fatal(err)
	}
	reservationID, err := store.Reserve(root, reserved)
	if err != nil {
		t.Fatal(err)
	}
	before := store.Snapshot(root)
	if before.Reserved != reserved || before.Available != (scheduler.Resources{MoneyMicros: 400, Tokens: 300}) {
		t.Fatalf("before reopen snapshot=%+v", before)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	after := store.Snapshot(root)
	if after != before {
		t.Fatalf("budget changed across reopen: before=%+v after=%+v", before, after)
	}
	if amount, ok := store.Reservation(reservationID); !ok || amount != reserved {
		t.Fatalf("active reservation lost across reopen: amount=%+v ok=%v", amount, ok)
	}
	if _, err := store.Reserve(root, scheduler.Resources{MoneyMicros: 401, Tokens: 1}); !errors.Is(err, scheduler.ErrBudgetExhausted) {
		t.Fatalf("overspend after reopen err=%v", err)
	}

	actual := scheduler.Resources{MoneyMicros: 200, Tokens: 250}
	if err := store.Settle(reservationID, actual); err != nil {
		t.Fatal(err)
	}
	settled := store.Snapshot(root)
	if settled.Spent != actual || settled.Reserved != (scheduler.Resources{}) || settled.Available != (scheduler.Resources{MoneyMicros: 800, Tokens: 750}) {
		t.Fatalf("settled snapshot=%+v", settled)
	}
	if err := store.Settle(reservationID, scheduler.Resources{}); !errors.Is(err, scheduler.ErrReservationSettled) {
		t.Fatalf("second settlement err=%v", err)
	}
}

func TestG6SchedulerSpentBudgetSurvivesSecondReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "runtime.db")
	cfg := Config{Path: path, BusyTimeout: 5 * time.Second, MaxOpenConns: 8, Clock: clock.NewFakeClock(time.Date(2026, 9, 7, 20, 0, 0, 0, time.UTC))}
	root := id.AgentID("agt_g6_spent")
	limit := scheduler.Resources{MoneyMicros: 10_000, Tokens: 10_000}

	store, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetLimit(root, limit); err != nil {
		t.Fatal(err)
	}
	reservationID, err := store.Reserve(root, scheduler.Resources{MoneyMicros: 2_000, Tokens: 3_000})
	if err != nil {
		t.Fatal(err)
	}
	actual := scheduler.Resources{MoneyMicros: 900, Tokens: 1_200}
	if err := store.Settle(reservationID, actual); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snapshot := store.Snapshot(root)
	if snapshot.Spent != actual || snapshot.Available != limit.Sub(actual) {
		t.Fatalf("spent budget reset across restart: %+v", snapshot)
	}
}
