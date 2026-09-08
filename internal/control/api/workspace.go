package api

import "github.com/Methamorphe/go-agent/internal/workspace"

const (
	TypeWorkspaceAttach  = "workspace.attach"
	TypeWorkspaceRefresh = "workspace.refresh"
	TypeWorkspaceHistory = "workspace.history"
	TypeWorkspaceSearch  = "workspace.search"
)

const (
	MessageWorkspaceAttach  = TypeWorkspaceAttach
	MessageWorkspaceRefresh = TypeWorkspaceRefresh
	MessageWorkspaceHistory = TypeWorkspaceHistory
	MessageWorkspaceSearch  = TypeWorkspaceSearch
)

type WorkspaceAttachRequest = workspace.AttachRequest
type WorkspaceRefreshRequest = workspace.RefreshRequest
type WorkspaceHistoryRequest = workspace.HistoryRequest
type WorkspaceSearchRequest = workspace.SearchRequest

type WorkspaceAttachResponse struct {
	Snapshot workspace.Snapshot `json:"snapshot"`
}

type WorkspaceRefreshResponse struct {
	Refresh workspace.Refresh `json:"refresh"`
}

type WorkspaceHistoryResponse struct {
	Viewport workspace.ConversationViewport `json:"viewport"`
}

type WorkspaceSearchResponse struct {
	Result workspace.SearchResult `json:"result"`
}
