package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Methamorphe/go-agent/internal/agent"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/workspace"
)

type fakeRuntimeClient struct {
	snapshot workspace.Snapshot
	sent     []agent.SendMessageRequest
	mutated  int
}

func (f *fakeRuntimeClient) Attach(context.Context, workspace.AttachRequest) (workspace.Snapshot, error) {
	return f.snapshot, nil
}
func (f *fakeRuntimeClient) Refresh(context.Context, workspace.RefreshRequest) (workspace.Refresh, error) {
	return workspace.Refresh{Cursor: f.snapshot.Cursor}, nil
}
func (f *fakeRuntimeClient) History(context.Context, workspace.HistoryRequest) (workspace.ConversationViewport, error) {
	return workspace.ConversationViewport{}, nil
}
func (f *fakeRuntimeClient) Search(context.Context, workspace.SearchRequest) (workspace.SearchResult, error) {
	return workspace.SearchResult{}, nil
}
func (f *fakeRuntimeClient) SendMessage(_ context.Context, agentID id.AgentID, text string, queue agent.MessageQueue) (agent.SendMessageResult, error) {
	f.sent = append(f.sent, agent.SendMessageRequest{AgentID: agentID, Text: text, Queue: queue})
	return agent.SendMessageResult{AgentID: agentID, Queue: queue, Pending: len(f.sent)}, nil
}
func (f *fakeRuntimeClient) Suspend(context.Context, id.AgentID, uint64, string) (process.State, error) {
	f.mutated++
	return process.State{}, nil
}
func (f *fakeRuntimeClient) Resume(context.Context, id.AgentID, uint64, string) (process.State, error) {
	f.mutated++
	return process.State{}, nil
}

func TestBoundedBlocksKeeps100KSessionViewportBounded(t *testing.T) {
	blocks := make([]workspace.Block, 100_000)
	for index := range blocks {
		blocks[index] = workspace.Block{ID: fmt.Sprintf("evt_%06d", index), ProcessVersion: uint64(index + 1), Preview: "synthetic history"}
	}
	bounded := boundedBlocks(blocks, MaximumCacheBlocks)
	if len(bounded) != MaximumCacheBlocks {
		t.Fatalf("expected %d cached blocks, got %d", MaximumCacheBlocks, len(bounded))
	}
	if bounded[0].ID != "evt_097952" || bounded[len(bounded)-1].ID != "evt_099999" {
		t.Fatalf("cache did not retain the live tail: first=%s last=%s", bounded[0].ID, bounded[len(bounded)-1].ID)
	}
}

func TestRenderShowsTransactionUncertaintyHonestly(t *testing.T) {
	cfg := DefaultConfig()
	model := NewModel(context.Background(), &fakeRuntimeClient{}, cfg)
	model.width, model.height = 140, 32
	model.loading = false
	model.rootID, model.focusID = "agt_root", "agt_root"
	model.inspectorTab = inspectorTransactions
	model.inspector.Transactions = []workspace.TransactionSummary{{
		ID: "tx_uncertain", AgentID: "agt_root", State: "NEEDS_RECONCILIATION", Version: 7,
		EffectCount: 2, UncertainEffects: 1, ReconcileReason: "promotion outcome could not be proven",
	}}
	output := model.render()
	if !strings.Contains(output, "NEEDS_RECONCILIATION") || !strings.Contains(output, "outcome uncertainty preserved") {
		t.Fatalf("uncertain transaction must be explicit in TUI:\n%s", output)
	}
	if strings.Contains(output, "rollback successful") || strings.Contains(output, "commit successful") {
		t.Fatal("uncertain transaction must never be rendered as a false success")
	}
}

func TestPalettePlanAndReviewFeedbackUseAgentMessage(t *testing.T) {
	client := &fakeRuntimeClient{}
	model := NewModel(context.Background(), client, DefaultConfig())
	model.focusID = "agt_focus"
	model.loading = false

	updated, cmd := model.executePalette("plan approve step 2 after checking tests")
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("plan feedback should create a runtime command")
	}
	message := cmd()
	if _, ok := message.(sendMsg); !ok {
		t.Fatalf("plan feedback command returned %T", message)
	}
	if len(client.sent) != 1 || client.sent[0].Queue != agent.QueueSteer || !strings.Contains(client.sent[0].Text, "Plan review:") {
		t.Fatalf("unexpected plan feedback: %#v", client.sent)
	}

	model.loading = false
	updated, cmd = model.executePalette("review hunk 3 needs a race regression test")
	_ = updated
	if cmd == nil {
		t.Fatal("review feedback should create a runtime command")
	}
	_ = cmd()
	if len(client.sent) != 2 || !strings.Contains(client.sent[1].Text, "Review feedback:") {
		t.Fatalf("unexpected review feedback: %#v", client.sent)
	}
}

func TestDetachNeverMutatesRuntime(t *testing.T) {
	client := &fakeRuntimeClient{}
	model := NewModel(context.Background(), client, DefaultConfig())
	updated, cmd := model.executePalette("detach")
	_ = updated
	if cmd == nil {
		t.Fatal("detach should quit the client")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("detach should yield tea.QuitMsg, got %T", msg)
	}
	if client.mutated != 0 || len(client.sent) != 0 {
		t.Fatal("detaching the TUI must not mutate or cancel runtime work")
	}
}

func TestPrependHistoryKeepsRequestedOldWindowWithinBoundedCache(t *testing.T) {
	model := NewModel(context.Background(), &fakeRuntimeClient{}, DefaultConfig())
	model.cfg.MaxCachedBlocks = 5
	model.blocks = []workspace.Block{{ID: "6"}, {ID: "7"}, {ID: "8"}, {ID: "9"}, {ID: "10"}}
	model.prependBlocks([]workspace.Block{{ID: "3"}, {ID: "4"}, {ID: "5"}})
	if len(model.blocks) != 5 {
		t.Fatalf("cache must remain bounded, got %d", len(model.blocks))
	}
	want := []string{"3", "4", "5", "6", "7"}
	for index, id := range want {
		if model.blocks[index].ID != id {
			t.Fatalf("at %d expected %s got %s", index, id, model.blocks[index].ID)
		}
	}
}

func TestAdaptiveRendererPrioritizesConversationOnNarrowTerminal(t *testing.T) {
	model := NewModel(context.Background(), &fakeRuntimeClient{}, DefaultConfig())
	model.width, model.height = 70, 22
	model.loading = false
	model.rootID, model.focusID = "agt_root", "agt_root"
	model.blocks = []workspace.Block{{ID: "evt_1", Kind: workspace.BlockAssistantMessage, Preview: "visible answer", OccurredAt: time.Now()}}
	output := model.render()
	if !strings.Contains(output, "CONVERSATION / WORK") || !strings.Contains(output, "visible answer") {
		t.Fatalf("narrow renderer lost primary conversation surface:\n%s", output)
	}
	if strings.Contains(output, "INSPECTOR ·") {
		t.Fatal("narrow layout should collapse inspector rather than squeeze conversation")
	}
}
