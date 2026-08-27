package supervisor

import (
	"context"
	"io"
	"log/slog"
	"runtime"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
)

func TestRecoverReconcilesRunningProcessesAndDueSleeps(t *testing.T) {
	processes := &fakeProcessService{
		running: makeRunningStates(300),
		due:     makeDueSleeps(513),
	}
	fakeClock := clock.NewFakeClock(time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC))
	supervisor := New(testLogger(), processes, fakeClock, "run_recovery_test")

	if err := supervisor.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}

	if processes.recovered != 300 {
		t.Fatalf("recovered = %d, want 300", processes.recovered)
	}
	if processes.woken != 513 {
		t.Fatalf("woken = %d, want 513", processes.woken)
	}
	if processes.lastRecoverMeta.Actor.Kind != ledger.ActorSupervisor ||
		processes.lastRecoverMeta.Actor.ID != "run_recovery_test" {
		t.Fatalf("unexpected recovery actor: %#v", processes.lastRecoverMeta.Actor)
	}
	if processes.lastWakeMeta.Actor.Kind != ledger.ActorSupervisor ||
		processes.lastWakeMeta.Actor.ID != "run_recovery_test" {
		t.Fatalf("unexpected wake actor: %#v", processes.lastWakeMeta.Actor)
	}
}

func TestThousandsOfSleepingProcessesDoNotCreatePerProcessGoroutines(t *testing.T) {
	const sleepers = 10_000
	processes := &fakeProcessService{due: makeDueSleeps(sleepers)}
	fakeClock := clock.NewFakeClock(time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC))
	supervisor := New(testLogger(), processes, fakeClock, "run_sleep_test")

	runtime.GC()
	before := runtime.NumGoroutine()

	if err := supervisor.wakeDue(context.Background()); err != nil {
		t.Fatal(err)
	}

	runtime.Gosched()
	after := runtime.NumGoroutine()

	if processes.woken != sleepers {
		t.Fatalf("woken = %d, want %d", processes.woken, sleepers)
	}
	if delta := after - before; delta > 4 {
		t.Fatalf("goroutine count grew by %d for %d sleepers; sleepers must be non-resident", delta, sleepers)
	}
}

func TestRunUsesOneSharedTickerAndStopsWithContext(t *testing.T) {
	processes := &fakeProcessService{}
	fakeClock := clock.NewFakeClock(time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC))
	supervisor := New(testLogger(), processes, fakeClock, "run_loop_test")
	supervisor.tick = time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- supervisor.Run(ctx)
	}()

	time.Sleep(5 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("supervisor did not stop after context cancellation")
	}

	if processes.dueCalls == 0 {
		t.Fatal("supervisor ticker never performed a shared durable wake scan")
	}
}

func BenchmarkWakeDue10000(b *testing.B) {
	fakeClock := clock.NewFakeClock(time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC))
	logger := testLogger()

	b.ReportAllocs()
	for range b.N {
		b.StopTimer()
		processes := &fakeProcessService{due: makeDueSleeps(10_000)}
		supervisor := New(logger, processes, fakeClock, "run_benchmark")
		b.StartTimer()

		if err := supervisor.wakeDue(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

type fakeProcessService struct {
	running       []agentprocess.State
	runningOffset int
	due           []agentprocess.SleepDue
	dueOffset     int
	recovered     int
	woken         int
	dueCalls      int
	lastRecoverMeta agentprocess.CommandMeta
	lastWakeMeta    agentprocess.CommandMeta
}

func (f *fakeProcessService) Activate(
	context.Context,
	id.AgentID,
	id.RuntimeInstanceID,
	*uint64,
	agentprocess.CommandMeta,
) (agentprocess.State, error) {
	return agentprocess.State{}, nil
}

func (f *fakeProcessService) ListByStatus(
	_ context.Context,
	status agentprocess.Status,
	limit int,
) ([]agentprocess.State, error) {
	if status != agentprocess.StatusRunning || f.runningOffset >= len(f.running) {
		return nil, nil
	}
	end := min(f.runningOffset+limit, len(f.running))
	batch := append([]agentprocess.State(nil), f.running[f.runningOffset:end]...)
	return batch, nil
}

func (f *fakeProcessService) RecoverRunning(
	_ context.Context,
	state agentprocess.State,
	meta agentprocess.CommandMeta,
) (agentprocess.State, error) {
	if f.runningOffset < len(f.running) && f.running[f.runningOffset].AgentID == state.AgentID {
		f.runningOffset++
	}
	f.recovered++
	f.lastRecoverMeta = meta
	state.Status = agentprocess.StatusReady
	state.Version++
	return state, nil
}

func (f *fakeProcessService) DueSleeps(
	_ context.Context,
	_ time.Time,
	limit int,
) ([]agentprocess.SleepDue, error) {
	f.dueCalls++
	if f.dueOffset >= len(f.due) {
		return nil, nil
	}
	end := min(f.dueOffset+limit, len(f.due))
	batch := append([]agentprocess.SleepDue(nil), f.due[f.dueOffset:end]...)
	return batch, nil
}

func (f *fakeProcessService) Wake(
	_ context.Context,
	due agentprocess.SleepDue,
	meta agentprocess.CommandMeta,
) (agentprocess.State, error) {
	if f.dueOffset < len(f.due) && f.due[f.dueOffset].AgentID == due.AgentID {
		f.dueOffset++
	}
	f.woken++
	f.lastWakeMeta = meta
	return agentprocess.State{AgentID: due.AgentID, Status: agentprocess.StatusReady}, nil
}

func makeRunningStates(count int) []agentprocess.State {
	states := make([]agentprocess.State, count)
	for i := range states {
		states[i] = agentprocess.State{
			AgentID: id.AgentID("agt_running_" + integerString(i)),
			Status:  agentprocess.StatusRunning,
			Version: 4,
		}
	}
	return states
}

func makeDueSleeps(count int) []agentprocess.SleepDue {
	due := make([]agentprocess.SleepDue, count)
	wakeAt := time.Date(2026, time.August, 27, 11, 0, 0, 0, time.UTC)
	for i := range due {
		due[i] = agentprocess.SleepDue{
			AgentID:    id.AgentID("agt_sleep_" + integerString(i)),
			SleepID:    id.SleepID("slp_" + integerString(i)),
			Generation: 1,
			WakeAt:     wakeAt,
			Version:    5,
		}
	}
	return due
}

func integerString(value int) string {
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

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
