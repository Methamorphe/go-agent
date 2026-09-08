package tui

import (
	"context"
	"time"

	"github.com/Methamorphe/go-agent/internal/agent"
	"github.com/Methamorphe/go-agent/internal/control"
	controlapi "github.com/Methamorphe/go-agent/internal/control/api"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/workspace"
)

type Caller interface {
	Call(context.Context, string, any, any) error
}

type Client struct {
	caller  Caller
	timeout time.Duration
}

func NewClient(caller Caller) *Client {
	return &Client{caller: caller, timeout: 5 * time.Second}
}

func NewControlClient(client *control.Client) *Client {
	return NewClient(client)
}

func (c *Client) call(ctx context.Context, messageType string, request, response any) error {
	if c == nil || c.caller == nil {
		return context.Canceled
	}
	timeout := c.timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.caller.Call(requestCtx, messageType, request, response)
}

func (c *Client) Attach(ctx context.Context, request workspace.AttachRequest) (workspace.Snapshot, error) {
	var response controlapi.WorkspaceAttachResponse
	err := c.call(ctx, controlapi.MessageWorkspaceAttach, request, &response)
	return response.Snapshot, err
}

func (c *Client) Refresh(ctx context.Context, request workspace.RefreshRequest) (workspace.Refresh, error) {
	var response controlapi.WorkspaceRefreshResponse
	err := c.call(ctx, controlapi.MessageWorkspaceRefresh, request, &response)
	return response.Refresh, err
}

func (c *Client) History(ctx context.Context, request workspace.HistoryRequest) (workspace.ConversationViewport, error) {
	var response controlapi.WorkspaceHistoryResponse
	err := c.call(ctx, controlapi.MessageWorkspaceHistory, request, &response)
	return response.Viewport, err
}

func (c *Client) Search(ctx context.Context, request workspace.SearchRequest) (workspace.SearchResult, error) {
	var response controlapi.WorkspaceSearchResponse
	err := c.call(ctx, controlapi.MessageWorkspaceSearch, request, &response)
	return response.Result, err
}

func (c *Client) SendMessage(ctx context.Context, agentID id.AgentID, text string, queue agent.MessageQueue) (agent.SendMessageResult, error) {
	var response controlapi.AgentMessageResponse
	err := c.call(ctx, controlapi.MessageAgentMessage, controlapi.AgentMessageRequest{
		AgentID: agentID,
		Text: text,
		Queue: queue,
	}, &response)
	return response.Result, err
}
