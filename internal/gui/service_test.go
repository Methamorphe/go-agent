package gui

import (
	"context"
	"testing"

	"github.com/Methamorphe/go-agent/internal/agent"
	controlapi "github.com/Methamorphe/go-agent/internal/control/api"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/live"
	"github.com/Methamorphe/go-agent/internal/workspace"
)

type fakeCaller struct {
	call func(context.Context, string, any, any) error
}

func (f fakeCaller) Call(ctx context.Context, messageType string, request, response any) error {
	return f.call(ctx, messageType, request, response)
}

func TestBootstrapIsBoundedAndDefaultsToAct(t *testing.T) {
	service := NewService(nil, LaunchOptions{})
	bootstrap := service.Bootstrap()
	if bootstrap.BridgeVersion != BridgeVersion || bootstrap.WorkspaceProtocolVersion != workspace.ProtocolVersion {
		t.Fatalf("unexpected protocol versions: %#v", bootstrap)
	}
	if bootstrap.Launch.Mode != workspace.ModeAct {
		t.Fatalf("expected ACT launch mode, got %q", bootstrap.Launch.Mode)
	}
	if bootstrap.MaxHistoryPage != workspace.MaxPageSize || bootstrap.MaxTree != workspace.MaxTreeLimit {
		t.Fatalf("unexpected projection limits: %#v", bootstrap)
	}
}

func TestAttachUsesLaunchDefaults(t *testing.T) {
	const wantedAgent id.AgentID = "agt_test"
	caller := fakeCaller{call: func(_ context.Context, messageType string, request, response any) error {
		if messageType != controlapi.MessageWorkspaceAttach {
			t.Fatalf("unexpected message type %q", messageType)
		}
		got := request.(workspace.AttachRequest)
		if got.AgentID != wantedAgent || got.Mode != workspace.ModeReview {
			t.Fatalf("unexpected attach request: %#v", got)
		}
		response.(*controlapi.WorkspaceAttachResponse).Snapshot = workspace.Snapshot{FocusedAgentID: wantedAgent, Mode: workspace.ModeReview}
		return nil
	}}
	service := NewService(caller, LaunchOptions{AgentID: wantedAgent, Mode: workspace.ModeReview})
	snapshot, err := service.Attach(workspace.AttachRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.FocusedAgentID != wantedAgent || snapshot.Mode != workspace.ModeReview {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}

func TestLiveUsesBoundedDaemonSnapshot(t *testing.T) {
	caller := fakeCaller{call: func(_ context.Context, messageType string, request, response any) error {
		if messageType != controlapi.MessageAgentLive {
			t.Fatalf("unexpected message type %q", messageType)
		}
		got := request.(controlapi.AgentLiveRequest)
		response.(*controlapi.AgentLiveResponse).Stream = live.Snapshot{AgentID: got.AgentID, Status: live.StatusRunning, Data: "stream"}
		return nil
	}}
	service := NewService(caller, LaunchOptions{})
	stream, err := service.Live("agt_live")
	if err != nil {
		t.Fatal(err)
	}
	if stream.Data != "stream" || stream.Status != live.StatusRunning {
		t.Fatalf("unexpected live stream: %#v", stream)
	}
}

func TestSendMessagePreservesQueueSemantics(t *testing.T) {
	caller := fakeCaller{call: func(_ context.Context, messageType string, request, response any) error {
		if messageType != controlapi.MessageAgentMessage {
			t.Fatalf("unexpected message type %q", messageType)
		}
		got := request.(agent.SendMessageRequest)
		if got.Queue != agent.QueueFollowUp || got.Text != "after this" {
			t.Fatalf("unexpected message request: %#v", got)
		}
		response.(*controlapi.AgentMessageResponse).Result = agent.SendMessageResult{AgentID: got.AgentID, Queue: got.Queue, Pending: 1}
		return nil
	}}
	service := NewService(caller, LaunchOptions{})
	result, err := service.SendMessage(agent.SendMessageRequest{AgentID: "agt_test", Text: "after this", Queue: agent.QueueFollowUp})
	if err != nil {
		t.Fatal(err)
	}
	if result.Pending != 1 || result.Queue != agent.QueueFollowUp {
		t.Fatalf("unexpected result: %#v", result)
	}
}
