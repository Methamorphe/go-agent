package fork

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/scheduler"
	agenttx "github.com/Methamorphe/go-agent/internal/transaction"
	"github.com/Methamorphe/go-agent/internal/world"
)

type g8ProcessStore struct {
	state    agentprocess.State
	sequence uint64
}
func (s *g8ProcessStore) Current(context.Context, id.AgentID) (agentprocess.State, error) { return s.state, nil }
func (s *g8ProcessStore) Append(context.Context, uint64, []ledger.Event, agentprocess.State, agentprocess.Receipt) error { return errors.New("unused") }
func (s *g8ProcessStore) Events(context.Context, id.AgentID, uint64, int) ([]ledger.Event, error) { return nil, nil }
func (s *g8ProcessStore) Receipt(context.Context, id.RequestID) (agentprocess.Receipt, bool, error) { return agentprocess.Receipt{}, false, nil }
func (s *g8ProcessStore) SaveSnapshot(context.Context, agentprocess.Snapshot) error { return nil }
func (s *g8ProcessStore) LatestSnapshot(context.Context, id.AgentID) (agentprocess.Snapshot, bool, error) { return agentprocess.Snapshot{}, false, nil }
func (s *g8ProcessStore) LedgerSequenceAtVersion(context.Context, id.AgentID, uint64) (uint64, error) { return s.sequence, nil }
func (s *g8ProcessStore) ListByStatus(context.Context, agentprocess.Status, int) ([]agentprocess.State, error) { return nil, nil }
func (s *g8ProcessStore) DueSleeps(context.Context, time.Time, int) ([]agentprocess.SleepDue, error) { return nil, nil }

type g8Runtime struct {
	frontier agentprocess.ExecutionFrontier
	forks    int
}
func (r *g8Runtime) ExecutionFrontier(context.Context, id.AgentID) (agentprocess.ExecutionFrontier, error) { return r.frontier, nil }
func (r *g8Runtime) ForkTimelineFromState(_ context.Context, source agentprocess.State, _ agentprocess.CommandMeta) (agentprocess.State, error) {
	r.forks++
	forked := source
	forked.AgentID = id.AgentID(fmt.Sprintf("agt_g8_fork_%d", r.forks))
	parent := source.AgentID
	forked.ParentAgentID = &parent
	forked.LineageDepth = source.LineageDepth + 1
	forked.Status = agentprocess.StatusReady
	forked.Lease, forked.Wait, forked.Sleep = nil, nil, nil
	return forked, nil
}

type g8Authority struct {
	intent world.Intent
	grants []world.Capability
}
func (a *g8Authority) CurrentAuthority(context.Context, id.AgentID) (world.Intent, []world.Capability, error) {
	return a.intent, append([]world.Capability(nil), a.grants...), nil
}

type g8Harness struct {
	ctx       context.Context
	manager   *Manager
	store     *MemoryStore
	budgets   *scheduler.BudgetLedger
	authority *g8Authority
	runtime   *g8Runtime
	processes *g8ProcessStore
	tx        *agenttx.Manager
	snapshot  *world.WorkspaceWorld
	repo      string
	now       time.Time
}

func newG8Harness(t *testing.T) *g8Harness {
	t.Helper()
	ctx := context.Background()
	repo := initG8GitRepo(t, map[string]string{"candidate.txt": "base\n"})
	snapshot, err := world.NewWorkspaceWorld(ctx, world.WorkspaceConfig{Repository: repo, WorldID: id.WorldID("wld_g8_checkpoint")})
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = snapshot.Rollback(ctx) })
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	root := id.AgentID("agt_g8_root")
	state := agentprocess.State{
		AgentID: root, RootAgentID: root, Version: 3, Status: agentprocess.StatusReady,
		LastEventID: id.EventID("evt_g8_checkpoint"), UpdatedAt: now,
		RootIntent: &agentprocess.Intent{ID: id.IntentID("int_g8"), SchemaVersion: agentprocess.IntentSchemaVersion, Goal: "pick fastest correct implementation", CreatedAt: now},
	}
	processes := &g8ProcessStore{state: state, sequence: 42}
	runtime := &g8Runtime{frontier: agentprocess.ExecutionFrontier{AgentID: root, ProcessVersion: state.Version, LedgerSequence: 42, CompletedResultRefs: []string{"obj://required-result"}}}
	budgets := scheduler.NewBudgetLedger()
	if err := budgets.SetLimit(root, scheduler.Resources{MoneyMicros: 10_000, Tokens: 10_000}); err != nil { t.Fatal(err) }
	authority := &g8Authority{
		intent: world.Intent{Version: 7, Goal: "pick fastest correct implementation", AllowedDomains: []string{"fs.read", "fs.write", "process.exec"}, ForbiddenDomains: []string{"network"}},
		grants: []world.Capability{{Domain: "fs.read", Scope: "."}, {Domain: "fs.write", Scope: "."}, {Domain: "process.exec", Scope: "."}},
	}
	tx := agenttx.NewManager(agenttx.NewMemoryStore(), id.NewGenerator(), func() time.Time { return now })
	store := NewMemoryStore()
	manager := NewManager(store, processes, runtime, tx, budgets, authority, id.NewGenerator(), func() time.Time { return now })
	return &g8Harness{ctx: ctx, manager: manager, store: store, budgets: budgets, authority: authority, runtime: runtime, processes: processes, tx: tx, snapshot: snapshot, repo: repo, now: now}
}

