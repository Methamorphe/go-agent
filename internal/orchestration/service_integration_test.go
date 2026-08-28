package orchestration_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/objectstore"
	"github.com/Methamorphe/go-agent/internal/orchestration"
	"github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/storage/sqlite"
)

type harness struct {
	ctx context.Context
	store *sqlite.Store
	ids *id.Generator
	clock *clock.FakeClock
	processes *process.Service
	orchestrator *orchestration.Service
	root process.State
}

func newHarness(t *testing.T, policy orchestration.RootPolicy) *harness {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 8, 28, 21, 0, 0, 0, time.UTC)
	clk := clock.NewFakeClock(now)
	store, err := sqlite.Open(ctx, sqlite.Config{Path: t.TempDir()+"/runtime.db", Clock: clk})
	if err != nil { t.Fatal(err) }
	t.Cleanup(func(){ _ = store.Close() })
	ids := id.NewGenerator()
	processes := process.NewService(store, ids, clk)
	root, err := processes.CreateRoot(ctx, "root task", process.CommandMeta{})
	if err != nil { t.Fatal(err) }
	orchestrator, err := orchestration.New(store, processes, ids, clk)
	if err != nil { t.Fatal(err) }
	if err := orchestrator.BootstrapRoot(ctx, root.AgentID, policy); err != nil { t.Fatal(err) }
	return &harness{ctx:ctx, store:store, ids:ids, clock:clk, processes:processes, orchestrator:orchestrator, root:root}
}

func defaultPolicy() orchestration.RootPolicy {
	return orchestration.RootPolicy{
		Authority: []orchestration.CapabilityGrant{{Domain:"filesystem.read",Scope:"workspace"},{Domain:"process.execute",Scope:"workspace"}},
		Budget: orchestration.Budget{Tokens:100000,ToolCalls:1000,StorageBytes:1<<30,WorldMillis:1000000,MoneyMicros:10000000},
		MaxTotalDescendants: 2000, MaxChildrenPerNode: 2000, MaxDepth: 8, MaxParallelism: 4, MailboxMessages: 8, MailboxBytes: 1024,
	}
}

func spawn(t *testing.T, h *harness, parent id.AgentID, task string) orchestration.SpawnOutcome {
	t.Helper()
	out, err := h.orchestrator.Spawn(h.ctx, orchestration.SpawnSpec{ParentAgentID:parent,TaskIntent:task,Authority:[]orchestration.CapabilityGrant{{Domain:"filesystem.read",Scope:"workspace/src"}},Budget:orchestration.Budget{Tokens:1000,ToolCalls:10,StorageBytes:1024,WorldMillis:1000,MoneyMicros:1000}})
	if err != nil { t.Fatal(err) }
	return out
}

func TestORCH001ChildAuthorityIsSubset(t *testing.T) {
	h := newHarness(t, defaultPolicy())
	_, err := h.orchestrator.Spawn(h.ctx, orchestration.SpawnSpec{ParentAgentID:h.root.AgentID,TaskIntent:"escape",Authority:[]orchestration.CapabilityGrant{{Domain:"filesystem.write",Scope:"workspace"}},Budget:orchestration.Budget{Tokens:1}})
	if !errs.IsCode(err, errs.CodePermissionDenied) { t.Fatalf("expected permission denied, got %v", err) }
}

func TestORCH002BudgetReservedBeforeChildReady(t *testing.T) {
	h := newHarness(t, defaultPolicy())
	before, _ := h.store.Account(h.ctx, h.root.AgentID)
	out := spawn(t,h,h.root.AgentID,"inspect auth")
	if out.Child.Status != process.StatusReady { t.Fatalf("child status = %s", out.Child.Status) }
	after, _ := h.store.Account(h.ctx, h.root.AgentID)
	if after.Available.Tokens != before.Available.Tokens-1000 || after.Reserved.Tokens != 1000 { t.Fatalf("reservation not durable before READY: before=%+v after=%+v", before, after) }
}

