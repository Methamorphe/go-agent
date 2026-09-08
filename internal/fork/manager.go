package fork

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
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

type ProcessRuntime interface {
	ExecutionFrontier(context.Context, id.AgentID) (agentprocess.ExecutionFrontier, error)
	ForkTimelineFromState(context.Context, agentprocess.State, agentprocess.CommandMeta) (agentprocess.State, error)
}

type AuthorityProvider interface {
	CurrentAuthority(context.Context, id.AgentID) (world.Intent, []world.Capability, error)
}

// CognitivePromoter must be idempotent for operationKey. G8 invokes it only
// for explicitly whitelisted winner artifacts after World promotion succeeds.
type CognitivePromoter interface {
	PromoteForkArtifact(context.Context, string, Checkpoint, Branch, OverlayEntry) error
}

type Manager struct {
	mu           sync.Mutex
	store        Store
	processes    agentprocess.Store
	runtime      ProcessRuntime
	transactions *agenttx.Manager
	budgets      scheduler.BudgetStore
	authority    AuthorityProvider
	ids          IDGenerator
	now          func() time.Time
}

func NewManager(store Store, processes agentprocess.Store, runtime ProcessRuntime, transactions *agenttx.Manager, budgets scheduler.BudgetStore, authority AuthorityProvider, ids IDGenerator, now func() time.Time) *Manager {
	if now == nil { now = time.Now }
	return &Manager{store: store, processes: processes, runtime: runtime, transactions: transactions, budgets: budgets, authority: authority, ids: ids, now: now}
}

type CheckpointRequest struct {
	SourceAgentID    id.AgentID
	WorldSnapshot    world.BranchRef
	ContextRefs      []id.ContextPageID
	RequiredResults  []ResultDependency
	ChildDependencies []ChildDependency
	RetainUntil      *time.Time
}

