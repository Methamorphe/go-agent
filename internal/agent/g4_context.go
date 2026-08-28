package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Methamorphe/go-agent/internal/agentsyscall"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/mmu"
	"github.com/Methamorphe/go-agent/internal/provider"
)

func (r *G4Runner) buildWorkingMessages(ctx context.Context, request RunRequest, tools []provider.ToolDefinition, mandatory []id.ContextPageID, allowed []mmu.Scope, recent []provider.Message, query string) ([]provider.Message, mmu.ContextManifest, error) {
	toolJSON, err := json.Marshal(tools)
	if err != nil {
		return nil, mmu.ContextManifest{}, errs.Wrap(errs.CodeInternal, "agent.g4.context", "encode tool schemas", err)
	}
	recentJSON, err := json.Marshal(recent)
	if err != nil {
		return nil, mmu.ContextManifest{}, errs.Wrap(errs.CodeInternal, "agent.g4.context", "encode active turn", err)
	}
	set, err := r.memory.Build(ctx, mmu.BuildRequest{
		AgentID: request.AgentID, Model: request.Model, AllowedScopes: allowed, MandatoryIDs: mandatory, Query: query,
		Budget: mmu.Budget{
			ModelMaxContext: r.cfg.ModelMaxContext, ReservedOutputTokens: r.cfg.ReservedOutputTokens,
			ProviderOverhead: r.cfg.ProviderOverhead, SafetyMargin: r.cfg.SafetyMargin,
			SchemaTokens: r.memory.EstimateText(request.Model, string(toolJSON)),
			ActiveTokens: r.memory.EstimateText(request.Model, string(recentJSON)) + g4ContextEnvelopeReserveTokens,
		},
	})
	if err != nil {
		return nil, mmu.ContextManifest{}, err
	}

	messages := make([]provider.Message, 0, len(recent)+3)
	var contextBuilder strings.Builder
	for _, page := range set.Pages {
		switch page.Meta.ID {
		case mandatory[0]:
			messages = append(messages, provider.Message{Role: provider.RoleSystem, Parts: []provider.ContentPart{provider.TextPart(page.Content)}})
		case mandatory[1]:
			messages = append(messages, provider.Message{Role: provider.RoleUser, Parts: []provider.ContentPart{provider.TextPart(page.Content)}})
		default:
			fmt.Fprintf(&contextBuilder, "[%s type=%s source=%s]\n%s\n\n", page.Meta.ID, page.Meta.Type, page.Meta.SourceRef, page.Content)
		}
	}
	if contextBuilder.Len() > 0 {
		messages = append(messages, provider.Message{Role: provider.RoleSystem, Parts: []provider.ContentPart{provider.TextPart("Durable cognitive context selected by the MMU:\n\n" + contextBuilder.String())}})
	}
	messages = append(messages, recent...)
	return messages, set.Manifest, nil
}

func (r *G4Runner) remember(ctx context.Context, agentID id.AgentID, pageType mmu.PageType, source, content string, importance float64) (mmu.PageMeta, error) {
	content = truncateUTF8(content, g4MaxAssistantMemoryBytes)
	return r.memory.CreatePage(ctx, mmu.PageInput{AgentID: agentID, Type: pageType, Scope: mmu.ScopeProcess, SourceRef: source, Content: content, Importance: importance, Confidence: 1})
}

func compactToolResult(result agentsyscall.Result) any {
	return struct {
		ActionID   id.ActionID `json:"action_id"`
		OK         bool        `json:"ok"`
		Kind       string      `json:"kind"`
		ContentRef string      `json:"content_ref,omitempty"`
		StdoutRef  string      `json:"stdout_ref,omitempty"`
		StderrRef  string      `json:"stderr_ref,omitempty"`
		Preview    string      `json:"preview,omitempty"`
		Stdout     string      `json:"stdout_preview,omitempty"`
		Stderr     string      `json:"stderr_preview,omitempty"`
		ExitCode   *int        `json:"exit_code,omitempty"`
		Checkpoint string      `json:"checkpoint_id,omitempty"`
		Error      string      `json:"error,omitempty"`
	}{
		ActionID: result.ActionID, OK: result.OK, Kind: result.Kind, ContentRef: result.ContentRef,
		StdoutRef: result.StdoutRef, StderrRef: result.StderrRef, Preview: truncateUTF8(result.Preview, g4MaxToolPreviewBytes),
		Stdout: truncateUTF8(result.Stdout, g4MaxToolPreviewBytes), Stderr: truncateUTF8(result.Stderr, g4MaxToolPreviewBytes),
		ExitCode: result.ExitCode, Checkpoint: result.Checkpoint, Error: truncateUTF8(result.Error, 2048),
	}
}

func truncateUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value
}

const g4SystemPrompt = `You are the G4 developer agent running inside a durable Agent Process with a Cognitive MMU.
The context window is a bounded working set, not canonical memory.
Use the provided syscalls when you need evidence from the workspace.
- observe reads files or lists directories.
- execute runs an executable with argv; do not assume shell expansion.
- checkpoint creates a durable process checkpoint marker.
- recall explicitly retrieves relevant durable context pages for the next invocation when important prior knowledge is missing.
Durable context pages are identified by ctx_* IDs. Prefer explicit IDs when a visible page is known.
Do not claim that a file, command, test, repository fact, or recalled fact was inspected unless a syscall or selected context page provides evidence for it.
When the task is complete, return the final answer as normal assistant text without a tool call.`
