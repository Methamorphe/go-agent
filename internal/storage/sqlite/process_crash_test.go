package sqlite

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/id"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
)

type processCrashFixture struct {
	AgentID       id.AgentID       `json:"agent_id"`
	RootAgentID   id.AgentID       `json:"root_agent_id"`
	ParentAgentID id.AgentID       `json:"parent_agent_id"`
	Version       uint64           `json:"version"`
	Status        agentprocess.Status `json:"status"`
	LineageDepth  uint32           `json:"lineage_depth"`
	LastEventID   id.EventID       `json:"last_event_id"`
	EventCount    int              `json:"event_count"`
}

func TestProcessReconstructsExactlyAfterHardKill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	cmd := exec.Command(os.Args[0], "-test.run=TestProcessCrashHelper")
	cmd.Env = append(
		os.Environ(),
		"GO_AGENT_PROCESS_CRASH_HELPER=1",
		"GO_AGENT_PROCESS_CRASH_DB="+path,
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		_ = cmd.Process.Kill()
		t.Fatalf("crash helper produced no fixture")
	}

	var fixture processCrashFixture
	if err := json.Unmarshal(scanner.Bytes(), &fixture); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("decode crash fixture %q: %v", scanner.Text(), err)
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()

	ctx := context.Background()
	store, err := Open(ctx, Config{Path: path})
	if err != nil {
		t.Fatalf("reopen after hard kill: %v", err)
	}
	defer store.Close()

	service := agentprocess.NewService(store, id.NewGenerator(), clock.NewSystemClock())
	current, err := service.Inspect(ctx, fixture.AgentID)
	if err != nil {
		t.Fatalf("inspect after hard kill: %v", err)
	}
	reconstructed, err := service.Reconstruct(ctx, fixture.AgentID)
	if err != nil {
		t.Fatalf("reconstruct after hard kill: %v", err)
	}

	if !reflect.DeepEqual(reconstructed, current) {
		t.Fatalf("reconstructed state differs from persisted projection\nreconstructed: %#v\ncurrent: %#v", reconstructed, current)
	}
	if current.AgentID != fixture.AgentID ||
		current.RootAgentID != fixture.RootAgentID ||
		current.ParentAgentID == nil || *current.ParentAgentID != fixture.ParentAgentID ||
		current.Version != fixture.Version ||
		current.Status != fixture.Status ||
		current.LineageDepth != fixture.LineageDepth ||
		current.LastEventID != fixture.LastEventID {
		t.Fatalf("state after hard kill does not match committed fixture: %#v", current)
	}

	events, err := store.Events(ctx, fixture.AgentID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != fixture.EventCount {
		t.Fatalf("event count after hard kill = %d, want %d", len(events), fixture.EventCount)
	}

	snapshot, ok, err := store.LatestSnapshot(ctx, fixture.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("snapshot disappeared after hard kill")
	}
	if snapshot.ThroughProcessVersion >= current.Version {
		t.Fatalf("test fixture has no snapshot tail: snapshot=%d current=%d", snapshot.ThroughProcessVersion, current.Version)
	}
}

func TestProcessCrashHelper(t *testing.T) {
	if os.Getenv("GO_AGENT_PROCESS_CRASH_HELPER") != "1" {
		return
	}

	ctx := context.Background()
	fakeClock := clock.NewFakeClock(time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC))
	store, err := Open(ctx, Config{
		Path:  os.Getenv("GO_AGENT_PROCESS_CRASH_DB"),
		Clock: fakeClock,
	})
	if err != nil {
		os.Exit(20)
	}

	service := agentprocess.NewService(store, id.NewGenerator(), fakeClock)
	root, err := service.CreateRoot(ctx, "hard kill root", commandMeta("req_crash_root"))
	if err != nil {
		os.Exit(21)
	}
	child, err := service.CreateChild(ctx, root.AgentID, commandMeta("req_crash_child"))
	if err != nil {
		os.Exit(22)
	}
	child, err = service.Activate(
		ctx,
		child.AgentID,
		"run_crash_helper",
		versionPtr(child.Version),
		commandMeta("req_crash_activate"),
	)
	if err != nil {
		os.Exit(23)
	}
	child, err = service.Yield(
		ctx,
		child.AgentID,
		versionPtr(child.Version),
		"before snapshot",
		commandMeta("req_crash_yield"),
	)
	if err != nil {
		os.Exit(24)
	}
	if _, err := service.Snapshot(ctx, child.AgentID); err != nil {
		os.Exit(25)
	}
	child, err = service.Suspend(
		ctx,
		child.AgentID,
		versionPtr(child.Version),
		"after snapshot",
		commandMeta("req_crash_suspend"),
	)
	if err != nil {
		os.Exit(26)
	}
	child, err = service.Resume(
		ctx,
		child.AgentID,
		versionPtr(child.Version),
		"committed before kill",
		commandMeta("req_crash_resume"),
	)
	if err != nil {
		os.Exit(27)
	}

	events, err := store.Events(ctx, child.AgentID, 0, 100)
	if err != nil {
		os.Exit(28)
	}
	if child.ParentAgentID == nil {
		os.Exit(29)
	}

	fixture := processCrashFixture{
		AgentID:       child.AgentID,
		RootAgentID:   child.RootAgentID,
		ParentAgentID: *child.ParentAgentID,
		Version:       child.Version,
		Status:        child.Status,
		LineageDepth:  child.LineageDepth,
		LastEventID:   child.LastEventID,
		EventCount:    len(events),
	}
	if err := json.NewEncoder(os.Stdout).Encode(fixture); err != nil {
		os.Exit(30)
	}

	select {}
}
