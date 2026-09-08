package fork

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/scheduler"
	agenttx "github.com/Methamorphe/go-agent/internal/transaction"
	"github.com/Methamorphe/go-agent/internal/world"
)

type IDGenerator interface {
	World() (id.WorldID, error)
	Checkpoint() (id.CheckpointID, error)
	ForkGroup() (id.ForkGroupID, error)
	Fork() (id.ForkID, error)
}

type TimelineForker interface {
	ForkTimelineFromState(context.Context, agentprocess.State, agentprocess.CommandMeta) (agentprocess.State, error)
}

type Manager struct {
	store         Store
	processes     agentprocess.Store
	timeline      TimelineForker
	transactions  *agenttx.Manager
	ids           IDGenerator
	now           func() time.Time
}

func NewManager(store Store, processes agentprocess.Store, timeline TimelineForker, transactions *agenttx.Manager, ids IDGenerator, now func() time.Time) *Manager {
	if now == nil { now = time.Now }
	return &Manager{store: store, processes: processes, timeline: timeline, transactions: transactions, ids: ids, now: now}
}

func (m *Manager) CreateCheckpoint(ctx context.Context, sourceAgentID id.AgentID, snapshot world.BranchRef, contextFrontier []id.ContextPageID, retainUntil *time.Time) (Checkpoint, error) {
	if m == nil || m.store == nil || m.processes == nil || m.ids == nil { return Checkpoint{}, errors.New("fork manager dependencies are required") }
	if sourceAgentID == "" || snapshot.WorldID == "" || snapshot.BaseIdentity == "" { return Checkpoint{}, errors.New("checkpoint source agent and world snapshot are required") }
	state, err := m.processes.Current(ctx, sourceAgentID)
	if err != nil { return Checkpoint{}, err }
	if state.Status != agentprocess.StatusReady && state.Status != agentprocess.StatusSuspended {
		return Checkpoint{}, fmt.Errorf("%w: checkpoint requires quiescent READY or SUSPENDED process, got %s", ErrInvalidState, state.Status)
	}
	if state.Lease != nil || state.Wait != nil || state.Sleep != nil { return Checkpoint{}, fmt.Errorf("%w: process has active execution/wait state", ErrInvalidState) }
	sequence, err := m.processes.LedgerSequenceAtVersion(ctx, sourceAgentID, state.Version)
	if err != nil { return Checkpoint{}, err }
	body, err := json.Marshal(state)
	if err != nil { return Checkpoint{}, err }
	sum := sha256.Sum256(body)
	checkpointID, err := m.ids.Checkpoint(); if err != nil { return Checkpoint{}, err }
	cp := Checkpoint{ID: checkpointID, SourceAgentID: sourceAgentID, RootAgentID: state.RootAgentID, ProcessVersion: state.Version, LedgerSequence: sequence, LastEventID: state.LastEventID, WorldRef: snapshot, ContextFrontier: append([]id.ContextPageID(nil), contextFrontier...), ProcessStateJSON: append(json.RawMessage(nil), body...), StateHash: hex.EncodeToString(sum[:]), CreatedAt: m.now().UTC(), RetainUntil: retainUntil}
	if err := m.store.PutCheckpoint(ctx, cp); err != nil { return Checkpoint{}, err }
	return cp, nil
}

func (m *Manager) CreateGroup(ctx context.Context, checkpointID id.CheckpointID) (Group, error) {
	cp, err := m.store.Checkpoint(ctx, checkpointID); if err != nil { return Group{}, err }
	groupID, err := m.ids.ForkGroup(); if err != nil { return Group{}, err }
	now := m.now().UTC()
	g := Group{ID: groupID, CheckpointID: cp.ID, SourceAgentID: cp.SourceAgentID, State: GroupOpen, CreatedAt: now, UpdatedAt: now}
	if err := m.store.CreateGroup(ctx, g); err != nil { return Group{}, err }
	return g, nil
}

type ForkRequest struct {
	GroupID     id.ForkGroupID
	BudgetLimit scheduler.Resources
	WorkRoot    string
}