func TestORCH003ThousandSleepingChildrenDoNotCreateResidentGoroutines(t *testing.T) {
	policy := defaultPolicy(); policy.MaxTotalDescendants=1200; policy.MaxChildrenPerNode=1200
	h := newHarness(t, policy)
	baseline := runtime.NumGoroutine()
	for i:=0;i<1000;i++ {
		child, err := h.processes.CreateDelegatedChild(h.ctx,h.root.AgentID,"sleeping child",process.CommandMeta{})
		if err != nil { t.Fatal(err) }
		expected:=child.Version
		if _,err:=h.processes.Sleep(h.ctx,child.AgentID,&expected,h.clock.Now().Add(time.Hour),process.CommandMeta{});err!=nil{t.Fatal(err)}
	}
	if delta:=runtime.NumGoroutine()-baseline;delta>20{t.Fatalf("1000 sleeping children created %d resident goroutines",delta)}
}

func TestORCH004MessageIDIdempotent(t *testing.T) {
	h:=newHarness(t,defaultPolicy()); child:=spawn(t,h,h.root.AgentID,"child")
	messageID, _ := h.ids.Message()
	msg:=orchestration.AgentMessage{ID:messageID,From:h.root.AgentID,To:child.Child.AgentID,Type:orchestration.MessageRequest,Inline:"hello"}
	_,inserted,err:=h.orchestrator.Send(h.ctx,msg);if err!=nil||!inserted{t.Fatalf("first send: inserted=%v err=%v",inserted,err)}
	_,inserted,err=h.orchestrator.Send(h.ctx,msg);if err!=nil||inserted{t.Fatalf("duplicate send: inserted=%v err=%v",inserted,err)}
}

func TestORCH005MailboxBackpressureBounded(t *testing.T) {
	policy:=defaultPolicy();policy.MailboxMessages=2;policy.MailboxBytes=32
	h:=newHarness(t,policy);child:=spawn(t,h,h.root.AgentID,"child")
	for i:=0;i<2;i++{if _,_,err:=h.orchestrator.Send(h.ctx,orchestration.AgentMessage{From:h.root.AgentID,To:child.Child.AgentID,Type:orchestration.MessageStatus,Inline:"12345678"});err!=nil{t.Fatal(err)}}
	_,_,err:=h.orchestrator.Send(h.ctx,orchestration.AgentMessage{From:h.root.AgentID,To:child.Child.AgentID,Type:orchestration.MessageStatus,Inline:"overflow"})
	if !errs.IsCode(err,errs.CodeResourceExhausted){t.Fatalf("expected mailbox backpressure, got %v",err)}
	messages,err:=h.orchestrator.Mailbox(h.ctx,child.Child.AgentID,100);if err!=nil{t.Fatal(err)};if len(messages)!=2{t.Fatalf("mailbox grew to %d",len(messages))}
}

func TestORCH006WaitCycleRejectedBeforeCommit(t *testing.T) {
	h:=newHarness(t,defaultPolicy());a:=spawn(t,h,h.root.AgentID,"a");b:=spawn(t,h,h.root.AgentID,"b")
	if _,_,err:=h.orchestrator.WaitFor(h.ctx,a.Child.AgentID,[]id.AgentID{b.Child.AgentID},orchestration.WaitAll,0,orchestration.FailureCollectAll);err!=nil{t.Fatal(err)}
	_,_,err:=h.orchestrator.WaitFor(h.ctx,b.Child.AgentID,[]id.AgentID{a.Child.AgentID},orchestration.WaitAll,0,orchestration.FailureCollectAll)
	if !errs.IsCode(err,errs.CodeConflict){t.Fatalf("expected wait cycle conflict, got %v",err)}
	edges,err:=h.store.WaitEdges(h.ctx,b.Child.AgentID);if err!=nil{t.Fatal(err)};if len(edges)!=0{t.Fatalf("cycle edge was committed: %v",edges)}
}

func TestORCH007CancelTreeIdempotent(t *testing.T) {
	h:=newHarness(t,defaultPolicy());a:=spawn(t,h,h.root.AgentID,"a");b:=spawn(t,h,a.Child.AgentID,"b")
	if err:=h.orchestrator.CancelTree(h.ctx,h.root.AgentID,"stop");err!=nil{t.Fatal(err)}
	if err:=h.orchestrator.CancelTree(h.ctx,h.root.AgentID,"stop again");err!=nil{t.Fatal(err)}
	for _,agentID:=range []id.AgentID{h.root.AgentID,a.Child.AgentID,b.Child.AgentID}{state,err:=h.processes.Inspect(h.ctx,agentID);if err!=nil{t.Fatal(err)};if state.Status!=process.StatusCancelled{t.Fatalf("%s status=%s",agentID,state.Status)}}
}

