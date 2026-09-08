package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
)

type ProjectionStore interface {
	Current(context.Context, id.AgentID) (agentprocess.State, error)
	LatestRoot(context.Context) (agentprocess.State, error)
	ProcessTree(context.Context, id.AgentID, int) ([]agentprocess.State, error)
	WorkspaceEventsBefore(context.Context, id.AgentID, uint64, int) ([]ledger.Event, error)
	SearchWorkspaceEvents(context.Context, id.AgentID, string, int) ([]ledger.Event, error)
	WorkspaceInspector(context.Context, id.AgentID) (Inspector, error)
}

type Service struct {
	store ProjectionStore
	now   func() time.Time
}

func New(store ProjectionStore, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, now: now}
}

func (s *Service) Attach(ctx context.Context, request AttachRequest) (Snapshot, error) {
	if s == nil || s.store == nil {
		return Snapshot{}, errs.New(errs.CodeUnavailable, "workspace.attach", "workspace projection store is unavailable")
	}
	mode := request.Mode
	if mode == "" {
		mode = ModeAct
	}
	if !mode.Valid() {
		return Snapshot{}, errs.New(errs.CodeInvalidArgument, "workspace.attach", "invalid workspace mode")
	}
	historyLimit, err := pageLimit(request.HistoryLimit)
	if err != nil {
		return Snapshot{}, err
	}
	treeLimit := request.TreeLimit
	if treeLimit <= 0 {
		treeLimit = DefaultTreeLimit
	}
	if treeLimit > MaxTreeLimit {
		return Snapshot{}, errs.New(errs.CodeInvalidArgument, "workspace.attach", "tree limit exceeds maximum")
	}

	var focused agentprocess.State
	if request.AgentID == "" {
		focused, err = s.store.LatestRoot(ctx)
	} else {
		focused, err = s.store.Current(ctx, request.AgentID)
	}
	if err != nil {
		return Snapshot{}, err
	}

	tree, err := s.store.ProcessTree(ctx, focused.RootAgentID, treeLimit)
	if err != nil {
		return Snapshot{}, err
	}
	viewport, err := s.history(ctx, focused.AgentID, 0, historyLimit)
	if err != nil {
		return Snapshot{}, err
	}
	inspector, err := s.store.WorkspaceInspector(ctx, focused.RootAgentID)
	if err != nil {
		return Snapshot{}, err
	}

	return Snapshot{
		ProtocolVersion: ProtocolVersion,
		RootAgentID:     focused.RootAgentID,
		FocusedAgentID:  focused.AgentID,
		Mode:            mode,
		Tree:            summarizeTree(tree),
		Viewport:        viewport,
		Inspector:       inspector,
		GeneratedAt:     s.now().UTC(),
	}, nil
}

func (s *Service) History(ctx context.Context, request HistoryRequest) (ConversationViewport, error) {
	if request.AgentID == "" {
		return ConversationViewport{}, errs.New(errs.CodeInvalidArgument, "workspace.history", "agent id is required")
	}
	limit, err := pageLimit(request.Limit)
	if err != nil {
		return ConversationViewport{}, err
	}
	return s.history(ctx, request.AgentID, request.Before, limit)
}

func (s *Service) history(ctx context.Context, agentID id.AgentID, before uint64, limit int) (ConversationViewport, error) {
	events, err := s.store.WorkspaceEventsBefore(ctx, agentID, before, limit+1)
	if err != nil {
		return ConversationViewport{}, err
	}
	hasPrevious := len(events) > limit
	if hasPrevious {
		events = events[len(events)-limit:]
	}
	blocks := make([]Block, 0, len(events))
	for _, event := range events {
		blocks = append(blocks, blockFromEvent(event))
	}
	viewport := ConversationViewport{AgentID: agentID, Blocks: blocks, Before: before, HasPrevious: hasPrevious}
	if len(blocks) > 0 {
		viewport.Oldest = blocks[0].ProcessVersion
		viewport.Newest = blocks[len(blocks)-1].ProcessVersion
	}
	return viewport, nil
}

func (s *Service) Search(ctx context.Context, request SearchRequest) (SearchResult, error) {
	if request.RootAgentID == "" {
		return SearchResult{}, errs.New(errs.CodeInvalidArgument, "workspace.search", "root agent id is required")
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return SearchResult{}, errs.New(errs.CodeInvalidArgument, "workspace.search", "query is required")
	}
	limit, err := pageLimit(request.Limit)
	if err != nil {
		return SearchResult{}, err
	}
	events, err := s.store.SearchWorkspaceEvents(ctx, request.RootAgentID, query, limit)
	if err != nil {
		return SearchResult{}, err
	}
	blocks := make([]Block, 0, len(events))
	for _, event := range events {
		blocks = append(blocks, blockFromEvent(event))
	}
	return SearchResult{Blocks: blocks}, nil
}

func pageLimit(limit int) (int, error) {
	if limit <= 0 {
		return DefaultPageSize, nil
	}
	if limit > MaxPageSize {
		return 0, errs.New(errs.CodeInvalidArgument, "workspace.page", "page limit exceeds maximum")
	}
	return limit, nil
}