func (m *Manager) CreateCheckpoint(ctx context.Context, req CheckpointRequest) (Checkpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m == nil || m.store == nil || m.processes == nil || m.runtime == nil || m.transactions == nil || m.authority == nil || m.ids == nil {
		return Checkpoint{}, errors.New("fork manager dependencies are required")
	}
	if req.SourceAgentID == "" || req.WorldSnapshot.WorldID == "" || req.WorldSnapshot.BaseIdentity == "" {
		return Checkpoint{}, errors.New("checkpoint source agent and immutable world snapshot are required")
	}
	if req.WorldSnapshot.Type != world.TypeWorkspace {
		return Checkpoint{}, fmt.Errorf("%w: v0 mutation-capable forks require WorkspaceWorld snapshot guarantees", ErrUnsafeCheckpoint)
	}
	state, err := m.processes.Current(ctx, req.SourceAgentID)
	if err != nil { return Checkpoint{}, err }
	if state.Status != agentprocess.StatusReady && state.Status != agentprocess.StatusSuspended {
		return Checkpoint{}, fmt.Errorf("%w: checkpoint requires READY or SUSPENDED process, got %s", ErrUnsafeCheckpoint, state.Status)
	}
	if state.Lease != nil || state.Wait != nil || state.Sleep != nil {
		return Checkpoint{}, fmt.Errorf("%w: process has active execution/wait state", ErrUnsafeCheckpoint)
	}
	processFrontier, err := m.runtime.ExecutionFrontier(ctx, req.SourceAgentID)
	if err != nil { return Checkpoint{}, err }
	if processFrontier.ProcessVersion != state.Version || processFrontier.AgentID != state.AgentID {
		return Checkpoint{}, fmt.Errorf("%w: process frontier changed during checkpoint", ErrUnsafeCheckpoint)
	}
	if len(processFrontier.InFlightActions) > 0 || len(processFrontier.InFlightInvocations) > 0 {
		return Checkpoint{}, fmt.Errorf("%w: in-flight model/tool operations remain", ErrUnsafeCheckpoint)
	}
	if err := m.transactions.RequireExecutionEditQuiescence(ctx, req.SourceAgentID); err != nil {
		return Checkpoint{}, fmt.Errorf("%w: %v", ErrUnsafeCheckpoint, err)
	}
	txFrontier, err := m.transactions.ExecutionFrontier(ctx, req.SourceAgentID)
	if err != nil { return Checkpoint{}, err }
	if len(txFrontier.UnknownOutcomeActions) > 0 || len(txFrontier.TransactionRefs) > 0 {
		return Checkpoint{}, fmt.Errorf("%w: unresolved transaction obligations remain", ErrUnsafeCheckpoint)
	}
	completed := make(map[string]struct{}, len(processFrontier.CompletedResultRefs))
	for _, ref := range processFrontier.CompletedResultRefs { completed[ref] = struct{}{} }
	for _, required := range req.RequiredResults {
		if required.Ref == "" { return Checkpoint{}, errors.New("required result reference is empty") }
		if _, ok := completed[required.Ref]; !ok {
			return Checkpoint{}, fmt.Errorf("%w: required result %q is not present in durable frontier", ErrUnsafeCheckpoint, required.Ref)
		}
	}
	intent, grants, err := m.authority.CurrentAuthority(ctx, req.SourceAgentID)
	if err != nil { return Checkpoint{}, err }
	body, err := json.Marshal(state)
	if err != nil { return Checkpoint{}, err }
	sum := sha256.Sum256(body)
	checkpointID, err := m.ids.Checkpoint()
	if err != nil { return Checkpoint{}, err }
	now := m.now().UTC()
	frontier := ExecutionFrontier{
		AgentID: req.SourceAgentID, ProcessVersion: state.Version, LedgerSequence: processFrontier.LedgerSequence,
		InvocationBoundary: processFrontier.LastInvocationID,
		CompletedResultRefs: append([]string(nil), processFrontier.CompletedResultRefs...),
		InFlightInvocations: append([]id.InvocationID(nil), processFrontier.InFlightInvocations...),
		InFlightActions: append([]id.ActionID(nil), processFrontier.InFlightActions...),
		UnknownOutcomeActions: append([]id.ActionID(nil), txFrontier.UnknownOutcomeActions...),
		TransactionRefs: append([]id.TransactionID(nil), txFrontier.TransactionRefs...),
		RequiredResults: append([]ResultDependency(nil), req.RequiredResults...),
		ChildDependencies: append([]ChildDependency(nil), req.ChildDependencies...),
		ContextRefs: append([]id.ContextPageID(nil), req.ContextRefs...),
		WorldSnapshotRef: req.WorldSnapshot,
	}
	cp := Checkpoint{
		ID: checkpointID, Class: CheckpointCommittable, SourceAgentID: state.AgentID, RootAgentID: state.RootAgentID,
		Frontier: frontier,
		Authority: AuthoritySnapshot{Intent: intent, Capabilities: append([]world.Capability(nil), grants...), CapturedAt: now},
		LastEventID: state.LastEventID, ProcessStateJSON: append(json.RawMessage(nil), body...),
		StateHash: hex.EncodeToString(sum[:]), CreatedAt: now, RetainUntil: req.RetainUntil,
	}
	if err := m.store.PutCheckpoint(ctx, cp); err != nil { return Checkpoint{}, err }
	return cp, nil
}

func (m *Manager) CreateGroup(ctx context.Context, checkpointID id.CheckpointID) (Group, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.createGroupLocked(ctx, checkpointID)
}

func (m *Manager) createGroupLocked(ctx context.Context, checkpointID id.CheckpointID) (Group, error) {
	cp, err := m.store.Checkpoint(ctx, checkpointID)
	if err != nil { return Group{}, err }
	if cp.Class != CheckpointForkable && cp.Class != CheckpointCommittable { return Group{}, ErrUnsafeCheckpoint }
	groupID, err := m.ids.ForkGroup()
	if err != nil { return Group{}, err }
	now := m.now().UTC()
	g := Group{ID: groupID, CheckpointID: cp.ID, SourceAgentID: cp.SourceAgentID, State: GroupOpen, CreatedAt: now, UpdatedAt: now}
	if err := m.store.CreateGroup(ctx, g); err != nil { return Group{}, err }
	return g, nil
}

