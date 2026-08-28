package orchestration_test

import (
	"testing"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/orchestration"
)

func TestG5ResultContractRequiresEvidenceAndBoundsInlineSummary(t *testing.T) {
	h:=newHarness(t,defaultPolicy())
	out,err:=h.orchestrator.Spawn(h.ctx,orchestration.SpawnSpec{ParentAgentID:h.root.AgentID,TaskIntent:"verify",Authority:[]orchestration.CapabilityGrant{{Domain:"filesystem.read",Scope:"workspace/src"}},Budget:orchestration.Budget{Tokens:1000},Result:orchestration.ResultContract{EvidenceRequired:true,MaxInlineBytes:8}})
	if err!=nil{t.Fatal(err)}
	complete(t,h,out.Child.AgentID)
	if err:=h.orchestrator.Settle(h.ctx,out.Spawn.ID,orchestration.Budget{Tokens:10},orchestration.ChildResult{Summary:"short"});!errs.IsCode(err,errs.CodeInvalidArgument){t.Fatalf("missing evidence accepted: %v",err)}
	if err:=h.orchestrator.Settle(h.ctx,out.Spawn.ID,orchestration.Budget{Tokens:10},orchestration.ChildResult{Summary:"summary-too-long",EvidenceRefs:[]string{"object://evidence"}});!errs.IsCode(err,errs.CodeResourceExhausted){t.Fatalf("oversized inline result accepted: %v",err)}
	if err:=h.orchestrator.Settle(h.ctx,out.Spawn.ID,orchestration.Budget{Tokens:10},orchestration.ChildResult{Summary:"short",EvidenceRefs:[]string{"object://evidence"}});err!=nil{t.Fatal(err)}
}

func TestG5AdmissionHonorsRootMaxParallelism(t *testing.T) {
	policy:=defaultPolicy();policy.MaxParallelism=2
	h:=newHarness(t,policy)
	for i:=0;i<4;i++{spawn(t,h,h.root.AgentID,"parallel")}
	first,err:=h.orchestrator.AdmitFair(h.ctx,4);if err!=nil{t.Fatal(err)}
	if len(first)!=2{t.Fatalf("expected 2 admitted under root cap, got %d",len(first))}
	second,err:=h.orchestrator.AdmitFair(h.ctx,4);if err!=nil{t.Fatal(err)}
	if len(second)!=0{t.Fatalf("root exceeded persisted max parallelism: %+v",second)}
	if err:=h.store.MarkAdmitted(h.ctx,first[0].AgentID,false,h.clock.Now());err!=nil{t.Fatal(err)}
	third,err:=h.orchestrator.AdmitFair(h.ctx,4);if err!=nil{t.Fatal(err)}
	if len(third)!=1{t.Fatalf("released slot was not reused: %+v",third)}
}
