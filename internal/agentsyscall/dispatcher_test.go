package agentsyscall

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/objectstore"
	"github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/provider"
)

type syscallIDs struct{ next int }

func (g *syscallIDs) Action() (id.ActionID, error) {
	g.next++
	return id.ActionID(fmt.Sprintf("act_%d", g.next)), nil
}

type syscallProcesses struct{}

func (syscallProcesses) SyscallRequested(_ context.Context, agentID id.AgentID, expected *uint64, _ process.SyscallRequestedPayload, _ process.CommandMeta) (process.State, error) {
	return process.State{AgentID: agentID, Version: *expected + 1, Status: process.StatusRunning}, nil
}
func (syscallProcesses) SyscallCompleted(_ context.Context, agentID id.AgentID, expected *uint64, _ process.SyscallCompletedPayload, _ process.CommandMeta) (process.State, error) {
	return process.State{AgentID: agentID, Version: *expected + 1, Status: process.StatusRunning}, nil
}
func (syscallProcesses) SyscallFailed(_ context.Context, agentID id.AgentID, expected *uint64, _ process.SyscallFailedPayload, _ process.CommandMeta) (process.State, error) {
	return process.State{AgentID: agentID, Version: *expected + 1, Status: process.StatusRunning}, nil
}
func (syscallProcesses) Checkpoint(_ context.Context, agentID id.AgentID, expected *uint64, _ process.CommandMeta) (process.State, error) {
	return process.State{AgentID: agentID, Version: *expected + 1, Status: process.StatusRunning, LastCheckpointID: id.CheckpointID("chk_test")}, nil
}

func TestObserveReadAndTraversalDenial(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello world"), 0o600); err != nil {
		t.Fatal(err)
	}
	objects, err := objectstore.New(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := New(syscallProcesses{}, objects, &syscallIDs{}, root)
	if err != nil {
		t.Fatal(err)
	}

	state := process.State{AgentID: id.AgentID("agt_test"), Version: 10, Status: process.StatusRunning}
	call := provider.ToolCall{ID: "call_1", Name: "observe", Arguments: json.RawMessage(`{"operation":"read_file","path":"hello.txt"}`)}
	result, next, err := dispatcher.Call(context.Background(), state, call, process.CommandMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Preview != "hello world" || result.ContentRef == "" {
		t.Fatalf("result = %+v", result)
	}
	if next.Version != 12 {
		t.Fatalf("version = %d, want 12", next.Version)
	}

	call = provider.ToolCall{ID: "call_2", Name: "observe", Arguments: json.RawMessage(`{"operation":"list_directory","path":".."}`)}
	result, _, err = dispatcher.Call(context.Background(), next, call, process.CommandMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || !strings.Contains(result.Error, "workspace") {
		t.Fatalf("escape result = %+v", result)
	}
}

func TestExecuteCapturesBoundedPreview(t *testing.T) {
	if mode := os.Getenv("GO_AGENT_TEST_HELPER"); mode != "" {
		size := DefaultPreviewBytes * 2
		if mode == "limit" {
			size = 2 << 20
		}
		fmt.Print(strings.Repeat("x", size))
		return
	}

	root := t.TempDir()
	objects, err := objectstore.New(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := New(syscallProcesses{}, objects, &syscallIDs{}, root)
	if err != nil {
		t.Fatal(err)
	}

	old := os.Getenv("GO_AGENT_TEST_HELPER")
	if err := os.Setenv("GO_AGENT_TEST_HELPER", "preview"); err != nil {
		t.Fatal(err)
	}
	defer os.Setenv("GO_AGENT_TEST_HELPER", old)

	arguments, _ := json.Marshal(map[string]any{
		"executable": os.Args[0],
		"args":       []string{"-test.run=TestExecuteCapturesBoundedPreview"},
		"cwd":        root,
		"timeout_ms": 10000,
	})
	state := process.State{AgentID: id.AgentID("agt_test"), Version: 1, Status: process.StatusRunning}
	result, _, err := dispatcher.Call(context.Background(), state, provider.ToolCall{ID: "call_1", Name: "execute", Arguments: arguments}, process.CommandMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode == nil || *result.ExitCode != 0 || result.StdoutRef == "" {
		t.Fatalf("result = %+v", result)
	}
	if !result.Truncated {
		t.Fatal("expected bounded preview to be truncated")
	}
}

func TestExecuteHardOutputLimit(t *testing.T) {
	if os.Getenv("GO_AGENT_TEST_HELPER") != "" {
		return
	}

	root := t.TempDir()
	objects, err := objectstore.New(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := New(syscallProcesses{}, objects, &syscallIDs{}, root)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.maxOutput = 64 << 10

	old := os.Getenv("GO_AGENT_TEST_HELPER")
	if err := os.Setenv("GO_AGENT_TEST_HELPER", "limit"); err != nil {
		t.Fatal(err)
	}
	defer os.Setenv("GO_AGENT_TEST_HELPER", old)

	arguments, _ := json.Marshal(map[string]any{
		"executable": os.Args[0],
		"args":       []string{"-test.run=TestExecuteCapturesBoundedPreview"},
		"cwd":        root,
		"timeout_ms": 10000,
	})
	state := process.State{AgentID: id.AgentID("agt_test"), Version: 1, Status: process.StatusRunning}
	result, _, err := dispatcher.Call(context.Background(), state, provider.ToolCall{ID: "call_limit", Name: "execute", Arguments: arguments}, process.CommandMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || !strings.Contains(result.Error, "output exceeded") {
		t.Fatalf("result = %+v", result)
	}
}

func TestCheckpointUsesExistingProcessCheckpoint(t *testing.T) {
	root := t.TempDir()
	objects, err := objectstore.New(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := New(syscallProcesses{}, objects, &syscallIDs{}, root)
	if err != nil {
		t.Fatal(err)
	}
	state := process.State{AgentID: id.AgentID("agt_test"), Version: 3, Status: process.StatusRunning}
	result, next, err := dispatcher.Call(context.Background(), state, provider.ToolCall{ID: "call_1", Name: "checkpoint", Arguments: json.RawMessage(`{}`)}, process.CommandMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Checkpoint != "chk_test" || next.Version != 6 {
		t.Fatalf("result=%+v next=%+v", result, next)
	}
}
