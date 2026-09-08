package world

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
)

type secureTransactionalSpy struct {
	calls int
}

func (s *secureTransactionalSpy) Profile() Profile {
	return Profile{
		Type:       TypeWorkspace,
		Filesystem: FilesystemGuarantees{MutationIsolation: true},
		Promotion:  PromotionGuarantees{Supported: true, ThreeWay: true, Reconciliation: true},
	}
}

func (s *secureTransactionalSpy) Execute(context.Context, Action) (Result, error) {
	s.calls++
	return Result{Status: ResultSucceeded}, nil
}

func (s *secureTransactionalSpy) BranchRef() BranchRef {
	return BranchRef{WorldID: id.WorldID("wld_secure_tx"), Type: TypeWorkspace, BaseIdentity: "base", Ref: "test"}
}

func (s *secureTransactionalSpy) PreparePromotion(context.Context, id.OperationID) (PromotionPlan, error) {
	return PromotionPlan{}, nil
}

func (s *secureTransactionalSpy) ApplyPromotion(context.Context, PromotionPlan) error { return nil }
func (s *secureTransactionalSpy) VerifyPromotion(context.Context, PromotionPlan) error { return nil }
func (s *secureTransactionalSpy) ReconcilePromotion(context.Context, PromotionPlan) (PromotionStatus, error) {
	return PromotionNotApplied, nil
}
func (s *secureTransactionalSpy) Rollback(context.Context) error { return nil }
func (s *secureTransactionalSpy) Finalize(context.Context) error { return nil }
func (s *secureTransactionalSpy) Close(context.Context) error    { return nil }

func TestSecureTransactionalWorldDeniesBeforeInnerWorld(t *testing.T) {
	inner := &secureTransactionalSpy{}
	auth := NewAuthorizer(
		Intent{Version: 9, AllowedDomains: []string{"fs.read"}},
		[]Capability{{Domain: "fs.read", Scope: "."}},
		nil,
	)
	secured := NewSecureTransactionalWorld(auth, inner, func() time.Time { return time.Unix(100, 0) })
	action := Action{
		ID:       id.ActionID("act_secure_tx_write"),
		AgentID:  id.AgentID("agt_secure_tx"),
		Kind:     "fs.write_file",
		Purpose:  "unauthorized speculative write",
		Resource: "x.txt",
		Effect:   CanonicalEffect("fs.write_file"),
	}
	result, err := secured.Execute(context.Background(), action)
	if !errors.Is(err, ErrDenied) || result.Status != ResultDenied {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if inner.calls != 0 {
		t.Fatalf("denied transactional action reached inner World: calls=%d", inner.calls)
	}
	if secured.BranchRef().WorldID != id.WorldID("wld_secure_tx") {
		t.Fatalf("branch reference was not preserved")
	}
}