func (h *g8Harness) checkpoint(t *testing.T, retainUntil *time.Time) Checkpoint {
	t.Helper()
	cp, err := h.manager.CreateCheckpoint(h.ctx, CheckpointRequest{SourceAgentID: h.processes.state.AgentID, WorldSnapshot: h.snapshot.BranchRef(), ContextRefs: []id.ContextPageID{"ctx_base"}, RequiredResults: []ResultDependency{{Ref: "obj://required-result", RequiredBy: "decision://benchmark"}}, RetainUntil: retainUntil})
	if err != nil { t.Fatal(err) }
	return cp
}

func TestG8CheckpointRejectsUnsupportedWorldInflightAndMissingDependency(t *testing.T) {
	h := newG8Harness(t)
	badWorld := h.snapshot.BranchRef(); badWorld.Type = world.TypeLocal
	_, err := h.manager.CreateCheckpoint(h.ctx, CheckpointRequest{SourceAgentID: h.processes.state.AgentID, WorldSnapshot: badWorld})
	if !errors.Is(err, ErrUnsafeCheckpoint) { t.Fatalf("unsupported world err=%v", err) }

	h.runtime.frontier.InFlightActions = []id.ActionID{"act_pending"}
	_, err = h.manager.CreateCheckpoint(h.ctx, CheckpointRequest{SourceAgentID: h.processes.state.AgentID, WorldSnapshot: h.snapshot.BranchRef()})
	if !errors.Is(err, ErrUnsafeCheckpoint) { t.Fatalf("in-flight err=%v", err) }
	h.runtime.frontier.InFlightActions = nil

	_, err = h.manager.CreateCheckpoint(h.ctx, CheckpointRequest{SourceAgentID: h.processes.state.AgentID, WorldSnapshot: h.snapshot.BranchRef(), RequiredResults: []ResultDependency{{Ref: "obj://missing"}}})
	if !errors.Is(err, ErrUnsafeCheckpoint) { t.Fatalf("missing dependency err=%v", err) }
}

func TestG8ForkReservesBudgetAndRevalidatesCurrentAuthority(t *testing.T) {
	h := newG8Harness(t)
	cp := h.checkpoint(t, nil)
	group, err := h.manager.CreateGroup(h.ctx, cp.ID)
	if err != nil { t.Fatal(err) }

	// Revoke writes after checkpoint. Historical grants must not revive them.
	h.authority.intent.Version = 8
	h.authority.grants = []world.Capability{{Domain: "fs.read", Scope: "."}}
	branch, branchWorld, err := h.manager.ForkWorkspace(h.ctx, ForkRequest{GroupID: group.ID, BudgetLimit: scheduler.Resources{MoneyMicros: 1000, Tokens: 500}})
	if err != nil { t.Fatal(err) }
	if branch.BudgetReservation == "" { t.Fatal("fork created without reservation") }
	budget := h.budgets.Snapshot(cp.RootAgentID)
	if budget.Reserved.MoneyMicros != 1000 || budget.Reserved.Tokens != 500 { t.Fatalf("reserved=%+v", budget.Reserved) }
	params, _ := json.Marshal(map[string]string{"content": "forbidden\n"})
	result, err := branchWorld.Execute(h.ctx, world.Action{ID: NamespaceAction(branch.ID, id.ActionID("act_write")), AgentID: branch.AgentID, Kind: "fs.write_file", Purpose: "try historical write", Resource: "candidate.txt", Params: params, Effect: world.CanonicalEffect("fs.write_file")})
	if err == nil || result.Status != world.ResultDenied { t.Fatalf("revoked write result=%+v err=%v", result, err) }
	if got := readG8File(t, h.repo, "candidate.txt"); got != "base\n" { t.Fatalf("revoked write leaked: %q", got) }
	if err := h.manager.CleanupGroup(h.ctx, group.ID); err != nil { t.Fatal(err) }
	if h.budgets.Snapshot(cp.RootAgentID).Reserved != (scheduler.Resources{}) { t.Fatalf("reservation not released: %+v", h.budgets.Snapshot(cp.RootAgentID)) }
}