func TestORCH008LargeResultRemainsObjectReferenced(t *testing.T) {
	h:=newHarness(t,defaultPolicy());out:=spawn(t,h,h.root.AgentID,"large result")
	objects,err:=objectstore.New(t.TempDir()+"/objects");if err!=nil{t.Fatal(err)}
	meta,err:=objects.Put(h.ctx,stringsReader(makeString(128<<10)));if err!=nil{t.Fatal(err)}
	if err:=h.orchestrator.Settle(h.ctx,out.Spawn.ID,orchestration.Budget{Tokens:100},orchestration.ChildResult{Summary:"bounded summary",ArtifactRef:meta.Ref});err!=nil{t.Fatal(err)}
	result,ok,err:=h.store.Result(h.ctx,out.Child.AgentID);if err!=nil||!ok{t.Fatalf("result err=%v ok=%v",err,ok)};if result.ArtifactRef==""||len(result.Summary)>1024{t.Fatalf("result not object referenced: %+v",result)}
}

func TestORCH009FanoutLimitDeterministic(t *testing.T) {
	policy:=defaultPolicy();policy.MaxChildrenPerNode=2
	h:=newHarness(t,policy);spawn(t,h,h.root.AgentID,"one");spawn(t,h,h.root.AgentID,"two")
	_,err:=h.orchestrator.Spawn(h.ctx,orchestration.SpawnSpec{ParentAgentID:h.root.AgentID,TaskIntent:"three",Authority:[]orchestration.CapabilityGrant{{Domain:"filesystem.read",Scope:"workspace/src"}},Budget:orchestration.Budget{Tokens:1}})
	if !errs.IsCode(err,errs.CodeResourceExhausted){t.Fatalf("expected fanout limit, got %v",err)}
}

func TestORCH010ParentWaitSurvivesStoreReopen(t *testing.T) {
	ctx:=context.Background();dir:=t.TempDir();clk:=clock.NewFakeClock(time.Date(2026,8,28,21,0,0,0,time.UTC));ids:=id.NewGenerator()
	store,err:=sqlite.Open(ctx,sqlite.Config{Path:dir+"/runtime.db",Clock:clk});if err!=nil{t.Fatal(err)};processes:=process.NewService(store,ids,clk);root,err:=processes.CreateRoot(ctx,"root",process.CommandMeta{});if err!=nil{t.Fatal(err)};svc,_:=orchestration.New(store,processes,ids,clk);if err:=svc.BootstrapRoot(ctx,root.AgentID,defaultPolicy());err!=nil{t.Fatal(err)};out,err:=svc.Spawn(ctx,orchestration.SpawnSpec{ParentAgentID:root.AgentID,TaskIntent:"child",Authority:[]orchestration.CapabilityGrant{{Domain:"filesystem.read",Scope:"workspace/src"}},Budget:orchestration.Budget{Tokens:100}});if err!=nil{t.Fatal(err)};wait,_,err:=svc.WaitFor(ctx,root.AgentID,[]id.AgentID{out.Child.AgentID},orchestration.WaitAll,0,orchestration.FailureCollectAll);if err!=nil{t.Fatal(err)};_ = store.Close()
	store,err=sqlite.Open(ctx,sqlite.Config{Path:dir+"/runtime.db",Clock:clk});if err!=nil{t.Fatal(err)};defer store.Close();waits,err:=store.WaitsForChild(ctx,out.Child.AgentID);if err!=nil{t.Fatal(err)};if len(waits)!=1||waits[0].ID!=wait.ID{t.Fatalf("wait not recovered: %+v",waits)}
}

