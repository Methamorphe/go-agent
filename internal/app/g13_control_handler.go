package app

import (
	"context"

	"github.com/Methamorphe/go-agent/internal/control"
	controlapi "github.com/Methamorphe/go-agent/internal/control/api"
	"github.com/Methamorphe/go-agent/internal/control/protocol"
	"github.com/Methamorphe/go-agent/internal/workspace"
)

type g13WorkspaceCommands interface {
	Attach(context.Context, workspace.AttachRequest) (workspace.Snapshot, error)
	Refresh(context.Context, workspace.RefreshRequest) (workspace.Refresh, error)
	History(context.Context, workspace.HistoryRequest) (workspace.ConversationViewport, error)
	Search(context.Context, workspace.SearchRequest) (workspace.SearchResult, error)
}

type g13TransactionCommands interface {
	Operate(context.Context, controlapi.WorkspaceTransactionOperateRequest) (controlapi.WorkspaceTransactionOperateResponse, error)
}

func newG13ControlHandler(base control.Handler, workspaces g13WorkspaceCommands, transactions g13TransactionCommands) control.Handler {
	return control.HandlerFunc(func(ctx context.Context, request protocol.Envelope) (protocol.Envelope, error) {
		switch request.Type {
		case controlapi.TypeWorkspaceAttach:
			payload, err := decodeControlPayload[controlapi.WorkspaceAttachRequest](request.Payload)
			if err != nil {
				return protocol.Envelope{}, err
			}
			snapshot, err := workspaces.Attach(ctx, payload)
			if err != nil {
				return protocol.Envelope{}, err
			}
			return response(request, controlapi.WorkspaceAttachResponse{Snapshot: snapshot})

		case controlapi.TypeWorkspaceRefresh:
			payload, err := decodeControlPayload[controlapi.WorkspaceRefreshRequest](request.Payload)
			if err != nil {
				return protocol.Envelope{}, err
			}
			refresh, err := workspaces.Refresh(ctx, payload)
			if err != nil {
				return protocol.Envelope{}, err
			}
			return response(request, controlapi.WorkspaceRefreshResponse{Refresh: refresh})

		case controlapi.TypeWorkspaceHistory:
			payload, err := decodeControlPayload[controlapi.WorkspaceHistoryRequest](request.Payload)
			if err != nil {
				return protocol.Envelope{}, err
			}
			viewport, err := workspaces.History(ctx, payload)
			if err != nil {
				return protocol.Envelope{}, err
			}
			return response(request, controlapi.WorkspaceHistoryResponse{Viewport: viewport})

		case controlapi.TypeWorkspaceSearch:
			payload, err := decodeControlPayload[controlapi.WorkspaceSearchRequest](request.Payload)
			if err != nil {
				return protocol.Envelope{}, err
			}
			result, err := workspaces.Search(ctx, payload)
			if err != nil {
				return protocol.Envelope{}, err
			}
			return response(request, controlapi.WorkspaceSearchResponse{Result: result})

		case controlapi.TypeWorkspaceTransactionOperate:
			payload, err := decodeControlPayload[controlapi.WorkspaceTransactionOperateRequest](request.Payload)
			if err != nil {
				return protocol.Envelope{}, err
			}
			result, err := transactions.Operate(ctx, payload)
			if err != nil {
				return protocol.Envelope{}, err
			}
			return response(request, result)

		default:
			return base.Handle(ctx, request)
		}
	})
}