func (m *Manager) ForkWorkspace(ctx context.Context, req ForkRequest) (Branch, *world.WorkspaceWorld, error) {
	if m.timeline == nil { return Branch{}, nil, errors.New("timeline forker is required") }
	if !req.BudgetLimit.Valid() { return Branch{}, nil, errors.New("fork budget must be non-negative") }
	g, err := m.store.Group(ctx, req.GroupID); if err != nil { return Branch{}, nil, err }
	if g.State != GroupOpen { return Branch{}, nil, fmt.Errorf("%w: group is %s", ErrInvalidState, g.State) }
	cp, err := m.store.Checkpoint(ctx, g.CheckpointID); if err != nil { return Branch{}, nil, err }
	forkID, err := m.ids.Fork(); if err != nil { return Branch{}, nil, err }
	worldID, err := m.ids.World(); if err != nil { return Branch{}, nil, err }
	branchWorld, err := world.ForkWorkspaceWorld(ctx, cp.WorldRef, worldID, req.WorkRoot)
	if err != nil { return Branch{}, nil, err }
	var source agentprocess.State
	if err := json.Unmarshal(cp.ProcessStateJSON, &source); err != nil { _ = branchWorld.Rollback(ctx); return Branch{}, nil, fmt.Errorf("decode checkpoint process state: %w", err) }
	sum := sha256.Sum256(cp.ProcessStateJSON)
	if hex.EncodeToString(sum[:]) != cp.StateHash || source.AgentID != cp.SourceAgentID || source.Version != cp.ProcessVersion || source.LastEventID != cp.LastEventID {
		_ = branchWorld.Rollback(ctx)
		return Branch{}, nil, errors.New("checkpoint process state integrity mismatch")
	}
	forked, err := m.timeline.ForkTimelineFromState(ctx, source, agentprocess.CommandMeta{Actor: ledger.ActorRef{Kind: ledger.ActorSystem, ID: "cognitive-fork"}})
	if err != nil { _ = branchWorld.Rollback(ctx); return Branch{}, nil, err }
	now := m.now().UTC()
	b := Branch{ID: forkID, GroupID: g.ID, AgentID: forked.AgentID, WorldID: worldID, WorldRef: branchWorld.BranchRef(), BudgetLimit: req.BudgetLimit, State: BranchOpen, CreatedAt: now, UpdatedAt: now}
	if err := m.store.CreateBranch(ctx, b); err != nil { _ = branchWorld.Rollback(ctx); return Branch{}, nil, err }
	return b, branchWorld, nil
}

func (m *Manager) Spend(ctx context.Context, forkID id.ForkID, usage scheduler.Resources) (Branch, error) {
	if !usage.Valid() { return Branch{}, errors.New("fork usage must be non-negative") }
	b, err := m.store.Branch(ctx, forkID); if err != nil { return Branch{}, err }
	if b.State != BranchOpen { return Branch{}, fmt.Errorf("%w: budget spend requires OPEN branch", ErrInvalidState) }
	next := b.BudgetSpent.Add(usage)
	if !next.Fits(b.BudgetLimit) { return Branch{}, ErrBudgetExhausted }
	b.BudgetSpent, b.UpdatedAt = next, m.now().UTC()
	if err := m.store.UpdateBranch(ctx, b); err != nil { return Branch{}, err }
	return b, nil
}

func (m *Manager) PutOverlay(ctx context.Context, forkID id.ForkID, entry OverlayEntry) (Branch, error) {
	if entry.Kind == "" || entry.Key == "" || len(entry.Value) == 0 { return Branch{}, errors.New("overlay kind, key and value are required") }
	b, err := m.store.Branch(ctx, forkID); if err != nil { return Branch{}, err }
	if b.State != BranchOpen { return Branch{}, fmt.Errorf("%w: overlay write requires OPEN branch", ErrInvalidState) }
	entry.CreatedAt = m.now().UTC()
	b.CognitiveOverlay = append(b.CognitiveOverlay, entry)
	b.UpdatedAt = entry.CreatedAt
	if err := m.store.UpdateBranch(ctx, b); err != nil { return Branch{}, err }
	return b, nil
}

func (m *Manager) Evaluate(ctx context.Context, forkID id.ForkID, metrics Metrics, objective Objective, evidenceRef string) (Branch, error) {
	if err := objective.Validate(); err != nil { return Branch{}, err }
	if metrics.Correctness < 0 || metrics.Correctness > 1 || metrics.Quality < 0 || metrics.Quality > 1 || metrics.LatencyMS < 0 || metrics.MoneyMicros < 0 || metrics.Tokens < 0 { return Branch{}, errors.New("invalid evaluation metrics") }
	b, err := m.store.Branch(ctx, forkID); if err != nil { return Branch{}, err }
	if b.State != BranchOpen { return Branch{}, fmt.Errorf("%w: evaluation requires OPEN branch", ErrInvalidState) }
	e := Evaluate(metrics, objective, m.now()); e.EvidenceRef = evidenceRef
	b.Evaluation, b.State, b.UpdatedAt = &e, BranchEvaluated, e.EvaluatedAt
	if err := m.store.UpdateBranch(ctx, b); err != nil { return Branch{}, err }
	return b, nil
}