type ForkRequest struct {
	GroupID      id.ForkGroupID
	BudgetLimit  scheduler.Resources
	Capabilities []world.Capability
	WorkRoot     string
}

func (m *Manager) ForkWorkspace(ctx context.Context, req ForkRequest) (Branch, world.TransactionalWorld, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.forkWorkspaceLocked(ctx, req)
}

func (m *Manager) forkWorkspaceLocked(ctx context.Context, req ForkRequest) (Branch, world.TransactionalWorld, error) {
	if m.runtime == nil || m.budgets == nil || m.authority == nil { return Branch{}, nil, errors.New("fork runtime, budget store and authority provider are required") }
	if !req.BudgetLimit.Valid() { return Branch{}, nil, errors.New("fork budget must be non-negative") }
	g, err := m.store.Group(ctx, req.GroupID)
	if err != nil { return Branch{}, nil, err }
	if g.State != GroupOpen { return Branch{}, nil, fmt.Errorf("%w: group is %s", ErrInvalidState, g.State) }
	cp, err := m.store.Checkpoint(ctx, g.CheckpointID)
	if err != nil { return Branch{}, nil, err }
	if cp.Class != CheckpointCommittable || cp.Frontier.WorldSnapshotRef.Type != world.TypeWorkspace {
		return Branch{}, nil, ErrUnsafeCheckpoint
	}
	currentIntent, currentGrants, err := m.authority.CurrentAuthority(ctx, cp.SourceAgentID)
	if err != nil { return Branch{}, nil, err }
	now := m.now().UTC()
	branchGrants := intersectCapabilities(cp.Authority.Capabilities, currentGrants, now)
	if len(req.Capabilities) > 0 {
		if !world.ValidateDelegation(cp.Authority.Capabilities, req.Capabilities, now) || !world.ValidateDelegation(currentGrants, req.Capabilities, now) {
			return Branch{}, nil, errors.New("requested fork authority exceeds checkpoint or current authority")
		}
		branchGrants = append([]world.Capability(nil), req.Capabilities...)
	}
	reservation, err := m.budgets.Reserve(cp.RootAgentID, req.BudgetLimit)
	if err != nil { return Branch{}, nil, err }
	reserved := true
	defer func() { if reserved { _ = m.budgets.Release(reservation) } }()

	forkID, err := m.ids.Fork()
	if err != nil { return Branch{}, nil, err }
	worldID, err := m.ids.World()
	if err != nil { return Branch{}, nil, err }
	rawWorld, err := world.ForkWorkspaceWorld(ctx, cp.Frontier.WorldSnapshotRef, worldID, req.WorkRoot)
	if err != nil { return Branch{}, nil, err }
	worldOwned := true
	defer func() { if worldOwned { _ = rawWorld.Rollback(ctx) } }()

	var source agentprocess.State
	if err := json.Unmarshal(cp.ProcessStateJSON, &source); err != nil { return Branch{}, nil, fmt.Errorf("decode checkpoint process state: %w", err) }
	sum := sha256.Sum256(cp.ProcessStateJSON)
	if hex.EncodeToString(sum[:]) != cp.StateHash || source.AgentID != cp.SourceAgentID || source.Version != cp.Frontier.ProcessVersion || source.LastEventID != cp.LastEventID {
		return Branch{}, nil, errors.New("checkpoint process state integrity mismatch")
	}
	forked, err := m.runtime.ForkTimelineFromState(ctx, source, agentprocess.CommandMeta{Actor: ledger.ActorRef{Kind: ledger.ActorSystem, ID: "cognitive-fork"}})
	if err != nil { return Branch{}, nil, err }
	secure := world.NewSecureTransactionalWorld(world.NewAuthorizer(currentIntent, branchGrants, nil), rawWorld, m.now)
	speculative := world.NewSpeculativeTransactionalWorld(secure)
	b := Branch{
		ID: forkID, GroupID: g.ID, AgentID: forked.AgentID, WorldID: worldID, WorldRef: speculative.BranchRef(),
		Authority: AuthoritySnapshot{Intent: currentIntent, Capabilities: append([]world.Capability(nil), branchGrants...), CapturedAt: now},
		BudgetLimit: req.BudgetLimit, BudgetReservation: reservation, State: BranchOpen, CreatedAt: now, UpdatedAt: now,
	}
	if err := m.store.CreateBranch(ctx, b); err != nil { return Branch{}, nil, err }
	reserved = false
	worldOwned = false
	return b, speculative, nil
}

