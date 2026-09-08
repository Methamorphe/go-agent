package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	controlapi "github.com/Methamorphe/go-agent/internal/control/api"
	"github.com/Methamorphe/go-agent/internal/id"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/transaction"
	"github.com/Methamorphe/go-agent/internal/world"
)

type g13TransactionStore interface {
	transaction.Store
}

type g13ProcessReader interface {
	Current(context.Context, id.AgentID) (agentprocess.State, error)
}

type g13TransactionOperator struct {
	store     g13TransactionStore
	processes g13ProcessReader
	manager   *transaction.Manager
	ids       *id.Generator
	now       func() time.Time
}

func newG13TransactionOperator(store g13TransactionStore, processes g13ProcessReader, ids *id.Generator, now func() time.Time) *g13TransactionOperator {
	if ids == nil {
		ids = id.NewGenerator()
	}
	if now == nil {
		now = time.Now
	}
	op := &g13TransactionOperator{store: store, processes: processes, ids: ids, now: now}
	op.manager = transaction.NewManager(store, ids, now)
	op.manager.SetCommitGuard(op.commitGuard)
	return op
}

// commitGuard revalidates the current daemon-owned operator boundary at the
// exact G7 PREPARE/COMMIT points. The TUI never supplies capabilities or skips
// this check. A cancelled/failed Agent or an invalid WorkspaceWorld reference
// fails closed.
func (o *g13TransactionOperator) commitGuard(ctx context.Context, tx transaction.Transaction) error {
	if o == nil || o.processes == nil {
		return fmt.Errorf("operator process policy unavailable")
	}
	state, err := o.processes.Current(ctx, tx.AgentID)
	if err != nil {
		return fmt.Errorf("read current Agent policy state: %w", err)
	}
	if state.Status == agentprocess.StatusFailed || state.Status == agentprocess.StatusCancelled || state.Cancel.Requested {
		return fmt.Errorf("Agent lifecycle no longer permits promotion: %s", state.Status)
	}
	if state.RootIntent == nil || strings.TrimSpace(state.RootIntent.Goal) == "" {
		return fmt.Errorf("current Agent intent is unavailable")
	}
	if _, _, err := world.WorkspaceTarget(tx.IsolatedWorldRef); err != nil {
		return fmt.Errorf("promotion target policy rejected: %w", err)
	}
	return nil
}

func (o *g13TransactionOperator) Operate(ctx context.Context, req controlapi.WorkspaceTransactionOperateRequest) (controlapi.WorkspaceTransactionOperateResponse, error) {
	if o == nil || o.store == nil || o.manager == nil {
		return controlapi.WorkspaceTransactionOperateResponse{}, fmt.Errorf("G13 transaction operator unavailable")
	}
	if req.TransactionID == "" {
		return controlapi.WorkspaceTransactionOperateResponse{}, fmt.Errorf("transaction id is required")
	}
	tx, err := o.store.Get(ctx, req.TransactionID)
	if err != nil {
		return controlapi.WorkspaceTransactionOperateResponse{}, err
	}
	branch, err := world.ReopenWorkspaceWorld(tx.IsolatedWorldRef)
	if err != nil {
		return controlapi.WorkspaceTransactionOperateResponse{}, fmt.Errorf("reopen transaction WorkspaceWorld: %w", err)
	}
	defer branch.Close(context.Background())

	response := controlapi.WorkspaceTransactionOperateResponse{Transaction: tx}
	switch req.Operation {
	case controlapi.WorkspaceTransactionVerify:
		verification, err := o.verify(ctx, tx, branch, req)
		if err != nil {
			return response, err
		}
		response.Verification = &verification
	case controlapi.WorkspaceTransactionPrepare:
		plan, err := o.manager.Prepare(ctx, tx.ID, branch)
		if err != nil {
			return response, err
		}
		response.Promotion = &plan
	case controlapi.WorkspaceTransactionCommit:
		committed, err := o.manager.Commit(ctx, tx.ID, branch)
		if err != nil {
			return response, err
		}
		response.Transaction = committed
		return response, nil
	case controlapi.WorkspaceTransactionRollback:
		rolledBack, err := o.manager.Rollback(ctx, tx.ID, branch)
		if err != nil {
			return response, err
		}
		response.Transaction = rolledBack
		return response, nil
	case controlapi.WorkspaceTransactionReconcile:
		reconciled, err := o.manager.Reconcile(ctx, tx.ID, branch)
		if err != nil {
			return response, err
		}
		response.Transaction = reconciled
		return response, nil
	case controlapi.WorkspaceTransactionResolve:
		if err := o.manager.ResolveEffect(ctx, tx.ID, transaction.EffectResolution{
			EffectID: req.EffectID,
			Certainty: req.Certainty,
			Evidence: strings.TrimSpace(req.Evidence),
		}); err != nil {
			return response, err
		}
	default:
		return response, fmt.Errorf("unsupported transaction operation %q", req.Operation)
	}

	updated, err := o.store.Get(ctx, tx.ID)
	if err != nil {
		return response, err
	}
	response.Transaction = updated
	return response, nil
}

func (o *g13TransactionOperator) verify(ctx context.Context, tx transaction.Transaction, branch world.TransactionalWorld, req controlapi.WorkspaceTransactionOperateRequest) (transaction.Verification, error) {
	if len(req.Command) == 0 || strings.TrimSpace(req.Command[0]) == "" {
		return transaction.Verification{}, fmt.Errorf("verification requires an executable command")
	}
	if len(req.Command) > 128 {
		return transaction.Verification{}, fmt.Errorf("verification command has too many arguments")
	}
	for _, arg := range req.Command {
		if len(arg) > 32<<10 {
			return transaction.Verification{}, fmt.Errorf("verification argument is too large")
		}
	}
	timeoutMS := req.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 120000
	}
	if timeoutMS > 600000 {
		return transaction.Verification{}, fmt.Errorf("verification timeout exceeds 10 minutes")
	}
	params, err := json.Marshal(map[string]any{
		"executable": req.Command[0],
		"args": req.Command[1:],
		"cwd": ".",
		"timeout_ms": timeoutMS,
	})
	if err != nil {
		return transaction.Verification{}, err
	}
	actionID, err := o.ids.Action()
	if err != nil {
		return transaction.Verification{}, err
	}
	intentVersion := tx.Version
	authorizer := world.NewAuthorizer(world.Intent{
		Version: intentVersion,
		Goal: "verify isolated transaction before promotion",
		AllowedDomains: []string{"process.exec"},
		ForbiddenDomains: []string{"network", "fs.write"},
	}, []world.Capability{{Domain: "process.exec", Scope: "*"}}, nil)
	secure := world.NewSecureTransactionalWorld(authorizer, branch, o.now)
	check := transaction.CheckSpec{
		Name: "operator verification",
		Action: world.Action{
			ID: actionID,
			AgentID: tx.AgentID,
			Kind: "process.exec",
			Purpose: "verify isolated transaction before promotion",
			Params: params,
			Effect: world.CanonicalEffect("process.exec"),
		},
	}
	return o.manager.Verify(ctx, tx.ID, secure, []transaction.CheckSpec{check})
}
