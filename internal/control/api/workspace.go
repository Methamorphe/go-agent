package api

import "github.com/Methamorphe/go-agent/internal/workspace"

const (
	TypeWorkspaceAttach  = "workspace.attach"
	TypeWorkspaceHistory = "workspace.history"
	TypeWorkspaceSearch  = "workspace.search"
)

const (
	MessageWorkspaceAttach  = TypeWorkspaceAttach
	MessageWorkspaceHistory = TypeWorkspaceHistory
	MessageWorkspaceSearch  = TypeWorkspaceSearch
)

type WorkspaceAttachRequest = workspace.AttachRequest
type WorkspaceHistoryRequest = workspace.HistoryRequest
type WorkspaceSearchRequest = workspace.SearchRequest

type WorkspaceAttachResponse struct {
	Snapshot workspace.Snapshot `json:"snapshot"`
}

type WorkspaceHistoryResponse struct {
	Viewport workspace.ConversationViewport `json:"viewport"`
}

type WorkspaceSearchResponse struct {
	Result workspace.SearchResult `json:"result"`
}