func (m *Manager) Spend(ctx context.Context, forkID id.ForkID, usage scheduler.Resources) (Branch, error) {
	m.mu.Lock(); defer m.mu.Unlock()
	if !usage.Valid() { return Branch{}, errors.New("fork usage must be non-negative") }
	b, err := m.store.Branch(ctx, forkID)
	if err != nil { return Branch{}, err }
	if b.State != BranchOpen || b.BudgetSettled { return Branch{}, fmt.Errorf("%w: budget spend requires open unsettled branch", ErrInvalidState) }
	next := b.BudgetSpent.Add(usage)
	if !next.Fits(b.BudgetLimit) { return Branch{}, ErrBudgetExhausted }
	b.BudgetSpent, b.UpdatedAt = next, m.now().UTC()
	if err := m.store.UpdateBranch(ctx, b); err != nil { return Branch{}, err }
	return b, nil
}

func (m *Manager) PutOverlay(ctx context.Context, forkID id.ForkID, entry OverlayEntry) (Branch, error) {
	m.mu.Lock(); defer m.mu.Unlock()
	if entry.Kind == "" || entry.Key == "" || len(entry.Value) == 0 { return Branch{}, errors.New("overlay kind, key and value are required") }
	if entry.Promotable && entry.Kind == OverlayMemory && entry.ProvenanceRef == "" { return Branch{}, errors.New("promotable memory requires provenance") }
	b, err := m.store.Branch(ctx, forkID)
	if err != nil { return Branch{}, err }
	if b.State != BranchOpen { return Branch{}, fmt.Errorf("%w: overlay write requires OPEN branch", ErrInvalidState) }
	entry.CreatedAt = m.now().UTC()
	entry.Promoted = false
	b.CognitiveOverlay = append(b.CognitiveOverlay, entry)
	b.UpdatedAt = entry.CreatedAt
	if err := m.store.UpdateBranch(ctx, b); err != nil { return Branch{}, err }
	return b, nil
}

func (m *Manager) Evaluate(ctx context.Context, forkID id.ForkID, metrics Metrics, objective Objective, evidenceRef string) (Branch, error) {
	m.mu.Lock(); defer m.mu.Unlock()
	if err := objective.Validate(); err != nil { return Branch{}, err }
	if evidenceRef == "" { return Branch{}, errors.New("objective evaluation evidence reference is required") }
	if metrics.Correctness < 0 || metrics.Correctness > 1 || metrics.Quality < 0 || metrics.Quality > 1 || metrics.LatencyMS < 0 || metrics.MoneyMicros < 0 || metrics.Tokens < 0 { return Branch{}, errors.New("invalid evaluation metrics") }
	b, err := m.store.Branch(ctx, forkID)
	if err != nil { return Branch{}, err }
	if b.State != BranchOpen { return Branch{}, fmt.Errorf("%w: evaluation requires OPEN branch", ErrInvalidState) }
	e := Evaluate(metrics, objective, m.now()); e.EvidenceRef = evidenceRef
	b.Evaluation, b.State, b.UpdatedAt = &e, BranchEvaluated, e.EvaluatedAt
	if err := m.store.UpdateBranch(ctx, b); err != nil { return Branch{}, err }
	return b, nil
}