func TestG8KillerDemoTwoSolutionsBenchmarkPromoteOnlyWinner(t *testing.T) {
	h := newG8Harness(t)
	retainUntil := h.now.Add(time.Hour)
	cp := h.checkpoint(t, &retainUntil)
	group, err := h.manager.CreateGroup(h.ctx, cp.ID)
	if err != nil { t.Fatal(err) }
	branchA, worldA, err := h.manager.ForkWorkspace(h.ctx, ForkRequest{GroupID: group.ID, BudgetLimit: scheduler.Resources{MoneyMicros: 1200, Tokens: 2000}})
	if err != nil { t.Fatal(err) }
	branchB, worldB, err := h.manager.ForkWorkspace(h.ctx, ForkRequest{GroupID: group.ID, BudgetLimit: scheduler.Resources{MoneyMicros: 1200, Tokens: 2000}})
	if err != nil { t.Fatal(err) }
	if branchA.AgentID == branchB.AgentID || branchA.WorldID == branchB.WorldID || branchA.BudgetReservation == branchB.BudgetReservation { t.Fatal("fork identities/reservations collided") }

	writeFork := func(branch Branch, w world.TransactionalWorld, body string) {
		params, _ := json.Marshal(map[string]string{"content": body})
		result, err := w.Execute(h.ctx, world.Action{ID: NamespaceAction(branch.ID, id.ActionID("act_candidate_write")), AgentID: branch.AgentID, Kind: "fs.write_file", Purpose: "implement candidate", Resource: "candidate.txt", Params: params, Effect: world.CanonicalEffect("fs.write_file")})
		if err != nil || result.Status != world.ResultSucceeded { t.Fatalf("write %s result=%+v err=%v", branch.ID, result, err) }
	}
	writeFork(branchA, worldA, "slow\n")
	writeFork(branchB, worldB, "fast\n")
	if got := readG8File(t, h.repo, "candidate.txt"); got != "base\n" { t.Fatalf("speculation leaked before promotion: %q", got) }

	if _, err := h.manager.Spend(h.ctx, branchA.ID, scheduler.Resources{MoneyMicros: 200, Tokens: 900}); err != nil { t.Fatal(err) }
	if _, err := h.manager.Spend(h.ctx, branchB.ID, scheduler.Resources{MoneyMicros: 250, Tokens: 1000}); err != nil { t.Fatal(err) }
	if _, err := h.manager.PutOverlay(h.ctx, branchA.ID, OverlayEntry{Kind: OverlayDecision, Key: "candidate-a", Value: json.RawMessage(`{"choice":"slow"}`), Promotable: false}); err != nil { t.Fatal(err) }
	if _, err := h.manager.PutOverlay(h.ctx, branchB.ID, OverlayEntry{Kind: OverlayDecision, Key: "candidate-b", Value: json.RawMessage(`{"choice":"fast"}`), SourceRef: "bench://b", ProvenanceRef: "fork://b", Promotable: true}); err != nil { t.Fatal(err) }
	if _, err := h.manager.PutOverlay(h.ctx, branchB.ID, OverlayEntry{Kind: OverlayMemory, Key: "temporary-hypothesis", Value: json.RawMessage(`{"note":"branch-only"}`), Promotable: false}); err != nil { t.Fatal(err) }

	objective := Objective{CorrectnessWeight: 0.7, QualityWeight: 0.1, LatencyWeight: 0.2, MinCorrectness: 1}
	if _, err := h.manager.Evaluate(h.ctx, branchA.ID, Metrics{Correctness: 1, Quality: 0.9, LatencyMS: 100}, objective, "bench://a"); err != nil { t.Fatal(err) }
	if _, err := h.manager.Evaluate(h.ctx, branchB.ID, Metrics{Correctness: 1, Quality: 0.9, LatencyMS: 20}, objective, "bench://b"); err != nil { t.Fatal(err) }
	winner, err := h.manager.Select(h.ctx, group.ID)
	if err != nil { t.Fatal(err) }
	if winner.ID != branchB.ID { t.Fatalf("winner=%s want %s", winner.ID, branchB.ID) }
	storedGroup, err := h.store.Group(h.ctx, group.ID)
	if err != nil { t.Fatal(err) }
	if storedGroup.SelectionReason == "" { t.Fatal("objective promotion reason was not persisted") }

	tx, err := h.manager.BeginPromotion(h.ctx, winner.ID, worldB)
	if err != nil { t.Fatal(err) }
	check := agenttx.CheckSpec{Name: "candidate readable", Action: world.Action{ID: NamespaceAction(winner.ID, id.ActionID("act_verify")), Kind: "fs.read_file", Purpose: "verify selected candidate", Resource: "candidate.txt", Effect: world.CanonicalEffect("fs.read_file")}}
	if _, err := h.tx.Verify(h.ctx, tx.ID, worldB, []agenttx.CheckSpec{check}); err != nil { t.Fatal(err) }
	if _, err := h.tx.Prepare(h.ctx, tx.ID, worldB); err != nil { t.Fatal(err) }
	committed, err := h.tx.Commit(h.ctx, tx.ID, worldB)
	if err != nil { t.Fatal(err) }
	if err := h.manager.FinalizePromotion(h.ctx, winner.ID, committed); err != nil { t.Fatal(err) }
	if got := readG8File(t, h.repo, "candidate.txt"); got != "fast\n" { t.Fatalf("promoted target=%q want fast", got) }

	var promoted []string
	promoter := CognitivePromoterFunc(func(_ context.Context, operationKey string, _ Checkpoint, branch Branch, entry OverlayEntry) error {
		if operationKey == "" || branch.ID != branchB.ID { return errors.New("bad promotion provenance") }
		promoted = append(promoted, entry.Key)
		return nil
	})
	if err := h.manager.PromoteCognitive(h.ctx, winner.ID, promoter); err != nil { t.Fatal(err) }
	if len(promoted) != 1 || promoted[0] != "candidate-b" { t.Fatalf("cognitive promotion leaked artifacts: %v", promoted) }

	if err := h.manager.CleanupGroup(h.ctx, group.ID); err != nil { t.Fatal(err) }
	if _, err := os.Stat(branchA.WorldRef.Ref); !errors.Is(err, os.ErrNotExist) { t.Fatalf("losing world not cleaned: %v", err) }
	if _, err := os.Stat(branchB.WorldRef.Ref); !errors.Is(err, os.ErrNotExist) { t.Fatalf("winner worktree not finalized: %v", err) }
	budget := h.budgets.Snapshot(cp.RootAgentID)
	if budget.Reserved != (scheduler.Resources{}) { t.Fatalf("fork reservations survived cleanup: %+v", budget.Reserved) }
	if budget.Spent.MoneyMicros != 450 || budget.Spent.Tokens != 1900 { t.Fatalf("settled usage=%+v", budget.Spent) }

	if err := h.manager.PurgeGroup(h.ctx, group.ID, h.now); err == nil { t.Fatal("retention window was ignored") }
	if err := h.manager.PurgeGroup(h.ctx, group.ID, retainUntil.Add(time.Second)); err != nil { t.Fatal(err) }
	if err := h.manager.PurgeCheckpoint(h.ctx, cp.ID, retainUntil.Add(time.Second)); err != nil { t.Fatal(err) }
}

func initG8GitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil { t.Skip("git is unavailable") }
	repo := t.TempDir()
	runG8Git(t, repo, "init")
	runG8Git(t, repo, "config", "core.autocrlf", "false")
	runG8Git(t, repo, "config", "user.name", "G8 Test")
	runG8Git(t, repo, "config", "user.email", "g8@example.invalid")
	for name, body := range files {
		full := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil { t.Fatal(err) }
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil { t.Fatal(err) }
	}
	runG8Git(t, repo, "add", "-A")
	runG8Git(t, repo, "commit", "-m", "base")
	return repo
}
func runG8Git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...); cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil { t.Fatalf("git %v: %v: %s", args, err, output) }
}
func readG8File(t *testing.T, repo, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repo, name)); if err != nil { t.Fatal(err) }; return string(body)
}