func TestORCH011FailFastWaitCompletesOnFailedChild(t *testing.T) {
	h:=newHarness(t,defaultPolicy());out:=spawn(t,h,h.root.AgentID,"child");wait,_,err:=h.orchestrator.WaitFor(h.ctx,h.root.AgentID,[]id.AgentID{out.Child.AgentID},orchestration.WaitAll,0,orchestration.FailureFailFast);if err!=nil{t.Fatal(err)}
	child,_:=h.processes.Inspect(h.ctx,out.Child.AgentID);expected:=child.Version;running,err:=h.processes.Activate(h.ctx,child.AgentID,id.RuntimeInstanceID("run_test"),&expected,process.CommandMeta{});if err!=nil{t.Fatal(err)};expected=running.Version;if _,err:=h.processes.Fail(h.ctx,child.AgentID,&expected,process.Failure{Code:"test",Message:"failed"},process.CommandMeta{});err!=nil{t.Fatal(err)}
	done,err:=h.orchestrator.EvaluateWait(h.ctx,wait);if err!=nil{t.Fatal(err)};if !done{t.Fatal("fail-fast wait did not complete")}
}

func TestORCH014FairAdmissionAcrossRoots(t *testing.T) {
	h1:=newHarness(t,defaultPolicy());h2:=newHarness(t,defaultPolicy())
	// Fairness is a property of a shared store, so bootstrap the second root into h1's store/process service.
	root2,err:=h1.processes.CreateRoot(h1.ctx,"root2",process.CommandMeta{});if err!=nil{t.Fatal(err)};if err:=h1.orchestrator.BootstrapRoot(h1.ctx,root2.AgentID,defaultPolicy());err!=nil{t.Fatal(err)}
	for i:=0;i<3;i++{spawn(t,h1,h1.root.AgentID,"storm")};spawn(t,h1,root2.AgentID,"other root")
	admitted,err:=h1.orchestrator.AdmitFair(h1.ctx,2);if err!=nil{t.Fatal(err)};if len(admitted)!=2{t.Fatalf("admitted=%d",len(admitted))};if admitted[0].RootID==admitted[1].RootID{t.Fatalf("one root starved the other: %+v",admitted)}
	_ = h2
}

func TestORCH015CompletedDuplicateReuseIsExplicit(t *testing.T) {
	h:=newHarness(t,defaultPolicy());spec:=orchestration.SpawnSpec{ParentAgentID:h.root.AgentID,TaskIntent:"same task",Authority:[]orchestration.CapabilityGrant{{Domain:"filesystem.read",Scope:"workspace/src"}},Budget:orchestration.Budget{Tokens:1000},ReuseCompleted:true}
	first,err:=h.orchestrator.Spawn(h.ctx,spec);if err!=nil{t.Fatal(err)}
	child:=first.Child;expected:=child.Version;running,err:=h.processes.Activate(h.ctx,child.AgentID,id.RuntimeInstanceID("run_reuse"),&expected,process.CommandMeta{});if err!=nil{t.Fatal(err)};expected=running.Version;completed,err:=h.processes.Complete(h.ctx,child.AgentID,&expected,"result",process.CommandMeta{});if err!=nil{t.Fatal(err)};if completed.Status!=process.StatusCompleted{t.Fatal("child not completed")}
	if err:=h.orchestrator.Settle(h.ctx,first.Spawn.ID,orchestration.Budget{Tokens:100},orchestration.ChildResult{Summary:"done"});err!=nil{t.Fatal(err)}
	second,err:=h.orchestrator.Spawn(h.ctx,spec);if err!=nil{t.Fatal(err)};if !second.Reused||second.Child.AgentID!=first.Child.AgentID{t.Fatalf("completed task not reused: %+v",second)}
	spec.IndependentVerification=true;third,err:=h.orchestrator.Spawn(h.ctx,spec);if err!=nil{t.Fatal(err)};if third.Reused||third.Child.AgentID==first.Child.AgentID{t.Fatal("independent verification was deduplicated")}
}

// Tiny helpers avoid adding test-only dependencies.
type stringReader struct{ s string; i int }
func stringsReader(s string)*stringReader{return &stringReader{s:s}}
func (r *stringReader)Read(p []byte)(int,error){if r.i>=len(r.s){return 0,io.EOF};n:=copy(p,r.s[r.i:]);r.i+=n;return n,nil}
func makeString(n int)string{b:=make([]byte,n);for i:=range b{b[i]='x'};return string(b)}
