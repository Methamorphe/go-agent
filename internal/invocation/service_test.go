package invocation

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/objectstore"
	"github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/provider"
)

type testIDs struct{}

func (testIDs) Invocation() (id.InvocationID, error) { return "inv_test", nil }

type memoryObjects struct {
	items map[objectstore.Ref]string
	next  int
}

func (s *memoryObjects) Put(_ context.Context, reader io.Reader) (objectstore.Meta, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		return objectstore.Meta{}, err
	}
	s.next++
	ref := objectstore.Ref(fmt.Sprintf("sha256:test%d", s.next))
	if s.items == nil {
		s.items = map[objectstore.Ref]string{}
	}
	s.items[ref] = string(body)
	return objectstore.Meta{Ref: ref}, nil
}

type recorder struct{ started, completed, failed int }

func (r *recorder) ModelInvocationStarted(_ context.Context, agentID id.AgentID, _ *uint64, _ process.ModelInvocationStartedPayload, _ process.CommandMeta) (process.State, error) {
	r.started++
	return process.State{AgentID: agentID, Version: 11, Status: process.StatusRunning}, nil
}
func (r *recorder) ModelInvocationCompleted(_ context.Context, agentID id.AgentID, _ *uint64, _ process.ModelInvocationCompletedPayload, _ process.CommandMeta) (process.State, error) {
	r.completed++
	return process.State{AgentID: agentID, Version: 12, Status: process.StatusRunning}, nil
}
func (r *recorder) ModelInvocationFailed(_ context.Context, agentID id.AgentID, _ *uint64, _ process.ModelInvocationFailedPayload, _ process.CommandMeta) (process.State, error) {
	r.failed++
	return process.State{AgentID: agentID, Version: 12, Status: process.StatusRunning}, nil
}

func TestManyModelChunksProduceOnlyInvocationLifecycleRecords(t *testing.T) {
	events := make([]provider.Event, 100)
	for i := range events {
		events[i] = provider.Event{Kind: provider.EventTextDelta, Text: "x"}
	}
	model := provider.NewFake(provider.FakeScript{
		Events: events,
		Result: provider.Result{FinishReason: provider.FinishStop},
	})
	recorder := &recorder{}
	objects := &memoryObjects{}
	service := New(recorder, objects, testIDs{}, nil)

	version := uint64(10)
	outcome, _, err := service.Invoke(
		context.Background(),
		model,
		id.AgentID("agt_test"),
		"model",
		[]provider.Message{{Role: provider.RoleUser, Parts: []provider.ContentPart{provider.TextPart("hello")}}},
		nil,
		&version,
		process.CommandMeta{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if recorder.started != 1 || recorder.completed != 1 || recorder.failed != 0 {
		t.Fatalf("lifecycle counts = started %d completed %d failed %d", recorder.started, recorder.completed, recorder.failed)
	}
	if len(outcome.TextPreview) != 100 || strings.Trim(outcome.TextPreview, "x") != "" {
		t.Fatalf("preview length = %d", len(outcome.TextPreview))
	}
	if objects.items[objectstore.Ref(outcome.ResponseRef)] != strings.Repeat("x", 100) {
		t.Fatal("full response was not persisted as one object")
	}
}