func (m *Manager) Select(ctx context.Context, groupID id.ForkGroupID) (Branch, error) {
	g, err := m.store.Group(ctx, groupID); if err != nil { return Branch{}, err }
	if g.State != GroupOpen && g.State != GroupEvaluated { return Branch{}, fmt.Errorf("%w: group cannot select from %s", ErrInvalidState, g.State) }
	branches, err := m.store.Branches(ctx, groupID); if err != nil { return Branch{}, err }
	winnerID, err := SelectWinner(branches); if err != nil { return Branch{}, err }
	var winner Branch
	for _, b := range branches {
		switch { case b.ID == winnerID: b.State = BranchSelected; winner = b; case b.State == BranchEvaluated: b.State = BranchDiscarded }
		b.UpdatedAt = m.now().UTC(); if err := m.store.UpdateBranch(ctx, b); err != nil { return Branch{}, err }
	}
	g.State, g.WinnerForkID, g.UpdatedAt = GroupEvaluated, winnerID, m.now().UTC()
	if err := m.store.UpdateGroup(ctx, g); err != nil { return Branch{}, err }
	return winner, nil
}

func (m *Manager) BeginPromotion(ctx context.Context, forkID id.ForkID, branchWorld world.TransactionalWorld) (agenttx.Transaction, error) {
	if m.transactions == nil { return agenttx.Transaction{}, errors.New("transaction manager is required") }
	b, err := m.store.Branch(ctx, forkID); if err != nil { return agenttx.Transaction{}, err }
	if b.State != BranchSelected || b.Evaluation == nil || !b.Evaluation.Eligible { return agenttx.Transaction{}, ErrPromotionRequired }
	g, err := m.store.Group(ctx, b.GroupID); if err != nil { return agenttx.Transaction{}, err }
	if g.WinnerForkID != b.ID || g.State != GroupEvaluated { return agenttx.Transaction{}, ErrPromotionRequired }
	cp, err := m.store.Checkpoint(ctx, g.CheckpointID); if err != nil { return agenttx.Transaction{}, err }
	if branchWorld.BranchRef().WorldID != b.WorldID { return agenttx.Transaction{}, errors.New("promotion world does not match selected fork") }
	g.State, g.UpdatedAt = GroupPromoting, m.now().UTC(); if err := m.store.UpdateGroup(ctx, g); err != nil { return agenttx.Transaction{}, err }
	tx, err := m.transactions.Begin(ctx, agenttx.BeginRequest{AgentID: b.AgentID, BaseCheckpoint: cp.ID}, branchWorld)
	if err != nil { g.State = GroupEvaluated; _ = m.store.UpdateGroup(ctx, g); return agenttx.Transaction{}, err }
	return tx, nil
}

func (m *Manager) MarkPromoted(ctx context.Context, forkID id.ForkID) error {
	b, err := m.store.Branch(ctx, forkID); if err != nil { return err }
	g, err := m.store.Group(ctx, b.GroupID); if err != nil { return err }
	if g.WinnerForkID != b.ID || g.State != GroupPromoting { return ErrPromotionRequired }
	b.State, b.UpdatedAt = BranchPromoted, m.now().UTC(); g.State, g.UpdatedAt = GroupPromoted, b.UpdatedAt
	if err := m.store.UpdateBranch(ctx, b); err != nil { return err }; return m.store.UpdateGroup(ctx, g)
}

func (m *Manager) PromoteableOverlay(ctx context.Context, forkID id.ForkID) ([]OverlayEntry, error) {
	b, err := m.store.Branch(ctx, forkID); if err != nil { return nil, err }
	if b.State != BranchSelected && b.State != BranchPromoted { return nil, ErrPromotionRequired }
	out := make([]OverlayEntry, 0); for _, entry := range b.CognitiveOverlay { if entry.Promotable { out = append(out, entry) } }; return out, nil
}

func (m *Manager) RestoreAsNewTimeline(ctx context.Context, checkpointID id.CheckpointID, budget scheduler.Resources, workRoot string) (Group, Branch, *world.WorkspaceWorld, error) {
	g, err := m.CreateGroup(ctx, checkpointID); if err != nil { return Group{}, Branch{}, nil, err }
	b, w, err := m.ForkWorkspace(ctx, ForkRequest{GroupID: g.ID, BudgetLimit: budget, WorkRoot: workRoot}); return g, b, w, err
}

func (m *Manager) CleanupGroup(ctx context.Context, groupID id.ForkGroupID) error {
	g, err := m.store.Group(ctx, groupID); if err != nil { return err }
	if g.State == GroupPromoting { return fmt.Errorf("%w: cannot cleanup during promotion", ErrInvalidState) }
	branches, err := m.store.Branches(ctx, groupID); if err != nil { return err }
	for _, b := range branches {
		if b.WorldRef.Type == world.TypeWorkspace { if w, reopenErr := world.ReopenWorkspaceWorld(b.WorldRef); reopenErr == nil { _ = w.Rollback(ctx) } }
		if err := m.store.DeleteBranch(ctx, b.ID); err != nil { return err }
	}
	g.State, g.UpdatedAt = GroupDiscarded, m.now().UTC(); if err := m.store.UpdateGroup(ctx, g); err != nil { return err }
	return m.store.DeleteGroup(ctx, groupID)
}
