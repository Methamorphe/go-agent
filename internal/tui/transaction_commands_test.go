package tui

import (
	"context"
	"testing"

	controlapi "github.com/Methamorphe/go-agent/internal/control/api"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/transaction"
	"github.com/Methamorphe/go-agent/internal/workspace"
)

// Keep the shared fake satisfying runtimeClient as G13 gains explicit headless
// transaction operations.
func (f *fakeRuntimeClient) OperateTransaction(_ context.Context, request controlapi.WorkspaceTransactionOperateRequest) (controlapi.WorkspaceTransactionOperateResponse, error) {
	return controlapi.WorkspaceTransactionOperateResponse{Transaction: transaction.Transaction{ID: request.TransactionID}}, nil
}

type transactionRecordingClient struct {
	fakeRuntimeClient
	requests []controlapi.WorkspaceTransactionOperateRequest
}

func (f *transactionRecordingClient) OperateTransaction(_ context.Context, request controlapi.WorkspaceTransactionOperateRequest) (controlapi.WorkspaceTransactionOperateResponse, error) {
	f.requests = append(f.requests, request)
	return controlapi.WorkspaceTransactionOperateResponse{Transaction: transaction.Transaction{ID: request.TransactionID, State: transaction.StateCommitted}}, nil
}

func TestPaletteTransactionCommitUsesExplicitRuntimeOperation(t *testing.T) {
	client := &transactionRecordingClient{}
	model := NewModel(context.Background(), client, DefaultConfig())
	model.loading = false
	model.inspector.Transactions = []workspace.TransactionSummary{{ID: id.TransactionID("txn_selected"), State: string(transaction.StateReadyToCommit)}}

	updated, cmd := model.executePalette("tx commit")
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("transaction commit must dispatch a runtime operation")
	}
	message := cmd()
	result, ok := message.(transactionMsg)
	if !ok {
		t.Fatalf("transaction command returned %T", message)
	}
	if result.err != nil {
		t.Fatal(result.err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("requests=%d", len(client.requests))
	}
	request := client.requests[0]
	if request.TransactionID != "txn_selected" || request.Operation != controlapi.WorkspaceTransactionCommit {
		t.Fatalf("unexpected request: %#v", request)
	}
	if model.inspectorTab != inspectorTransactions {
		t.Fatal("transaction command must focus the transaction inspector")
	}
}

func TestPaletteTransactionVerifyBuildsBoundedCommandRequest(t *testing.T) {
	client := &transactionRecordingClient{}
	model := NewModel(context.Background(), client, DefaultConfig())
	model.loading = false
	model.inspector.Transactions = []workspace.TransactionSummary{{ID: id.TransactionID("txn_verify"), State: string(transaction.StateOpen)}}

	_, cmd := model.executePalette("tx verify go test ./...")
	if cmd == nil {
		t.Fatal("verify must dispatch")
	}
	_ = cmd()
	if len(client.requests) != 1 {
		t.Fatalf("requests=%d", len(client.requests))
	}
	request := client.requests[0]
	if request.TransactionID != "txn_verify" || request.Operation != controlapi.WorkspaceTransactionVerify {
		t.Fatalf("unexpected verify request: %#v", request)
	}
	if len(request.Command) != 3 || request.Command[0] != "go" || request.Command[1] != "test" || request.Command[2] != "./..." {
		t.Fatalf("unexpected command: %#v", request.Command)
	}
}

func TestConfigurableAliasCannotInventRuntimeOperation(t *testing.T) {
	client := &transactionRecordingClient{}
	cfg := DefaultConfig()
	cfg.CommandAliases = map[string]string{"ship": "tx commit"}
	model := NewModel(context.Background(), client, cfg)
	model.loading = false
	model.inspector.Transactions = []workspace.TransactionSummary{{ID: id.TransactionID("txn_alias"), State: string(transaction.StateReadyToCommit)}}

	_, cmd := model.executePalette("ship")
	if cmd == nil {
		t.Fatal("alias should map to the existing explicit command")
	}
	_ = cmd()
	if len(client.requests) != 1 || client.requests[0].Operation != controlapi.WorkspaceTransactionCommit {
		t.Fatalf("alias bypassed command registry: %#v", client.requests)
	}
}
