package world

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/id"
)

type spyWorld struct{ calls int }

func (s *spyWorld) Profile() Profile { return Profile{Name: "spy"} }
func (s *spyWorld) Execute(context.Context, Action) (Result, error) {
	s.calls++
	return Result{Status: ResultSucceeded}, nil
}

func TestDeniedActionNeverReachesWorld(t *testing.T) {
	root := t.TempDir()
	spy := &spyWorld{}
	var decisions int
	auth := NewAuthorizer(Intent{Version: 1, AllowedDomains: []string{"fs.read"}}, []Capability{{Domain: "fs.read", Scope: root}}, func(Action, AuthorizationDecision) { decisions++ })
	secure := NewSecureWorld(auth, spy, func() time.Time { return time.Unix(100, 0) })
	action := Action{ID: id.ActionID("act_1"), AgentID: id.AgentID("agt_1"), Kind: "fs.write_file", Purpose: "modify source", Resource: filepath.Join(root, "x.txt"), Effect: CanonicalEffect("fs.write_file")}
	result, err := secure.Execute(context.Background(), action)
	if !errors.Is(err, ErrDenied) || result.Status != ResultDenied {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if spy.calls != 0 {
		t.Fatalf("denied action reached world: calls=%d", spy.calls)
	}
	if decisions != 1 {
		t.Fatalf("security decision count=%d, want 1", decisions)
	}
}

func TestReadOnlyAgentCannotWriteEvenIfParamsRequestAuthority(t *testing.T) {
	root := t.TempDir()
	params := json.RawMessage(`{"content":"x","capability":{"domain":"fs.write","scope":"*"}}`)
	action := Action{ID: id.ActionID("act_2"), AgentID: id.AgentID("agt_2"), Kind: "fs.write_file", Purpose: "write requested by model", Resource: filepath.Join(root, "x.txt"), Params: params, Effect: CanonicalEffect("fs.write_file")}
	auth := NewAuthorizer(Intent{Version: 2}, []Capability{{Domain: "fs.read", Scope: root}}, nil)
	decision := auth.Authorize(action, time.Now())
	if decision.Allowed {
		t.Fatal("tool content minted write authority")
	}
}

func TestExpiredLeaseFailsDeterministically(t *testing.T) {
	root := t.TempDir()
	expires := time.Unix(50, 0)
	auth := NewAuthorizer(Intent{Version: 1}, []Capability{{Domain: "fs.read", Scope: root, ExpiresAt: &expires}}, nil)
	action := Action{Kind: "fs.read_file", Purpose: "inspect", Resource: filepath.Join(root, "go.mod"), Effect: CanonicalEffect("fs.read_file")}
	for i := 0; i < 2; i++ {
		decision := auth.Authorize(action, time.Unix(50, 0))
		if decision.Allowed || decision.Code != "capability_missing_or_expired" {
			t.Fatalf("decision=%+v", decision)
		}
	}
}

func TestChildCannotAcquireMissingParentCapability(t *testing.T) {
	root := t.TempDir()
	parent := []Capability{{Domain: "fs.read", Scope: root}}
	child := []Capability{{Domain: "fs.read", Scope: root}, {Domain: "process.exec", Scope: root}}
	if ValidateDelegation(parent, child, time.Now()) {
		t.Fatal("child acquired capability absent from parent")
	}
}

func TestDelegationScopeAndLeaseMustBeSubset(t *testing.T) {
	root := t.TempDir()
	parentExpiry := time.Now().Add(time.Minute)
	parent := []Capability{{Domain: "fs.read", Scope: root, ExpiresAt: &parentExpiry}}
	insideExpiry := time.Now().Add(30 * time.Second)
	inside := []Capability{{Domain: "fs.read", Scope: filepath.Join(root, "sub"), ExpiresAt: &insideExpiry}}
	if !ValidateDelegation(parent, inside, time.Now()) {
		t.Fatal("valid subset delegation rejected")
	}
	outside := []Capability{{Domain: "fs.read", Scope: filepath.Dir(root), ExpiresAt: &insideExpiry}}
	if ValidateDelegation(parent, outside, time.Now()) {
		t.Fatal("broader child scope accepted")
	}
	late := time.Now().Add(2 * time.Minute)
	tooLong := []Capability{{Domain: "fs.read", Scope: root, ExpiresAt: &late}}
	if ValidateDelegation(parent, tooLong, time.Now()) {
		t.Fatal("child lease outlived parent")
	}
}

func TestModelCannotDowngradeEffectClassification(t *testing.T) {
	root := t.TempDir()
	auth := NewAuthorizer(Intent{Version: 3}, []Capability{{Domain: "process.exec", Scope: root}}, nil)
	action := Action{Kind: "process.exec", Purpose: "run tests", Resource: root, Effect: Effect{Class: EffectRead, Idempotent: true, Retryable: true}}
	decision := auth.Authorize(action, time.Now())
	if decision.Allowed || decision.Code != "effect_mismatch" {
		t.Fatalf("decision=%+v", decision)
	}
	if got := CanonicalEffect("process.exec").Class; got != EffectCompensatable {
		t.Fatalf("canonical class=%s", got)
	}
}

func TestAllowedActionProducesActionProof(t *testing.T) {
	root := t.TempDir()
	auth := NewAuthorizer(Intent{Version: 7, AllowedDomains: []string{"fs.read"}}, []Capability{{Domain: "fs.read", Scope: root}}, nil)
	action := Action{Kind: "fs.read_file", Purpose: "satisfy acceptance criterion", Resource: filepath.Join(root, "README.md"), Effect: CanonicalEffect("fs.read_file")}
	decision := auth.Authorize(action, time.Now())
	if !decision.Allowed || decision.Proof == nil {
		t.Fatalf("decision=%+v", decision)
	}
	if decision.Proof.IntentVersion != 7 || decision.Proof.Purpose != action.Purpose || decision.Proof.CapabilityDomain != "fs.read" {
		t.Fatalf("proof=%+v", decision.Proof)
	}
}