func summarizeTree(states []agentprocess.State) []ProcessSummary {
	children := make(map[id.AgentID]int, len(states))
	for _, state := range states {
		if state.ParentAgentID != nil {
			children[*state.ParentAgentID]++
		}
	}
	out := make([]ProcessSummary, 0, len(states))
	for _, state := range states {
		summary := ProcessSummary{
			AgentID: state.AgentID, RootAgentID: state.RootAgentID, ParentAgentID: state.ParentAgentID,
			Depth: state.LineageDepth, Status: state.Status, Version: state.Version,
			UpdatedAt: state.UpdatedAt, CreatedAt: state.CreatedAt, Children: children[state.AgentID],
		}
		if state.RootIntent != nil {
			summary.Goal = state.RootIntent.Goal
		}
		if state.Wait != nil {
			summary.WaitingReason = string(state.Wait.Reason)
			summary.WaitingRef = state.Wait.Reference
		}
		if state.Failure != nil {
			summary.Failure = state.Failure.Code + ": " + state.Failure.Message
		}
		out = append(out, summary)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Depth != out[j].Depth {
			return out[i].Depth < out[j].Depth
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func blockFromEvent(event ledger.Event) Block {
	kind := classifyEvent(string(event.Type), event.Actor.Kind)
	preview, objectRef := eventPreview(event.Payload)
	if preview == "" {
		preview = strings.ReplaceAll(string(event.Type), "_", " ")
	}
	return Block{
		ID: string(event.ID), AgentID: event.AgentID, Sequence: event.LedgerSequence,
		ProcessVersion: event.ProcessVersion, Kind: kind, Title: string(event.Type), Preview: preview,
		ObjectRef: objectRef, EventType: string(event.Type), Payload: append(json.RawMessage(nil), event.Payload...),
		OccurredAt: event.Timestamp, Critical: criticalEvent(string(event.Type)),
	}
}

func classifyEvent(eventType string, actor ledger.ActorKind) BlockKind {
	lower := strings.ToLower(eventType)
	switch {
	case strings.Contains(lower, "contextfault") || strings.Contains(lower, "context_fault"):
		return BlockContextFault
	case strings.Contains(lower, "approval"):
		return BlockApproval
	case strings.Contains(lower, "authority") || strings.Contains(lower, "denied") || strings.Contains(lower, "security"):
		return BlockSecurityDecision
	case strings.Contains(lower, "transaction"):
		return BlockTransaction
	case strings.Contains(lower, "fork"):
		return BlockForkComparison
	case strings.Contains(lower, "checkpoint"):
		return BlockCheckpoint
	case strings.Contains(lower, "syscallrequested"):
		return BlockToolCall
	case strings.Contains(lower, "syscallcompleted"):
		return BlockToolResult
	case strings.Contains(lower, "syscallfailed") || strings.Contains(lower, "failed"):
		return BlockError
	case strings.Contains(lower, "modelinvocationcompleted"):
		return BlockAssistantMessage
	case strings.Contains(lower, "modelinvocation") || strings.Contains(lower, "routing"):
		return BlockProgress
	case strings.Contains(lower, "spawn") || strings.Contains(lower, "child"):
		return BlockChildAgent
	case strings.Contains(lower, "plan"):
		return BlockPlan
	case strings.Contains(lower, "diff"):
		return BlockDiff
	case strings.Contains(lower, "test") || strings.Contains(lower, "verification"):
		return BlockTestResult
	case actor == ledger.ActorUser || strings.Contains(lower, "intentbound"):
		return BlockUserMessage
	default:
		return BlockSystem
	}
}

func criticalEvent(eventType string) bool {
	lower := strings.ToLower(eventType)
	return strings.Contains(lower, "failed") || strings.Contains(lower, "denied") ||
		strings.Contains(lower, "reconciliation") || strings.Contains(lower, "approval")
}

func eventPreview(payload json.RawMessage) (string, string) {
	if len(payload) == 0 {
		return "", ""
	}
	var fields map[string]any
	if json.Unmarshal(payload, &fields) != nil {
		return truncateUTF8(string(payload), 240), ""
	}
	objectRef := firstString(fields, "response_ref", "result_ref", "summary_ref", "decision_ref", "arguments_ref", "object_ref")
	parts := make([]string, 0, 4)
	for _, key := range []string{"goal", "reason", "failure", "name", "status", "model", "provider", "reference", "scope", "action"} {
		if value := valueString(fields[key]); value != "" {
			parts = append(parts, value)
		}
		if len(parts) == 3 {
			break
		}
	}
	if len(parts) == 0 && objectRef != "" {
		parts = append(parts, objectRef)
	}
	if len(parts) == 0 {
		body, _ := json.Marshal(fields)
		parts = append(parts, string(body))
	}
	return truncateUTF8(strings.Join(parts, " · "), 240), objectRef
}

func firstString(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := valueString(fields[key]); value != "" {
			return value
		}
	}
	return ""
}

func valueString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64, bool:
		return fmt.Sprint(typed)
	case map[string]any:
		for _, key := range []string{"message", "code", "goal", "reason", "id"} {
			if nested := valueString(typed[key]); nested != "" {
				return nested
			}
		}
	return ""
}

func truncateUTF8(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return strings.TrimSpace(value[:limit]) + "…"
}
