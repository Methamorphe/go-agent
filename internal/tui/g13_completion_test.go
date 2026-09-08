package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/workspace"
)

func TestForkInspectorVisuallyComparesCandidatesAndWinner(t *testing.T) {
	model := NewModel(context.Background(), &fakeRuntimeClient{}, DefaultConfig())
	model.width, model.height = 220, 36
	model.loading = false
	model.rootID, model.focusID = "agt_root", "agt_root"
	model.inspectorTab = inspectorForks
	model.inspector.Forks = []workspace.ForkSummary{{
		GroupID: id.ForkGroupID("fkg_compare"),
		State: "EVALUATED",
		WinnerForkID: id.ForkID("frk_winner"),
		SelectionReason: "winner has the best eligible objective score",
		Branches: []workspace.ForkBranchSummary{
			{ForkID: id.ForkID("frk_winner"), State: "SELECTED", SpentMoneyMicros: 12000, SpentTokens: 1400, Evaluation: `{"score":0.94,"tests":"pass"}`},
			{ForkID: id.ForkID("frk_other"), State: "DISCARDED", SpentMoneyMicros: 9000, SpentTokens: 1100, Evaluation: `{"score":0.81,"tests":"pass"}`},
		},
	}}
	output := model.render()
	for _, want := range []string{"INSPECTOR · FORKS", "frk_winner", "frk_other", "SELECTED", "DISCARDED", "winner has the best eligible"} {
		if !strings.Contains(output, want) {
			t.Fatalf("fork comparison missing %q:\n%s", want, output)
		}
	}
}

func TestUnicodeAndResizeRemainRenderableAcrossTerminalWidths(t *testing.T) {
	model := NewModel(context.Background(), &fakeRuntimeClient{}, DefaultConfig())
	model.loading = false
	model.rootID, model.focusID = "agt_root", "agt_root"
	model.tree = []workspace.ProcessSummary{{AgentID: "agt_root", RootAgentID: "agt_root", Goal: "Réviser 日本語 🚀 sans casser le terminal"}}
	model.blocks = []workspace.Block{{ID: "evt_unicode", Kind: workspace.BlockAssistantMessage, Preview: "Résultat ✓ — 日本語 — emoji 🚀 — texte durable"}}
	for _, size := range []struct{ width, height int }{{60, 20}, {90, 26}, {160, 40}} {
		model.width, model.height = size.width, size.height
		output := model.render()
		if output == "" || !strings.Contains(output, "CONVERSATION / WORK") {
			t.Fatalf("invalid render at %dx%d", size.width, size.height)
		}
	}
}

func TestRuntimeInspectorsExposeMMUSchedulerAndAuthorityWithoutHiddenReasoning(t *testing.T) {
	model := NewModel(context.Background(), &fakeRuntimeClient{}, DefaultConfig())
	model.width, model.height = 220, 32
	model.loading = false
	model.rootID, model.focusID = "agt_root", "agt_root"
	model.inspector.Context = workspace.ContextRuntimeSummary{PageCount: 17, EstimatedTokens: 4200, ActiveLeaseCount: 2, UnresolvedFaults: 1, LatestManifestRef: "object://manifest"}
	model.inspector.Scheduler = workspace.SchedulerSummary{LimitMoneyMicros: 2000000, SpentMoneyMicros: 500000, LimitTokens: 100000, SpentTokens: 12000, LastDecisionID: "route_123", LastProvider: "openai", LastModel: "model-x", LastProfileVersion: 3}
	model.inspector.Authority = workspace.AuthoritySummary{IntentID: id.IntentID("int_root"), IntentVersion: 1, Goal: "ship safely", CapabilityProjection: "runtime authority remains canonical below the TUI"}

	model.inspectorTab = inspectorContext
	if output := model.render(); !strings.Contains(output, "pages") || !strings.Contains(output, "4200") || !strings.Contains(output, "manifest") {
		t.Fatalf("context inspector incomplete:\n%s", output)
	}
	model.inspectorTab = inspectorScheduler
	if output := model.render(); !strings.Contains(output, "model-x") || !strings.Contains(output, "route_123") {
		t.Fatalf("scheduler inspector incomplete:\n%s", output)
	}
	model.inspectorTab = inspectorAuthority
	if output := model.render(); !strings.Contains(output, "ship safely") || !strings.Contains(output, "UI approval never grants runtime") || !strings.Contains(output, "capability") {
		t.Fatalf("authority inspector incomplete:\n%s", output)
	}
}
