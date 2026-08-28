package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/mmu"
	"github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/provider"
)

type recallToolArgs struct {
	Text        string             `json:"text,omitempty"`
	Scopes      []mmu.Scope        `json:"scopes,omitempty"`
	Types       []mmu.PageType     `json:"types,omitempty"`
	Limit       int                `json:"limit,omitempty"`
	MaxTokens   int                `json:"max_tokens,omitempty"`
	ExplicitIDs []id.ContextPageID `json:"explicit_ids,omitempty"`
}

type recallToolResult struct {
	ActionID   id.ActionID           `json:"action_id"`
	OK         bool                  `json:"ok"`
	Kind       string                `json:"kind"`
	Selected   []mmu.RecallSelection `json:"selected,omitempty"`
	Unresolved []id.ContextPageID    `json:"unresolved,omitempty"`
	BudgetUsed int                   `json:"budget_used,omitempty"`
	Error      string                `json:"error,omitempty"`
}

func (r *G4Runner) callRecall(ctx context.Context, state process.State, call provider.ToolCall, meta process.CommandMeta, allowed []mmu.Scope) (recallToolResult, process.State, error) {
	actionID, err := r.ids.Action()
	if err != nil {
		return recallToolResult{}, state, errs.Wrap(errs.CodeInternal, "agent.g4.recall", "generate action id", err)
	}
	argumentsObject, err := r.objects.Put(ctx, bytes.NewReader(call.Arguments))
	if err != nil {
		return recallToolResult{}, state, err
	}
	expected := state.Version
	started, err := r.processes.SyscallRequested(ctx, state.AgentID, &expected, process.SyscallRequestedPayload{ActionID: actionID, Name: "recall", ArgumentsRef: string(argumentsObject.Ref)}, meta)
	if err != nil {
		return recallToolResult{}, state, err
	}

	var args recallToolArgs
	decodeErr := strictDecode(call.Arguments, &args)
	var recalled mmu.RecallResult
	if decodeErr == nil {
		recalled, decodeErr = r.memory.Recall(ctx, mmu.RecallRequest{AgentID: state.AgentID, AllowedScopes: allowed, Query: mmu.RecallQuery{Text: args.Text, Scopes: args.Scopes, Types: args.Types, Limit: args.Limit, MaxTokens: args.MaxTokens, ExplicitIDs: args.ExplicitIDs}})
	}
	if decodeErr != nil {
		message := truncateUTF8(decodeErr.Error(), 2048)
		result := recallToolResult{ActionID: actionID, OK: false, Kind: "recall", Error: message}
		expected = started.Version
		failed, recordErr := r.processes.SyscallFailed(ctx, state.AgentID, &expected, process.SyscallFailedPayload{ActionID: actionID, Name: "recall", Failure: message}, meta)
		if recordErr != nil {
			return result, started, errors.Join(decodeErr, recordErr)
		}
		return result, failed, nil
	}

	result := recallToolResult{ActionID: actionID, OK: true, Kind: "recall", Selected: recalled.Selected, Unresolved: recalled.Unresolved, BudgetUsed: recalled.BudgetUsed}
	body, err := json.Marshal(result)
	if err != nil {
		return recallToolResult{}, started, errs.Wrap(errs.CodeInternal, "agent.g4.recall", "encode recall result", err)
	}
	resultObject, err := r.objects.Put(ctx, bytes.NewReader(body))
	if err != nil {
		return recallToolResult{}, started, err
	}
	expected = started.Version
	completed, err := r.processes.SyscallCompleted(ctx, state.AgentID, &expected, process.SyscallCompletedPayload{ActionID: actionID, Name: "recall", Status: "known_succeeded", ResultRef: string(resultObject.Ref)}, meta)
	if err != nil {
		return result, started, err
	}
	return result, completed, nil
}

func recallToolDefinition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "recall",
		Description: "Explicitly retrieve durable cognitive pages that are missing from the current bounded working set. Selected pages are leased into the next model invocation.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"},"scopes":{"type":"array","items":{"type":"string","enum":["process","root_task","project","workspace","world"]}},"types":{"type":"array","items":{"type":"string","enum":["user_message","assistant_summary","source_excerpt","file_summary","tool_result_summary","decision","plan","child_report","error_trace","documentation","recall_result"]}},"limit":{"type":"integer","minimum":1,"maximum":256},"max_tokens":{"type":"integer","minimum":1},"explicit_ids":{"type":"array","items":{"type":"string"}}},"additionalProperties":false}`),
	}
}

func strictDecode(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errs.Wrap(errs.CodeInvalidArgument, "agent.g4.recall", "invalid recall arguments", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errs.New(errs.CodeInvalidArgument, "agent.g4.recall", "multiple JSON values are not allowed")
		}
		return errs.Wrap(errs.CodeInvalidArgument, "agent.g4.recall", "invalid trailing JSON", err)
	}
	return nil
}