func (m *Manager) Select(ctx context.Context, groupID id.ForkGroupID) (Branch, error) {
	m.mu.Lock(); defer m.mu.Unlock()
	g, err := m.store.Group(ctx, groupID)
	if err != nil { return Branch{}, err }
	if g.State != GroupOpen { return Branch{}, fmt.Errorf("%w: group cannot select from %s", ErrInvalidState, g.State) }
	branches, err := m.store.Branches(ctx, groupID)
	if err != nil { return Branch{}, err }
	if len(branches) == 0 { return Branch{}, ErrNoEligibleWinner }
	for _, b := range branches {
		if b.State != BranchEvaluated || b.Evaluation == nil { return Branch{}, fmt.Errorf("%w: every fork must be evaluated before selection", ErrInvalidState) }
	}
	winnerID, err := SelectWinner(branches)
	if err != nil { return Branch{}, err }
	var winner Branch
	for _, b := range branches {
		if b.ID == winnerID { b.State = BranchSelected; winner = b } else { b.State = BranchDiscarded }
		b.UpdatedAt = m.now().UTC()
		if err := m.store.UpdateBranch(ctx, b); err != nil { return Branch{}, err }
	}
	g.State, g.WinnerForkID, g.UpdatedAt = GroupEvaluated, winnerID, m.now().UTC()
	g.SelectionReason = fmt.Sprintf("selected %s: highest eligible objective score %.6f; deterministic tie-break=fork_id", winnerID, winner.Evaluation.Score)
	if err := m.store.UpdateGroup(ctx, g); err != nil { return Branch{}, err }
	return winner, nil
}

func (m *Manager) BeginPromotion(ctx context.Context, forkID id.ForkID, branchWorld world.TransactionalWorld) (agenttx.Transaction, error) {
	m.mu.Lock(); defer m.mu.Unlock()
	if m.transactions == nil { return agenttx.Transaction{}, errors.New("transaction manager is required") }
	b, err := m.store.Branch(ctx, forkID)
	if err != nil { return agenttx.Transaction{}, err }
	if b.State != BranchSelected || b.Evaluation == nil || !b.Evaluation.Eligible { return agenttx.Transaction{}, ErrPromotionRequired }
	g, err := m.store.Group(ctx, b.GroupID)
	if err != nil { return agenttx.Transaction{}, err }
	if g.WinnerForkID != b.ID || g.State != GroupEvaluated { return agenttx.Transaction{}, ErrPromotionRequired }
	cp, err := m.store.Checkpoint(ctx, g.CheckpointID)
	if err != nil { return agenttx.Transaction{}, err }
	if branchWorld == nil || branchWorld.BranchRef().WorldID != b.WorldID || branchWorld.BranchRef().BaseIdentity != cp.Frontier.WorldSnapshotRef.BaseIdentity { return agenttx.Transaction{}, errors.New("promotion world does not match selected fork/base") }
	g.State, g.UpdatedAt = GroupPromoting, m.now().UTC()
	if err := m.store.UpdateGroup(ctx, g); err != nil { return agenttx.Transaction{}, err }
	tx, err := m.transactions.Begin(ctx, agenttx.BeginRequest{AgentID: b.AgentID, BaseCheckpoint: cp.ID}, branchWorld)
	if err != nil { g.State = GroupEvaluated; _ = m.store.UpdateGroup(ctx, g); return agenttx.Transaction{}, err }
	return tx, nil
}

