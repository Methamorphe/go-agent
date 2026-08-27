package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/id"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
)

func BenchmarkProcessCurrentProjection(b *testing.B) {
	ctx, store, service, _ := newBenchmarkProcessHarness(b)
	state, err := service.CreateRoot(ctx, "benchmark current projection", commandMeta("req_bench_current_create"))
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if _, err := store.Current(ctx, state.AgentID); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessReconstructSnapshotAndTail(b *testing.B) {
	ctx, _, service, fakeClock := newBenchmarkProcessHarness(b)
	state, err := service.CreateRoot(ctx, "benchmark reconstruction", commandMeta("req_bench_reconstruct_create"))
	if err != nil {
		b.Fatal(err)
	}
	state, err = service.Activate(
		ctx,
		state.AgentID,
		"run_benchmark",
		versionPtr(state.Version),
		commandMeta("req_bench_reconstruct_activate"),
	)
	if err != nil {
		b.Fatal(err)
	}
	state, err = service.Yield(
		ctx,
		state.AgentID,
		versionPtr(state.Version),
		"benchmark",
		commandMeta("req_bench_reconstruct_yield"),
	)
	if err != nil {
		b.Fatal(err)
	}
	if _, err := service.Snapshot(ctx, state.AgentID); err != nil {
		b.Fatal(err)
	}

	for i := 0; i < 20; i++ {
		fakeClock.Advance(time.Millisecond)
		state, err = service.Suspend(
			ctx,
			state.AgentID,
			versionPtr(state.Version),
			"benchmark",
			commandMeta(id.RequestID("req_bench_suspend_" + benchmarkIntegerString(i))),
		)
		if err != nil {
			b.Fatal(err)
		}
		state, err = service.Resume(
			ctx,
			state.AgentID,
			versionPtr(state.Version),
			"benchmark",
			commandMeta(id.RequestID("req_bench_resume_" + benchmarkIntegerString(i))),
		)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		reconstructed, err := service.Reconstruct(ctx, state.AgentID)
		if err != nil {
			b.Fatal(err)
		}
		if reconstructed.Version != state.Version {
			b.Fatalf("version = %d, want %d", reconstructed.Version, state.Version)
		}
	}
}

func newBenchmarkProcessHarness(b *testing.B) (context.Context, *Store, *agentprocess.Service, *clock.FakeClock) {
	b.Helper()
	ctx := context.Background()
	fakeClock := clock.NewFakeClock(time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC))
	store, err := Open(ctx, Config{
		Path:         filepath.Join(b.TempDir(), "runtime.db"),
		BusyTimeout:  5 * time.Second,
		MaxOpenConns: 8,
		Clock:        fakeClock,
	})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := store.Close(); err != nil {
			b.Errorf("close store: %v", err)
		}
	})
	return ctx, store, agentprocess.NewService(store, id.NewGenerator(), fakeClock), fakeClock
}

func benchmarkIntegerString(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	position := len(digits)
	for value > 0 {
		position--
		digits[position] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[position:])
}