func (m *Manager) FinalizePromotion(ctx context.Context, forkID id.ForkID, committed agenttx.Transaction) error {
	m.mu.Lock(); defer m.mu.Unlock()
	b, err := m.store.Branch(ctx, forkID)
	if err != nil { return err }
	g, err := m.store.Group(ctx, b.GroupID)
	if err != nil { return err }
	cp, err := m.store.Checkpoint(ctx, g.CheckpointID)
	if err != nil { return err }
	if g.WinnerForkID != b.ID || g.State != GroupPromoting || committed.State != agenttx.StateCommitted || committed.AgentID != b.AgentID || committed.WorldID != b.WorldID || committed.BaseCheckpoint != cp.ID {
		return ErrPromotionRequired
	}
	if err := m.settleBudgetLocked(b); err != nil { return err }
	b, err = m.store.Branch(ctx, forkID)
	if err != nil { return err }
	b.State, b.UpdatedAt = BranchPromoted, m.now().UTC()
	g.State, g.UpdatedAt = GroupPromoted, b.UpdatedAt
	if err := m.store.UpdateBranch(ctx, b); err != nil { return err }
	return m.store.UpdateGroup(ctx, g)
}

func (m *Manager) PromoteCognitive(ctx context.Context, forkID id.ForkID, promoter CognitivePromoter) error {
	m.mu.Lock(); defer m.mu.Unlock()
	if promoter == nil { return errors.New("cognitive promoter is required") }
	b, err := m.store.Branch(ctx, forkID)
	if err != nil { return err }
	if b.State != BranchPromoted { return ErrPromotionRequired }
	g, err := m.store.Group(ctx, b.GroupID)
	if err != nil { return err }
	cp, err := m.store.Checkpoint(ctx, g.CheckpointID)
	if err != nil { return err }
	for i := range b.CognitiveOverlay {
		entry := b.CognitiveOverlay[i]
		if !entry.Promotable || entry.Promoted { continue }
		opKey := "fork-promote:" + b.ID.String() + ":" + entry.Key
		if err := promoter.PromoteForkArtifact(ctx, opKey, cp, b, entry); err != nil { return err }
		b.CognitiveOverlay[i].Promoted = true
		b.UpdatedAt = m.now().UTC()
		if err := m.store.UpdateBranch(ctx, b); err != nil { return err }
	}
	return nil
}

func (m *Manager) PromoteableOverlay(ctx context.Context, forkID id.ForkID) ([]OverlayEntry, error) {
	b, err := m.store.Branch(ctx, forkID)
	if err != nil { return nil, err }
	if b.State != BranchSelected && b.State != BranchPromoted { return nil, ErrPromotionRequired }
	out := make([]OverlayEntry, 0)
	for _, entry := range b.CognitiveOverlay { if entry.Promotable { out = append(out, entry) } }
	return out, nil
}

func (m *Manager) RestoreAsNewTimeline(ctx context.Context, checkpointID id.CheckpointID, budget scheduler.Resources, workRoot string) (Group, Branch, world.TransactionalWorld, error) {
	m.mu.Lock(); defer m.mu.Unlock()
	g, err := m.createGroupLocked(ctx, checkpointID)
	if err != nil { return Group{}, Branch{}, nil, err }
	b, w, err := m.forkWorkspaceLocked(ctx, ForkRequest{GroupID: g.ID, BudgetLimit: budget, WorkRoot: workRoot})
	if err != nil { g.State, g.UpdatedAt = GroupDiscarded, m.now().UTC(); _ = m.store.UpdateGroup(ctx, g); return g, Branch{}, nil, err }
	return g, b, w, nil
}

func (m *Manager) CleanupGroup(ctx context.Context, groupID id.ForkGroupID) error {
	m.mu.Lock(); defer m.mu.Unlock()
	g, err := m.store.Group(ctx, groupID)
	if errors.Is(err, ErrNotFound) { return nil }
	if err != nil { return err }
	if g.State == GroupPromoting { return fmt.Errorf("%w: cannot cleanup during promotion", ErrInvalidState) }
	branches, err := m.store.Branches(ctx, groupID)
	if err != nil { return err }
	for _, b := range branches {
		if err := cleanupBranchWorld(ctx, b); err != nil { return err }
		if err := m.settleBudgetLocked(b); err != nil { return err }
		b, err = m.store.Branch(ctx, b.ID)
		if err != nil { return err }
		if b.State != BranchPromoted { b.State = BranchDiscarded }
		b.UpdatedAt = m.now().UTC()
		if err := m.store.UpdateBranch(ctx, b); err != nil { return err }
	}
	if g.State != GroupPromoted { g.State = GroupDiscarded }
	g.UpdatedAt = m.now().UTC()
	return m.store.UpdateGroup(ctx, g)
}

func (m *Manager) PurgeGroup(ctx context.Context, groupID id.ForkGroupID, now time.Time) error {
	m.mu.Lock(); defer m.mu.Unlock()
	g, err := m.store.Group(ctx, groupID)
	if errors.Is(err, ErrNotFound) { return nil }
	if err != nil { return err }
	if g.State != GroupPromoted && g.State != GroupDiscarded { return fmt.Errorf("%w: group is not terminal", ErrInvalidState) }
	cp, err := m.store.Checkpoint(ctx, g.CheckpointID)
	if err != nil { return err }
	if cp.RetainUntil != nil && now.UTC().Before(cp.RetainUntil.UTC()) { return fmt.Errorf("retention window has not expired") }
	branches, err := m.store.Branches(ctx, groupID)
	if err != nil { return err }
	for _, b := range branches {
		if !b.BudgetSettled { return fmt.Errorf("%w: fork %s budget is not settled", ErrInvalidState, b.ID) }
		if b.WorldRef.Type == world.TypeWorkspace {
			if _, err := os.Stat(b.WorldRef.Ref); err == nil { return fmt.Errorf("%w: fork %s world still exists", ErrInvalidState, b.ID) } else if !errors.Is(err, os.ErrNotExist) { return err }
		}
		if err := m.store.DeleteBranch(ctx, b.ID); err != nil { return err }
	}
	return m.store.DeleteGroup(ctx, groupID)
}

func (m *Manager) PurgeCheckpoint(ctx context.Context, checkpointID id.CheckpointID, now time.Time) error {
	m.mu.Lock(); defer m.mu.Unlock()
	cp, err := m.store.Checkpoint(ctx, checkpointID)
	if errors.Is(err, ErrNotFound) { return nil }
	if err != nil { return err }
	if cp.RetainUntil != nil && now.UTC().Before(cp.RetainUntil.UTC()) { return fmt.Errorf("retention window has not expired") }
	if err := world.ReleaseWorkspaceSnapshot(ctx, cp.Frontier.WorldSnapshotRef); err != nil { return err }
	return m.store.DeleteCheckpoint(ctx, checkpointID)
}

func (m *Manager) settleBudgetLocked(b Branch) error {
	if b.BudgetSettled { return nil }
	if m.budgets == nil || b.BudgetReservation == "" { return errors.New("fork budget reservation is missing") }
	err := m.budgets.Settle(b.BudgetReservation, b.BudgetSpent)
	if err != nil && !errors.Is(err, scheduler.ErrReservationSettled) { return err }
	b.BudgetSettled, b.UpdatedAt = true, m.now().UTC()
	return m.store.UpdateBranch(context.Background(), b)
}

func cleanupBranchWorld(ctx context.Context, b Branch) error {
	if b.WorldRef.Type != world.TypeWorkspace || b.WorldRef.Ref == "" { return nil }
	if _, err := os.Stat(b.WorldRef.Ref); errors.Is(err, os.ErrNotExist) { return nil } else if err != nil { return err }
	w, err := world.ReopenWorkspaceWorld(b.WorldRef)
	if err != nil { return err }
	return w.Rollback(ctx)
}

func intersectCapabilities(historical, current []world.Capability, now time.Time) []world.Capability {
	out := make([]world.Capability, 0, len(current))
	for _, grant := range current {
		if grant.ExpiresAt != nil && !now.Before(*grant.ExpiresAt) { continue }
		if world.ValidateDelegation(historical, []world.Capability{grant}, now) { out = append(out, grant) }
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Domain == out[j].Domain { return out[i].Scope < out[j].Scope }
		return out[i].Domain < out[j].Domain
	})
	return out
}
