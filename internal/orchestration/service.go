package orchestration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"

	"github.com/Methamorphe/go-agent/internal/clock"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
	"github.com/Methamorphe/go-agent/internal/process"
)

var ErrWaitCycleDetected = errors.New("wait cycle detected")

const defaultResultInlineBytes = 4 << 10

type IDGenerator interface {
	Spawn() (id.SpawnID, error)
	BudgetReservation() (id.BudgetReservationID, error)
	Message() (id.MessageID, error)
	Wait() (id.WaitID, error)
	Request() (id.RequestID, error)
	Correlation() (id.CorrelationID, error)
}

type ProcessAPI interface {
	Inspect(context.Context, id.AgentID) (process.State, error)
	CreateDelegatedChild(context.Context, id.AgentID, string, process.CommandMeta) (process.State, error)
	Wait(context.Context, id.AgentID, *uint64, process.WaitingReason, string, process.CommandMeta) (process.State, error)
	SatisfyWait(context.Context, id.AgentID, *uint64, string, process.CommandMeta) (process.State, error)
	Cancel(context.Context, id.AgentID, *uint64, string, string, process.CommandMeta) (process.State, error)
}

type Service struct { store Store; processes ProcessAPI; ids IDGenerator; clock clock.Clock }
type SpawnOutcome struct { Spawn SpawnRecord `json:"spawn"`; Child process.State `json:"child"`; Reused bool `json:"reused"`; Result *ChildResult `json:"result,omitempty"` }

func New(store Store, processes ProcessAPI, ids IDGenerator, source clock.Clock) (*Service,error) {
	if store==nil||processes==nil||ids==nil{return nil,errs.New(errs.CodeInvalidArgument,"orchestration.new","store, processes and ids are required")}
	if source==nil{source=clock.NewSystemClock()};return &Service{store:store,processes:processes,ids:ids,clock:source},nil
}

func (s *Service) BootstrapRoot(ctx context.Context,agentID id.AgentID,policy RootPolicy)error{
	if agentID==""{return errs.New(errs.CodeInvalidArgument,"orchestration.bootstrap","agent id is required")};if !policy.Budget.Valid(){return errs.New(errs.CodeInvalidArgument,"orchestration.bootstrap","budget contains negative value")}
	if policy.MaxTotalDescendants<=0{policy.MaxTotalDescendants=64};if policy.MaxChildrenPerNode<=0{policy.MaxChildrenPerNode=16};if policy.MaxDepth==0{policy.MaxDepth=4};if policy.MaxParallelism<=0{policy.MaxParallelism=4};if policy.MailboxMessages<=0{policy.MailboxMessages=128};if policy.MailboxBytes<=0{policy.MailboxBytes=1<<20}
	state,err:=s.processes.Inspect(ctx,agentID);if err!=nil{return err};if state.RootAgentID!=agentID{return errs.New(errs.CodeInvalidArgument,"orchestration.bootstrap","only a root process can be bootstrapped")};return s.store.BootstrapRoot(ctx,agentID,policy,s.clock.Now().UTC())
}

func (s *Service) Spawn(ctx context.Context,spec SpawnSpec)(SpawnOutcome,error){
	if spec.ParentAgentID==""||strings.TrimSpace(spec.TaskIntent)==""{return SpawnOutcome{},errs.New(errs.CodeInvalidArgument,"orchestration.spawn","parent and task intent are required")};if !spec.Budget.Valid()||spec.Budget.IsZero(){return SpawnOutcome{},errs.New(errs.CodeInvalidArgument,"orchestration.spawn","non-zero valid child budget is required")}
	if spec.Result.MaxInlineBytes<=0{spec.Result.MaxInlineBytes=defaultResultInlineBytes};if spec.Result.MaxInlineBytes>16<<10{return SpawnOutcome{},errs.New(errs.CodeInvalidArgument,"orchestration.spawn","result max inline bytes exceeds G5 bound")}
	parent,err:=s.processes.Inspect(ctx,spec.ParentAgentID);if err!=nil{return SpawnOutcome{},err};if parent.Status.Terminal(){return SpawnOutcome{},errs.New(errs.CodeConflict,"orchestration.spawn","terminal parent cannot spawn")};account,err:=s.store.Account(ctx,spec.ParentAgentID);if err!=nil{return SpawnOutcome{},err}
	if !authoritySubset(spec.Authority,account.Authority){return SpawnOutcome{},errs.New(errs.CodePermissionDenied,"orchestration.spawn","child authority is not a subset of parent authority")};if !spec.Budget.Fits(account.Available){return SpawnOutcome{},errs.New(errs.CodeResourceExhausted,"orchestration.spawn","child budget exceeds parent available budget")};if spec.Deadline!=nil&&!spec.Deadline.After(s.clock.Now()){return SpawnOutcome{},errs.New(errs.CodeInvalidArgument,"orchestration.spawn","deadline must be in the future")};if parent.LineageDepth+1>account.Policy.MaxDepth{return SpawnOutcome{},errs.New(errs.CodeResourceExhausted,"orchestration.spawn","maximum recursive depth reached")}
	children,err:=s.store.CountChildren(ctx,parent.AgentID);if err!=nil{return SpawnOutcome{},err};if children>=account.Policy.MaxChildrenPerNode{return SpawnOutcome{},errs.New(errs.CodeResourceExhausted,"orchestration.spawn","maximum children per node reached")};descendants,err:=s.store.CountDescendants(ctx,parent.RootAgentID);if err!=nil{return SpawnOutcome{},err};if descendants>=account.Policy.MaxTotalDescendants{return SpawnOutcome{},errs.New(errs.CodeResourceExhausted,"orchestration.spawn","maximum root descendants reached")}
	key:=taskKey(spec);if spec.ReuseCompleted&&!spec.IndependentVerification{if existing,ok,e:=s.store.FindReusableSpawn(ctx,parent.RootAgentID,key);e!=nil{return SpawnOutcome{},e}else if ok&&existing.ChildAgentID!=""{result,has,e:=s.store.Result(ctx,existing.ChildAgentID);if e!=nil{return SpawnOutcome{},e};if has{child,e:=s.processes.Inspect(ctx,existing.ChildAgentID);if e!=nil{return SpawnOutcome{},e};if child.Status==process.StatusCompleted{return SpawnOutcome{Spawn:existing,Child:child,Reused:true,Result:&result},nil}}}}
	spawnID,err:=s.ids.Spawn();if err!=nil{return SpawnOutcome{},errs.Wrap(errs.CodeInternal,"orchestration.spawn","generate spawn id",err)};reservationID,err:=s.ids.BudgetReservation();if err!=nil{return SpawnOutcome{},errs.Wrap(errs.CodeInternal,"orchestration.spawn","generate reservation id",err)};now:=s.clock.Now().UTC();record:=SpawnRecord{ID:spawnID,ParentAgentID:parent.AgentID,RootAgentID:parent.RootAgentID,Depth:parent.LineageDepth+1,TaskIntent:strings.TrimSpace(spec.TaskIntent),TaskKey:key,Authority:canonicalAuthority(spec.Authority),ReservationID:reservationID,Reserved:spec.Budget,Result:spec.Result,Deadline:spec.Deadline,Status:SpawnReserved,CreatedAt:now,UpdatedAt:now}
	if err:=s.store.ReserveSpawn(ctx,record);err!=nil{return SpawnOutcome{},err};meta,err:=s.commandMeta(parent.AgentID);if err!=nil{_ = s.store.RejectSpawn(ctx,spawnID,"id_generation_failed",now);return SpawnOutcome{},err};child,err:=s.processes.CreateDelegatedChild(ctx,parent.AgentID,spec.TaskIntent,meta);if err!=nil{_ = s.store.RejectSpawn(ctx,spawnID,"process_creation_failed",s.clock.Now().UTC());return SpawnOutcome{},err};if err:=s.store.BindSpawnChild(ctx,spawnID,child.AgentID,s.clock.Now().UTC());err!=nil{return SpawnOutcome{},err};if err:=s.store.Enqueue(ctx,Admission{AgentID:child.AgentID,RootID:child.RootAgentID,EnqueuedAt:s.clock.Now().UTC()});err!=nil{return SpawnOutcome{},err};record.ChildAgentID=child.AgentID;record.Status=SpawnCreated;record.UpdatedAt=s.clock.Now().UTC();return SpawnOutcome{Spawn:record,Child:child},nil
}

func (s *Service) Settle(ctx context.Context,spawnID id.SpawnID,actual Budget,result ChildResult)error{
	if spawnID==""||!actual.Valid(){return errs.New(errs.CodeInvalidArgument,"orchestration.settle","spawn id and valid actual usage are required")};record,err:=s.store.Spawn(ctx,spawnID);if err!=nil{return err};if !actual.Fits(record.Reserved){return errs.New(errs.CodeResourceExhausted,"orchestration.settle","actual usage exceeds reserved child budget")};if record.ChildAgentID==""{return errs.New(errs.CodeConflict,"orchestration.settle","spawn has no child")}
	child,err:=s.processes.Inspect(ctx,record.ChildAgentID);if err!=nil{return err};if !child.Status.Terminal(){return errs.New(errs.CodeConflict,"orchestration.settle","child must be terminal before settlement")};if record.Result.MaxInlineBytes>0&&len(result.Summary)>record.Result.MaxInlineBytes{return errs.New(errs.CodeResourceExhausted,"orchestration.settle","child result summary exceeds contract")};if record.Result.EvidenceRequired&&len(result.EvidenceRefs)==0{return errs.New(errs.CodeInvalidArgument,"orchestration.settle","child result contract requires evidence")}
	result.ChildAgentID=record.ChildAgentID;if result.CompletedAt.IsZero(){result.CompletedAt=s.clock.Now().UTC()};if err:=s.store.PutResult(ctx,result);err!=nil{return err};return s.store.SettleSpawn(ctx,spawnID,actual,s.clock.Now().UTC())
}

func (s *Service) Send(ctx context.Context,message AgentMessage)(AgentMessage,bool,error){if message.From==""||message.To==""{return AgentMessage{},false,errs.New(errs.CodeInvalidArgument,"orchestration.send","sender and receiver are required")};from,err:=s.processes.Inspect(ctx,message.From);if err!=nil{return AgentMessage{},false,err};to,err:=s.processes.Inspect(ctx,message.To);if err!=nil{return AgentMessage{},false,err};if from.RootAgentID!=to.RootAgentID{return AgentMessage{},false,errs.New(errs.CodePermissionDenied,"orchestration.send","cross-root messaging is denied")};if !related(from,to){return AgentMessage{},false,errs.New(errs.CodePermissionDenied,"orchestration.send","only parent/child or sibling messaging is allowed in G5")};account,err:=s.store.Account(ctx,message.To);if err!=nil{return AgentMessage{},false,err};if message.ID==""{message.ID,err=s.ids.Message();if err!=nil{return AgentMessage{},false,err}};if message.CreatedAt.IsZero(){message.CreatedAt=s.clock.Now().UTC()};if message.PayloadRef!=""&&message.Inline!=""{return AgentMessage{},false,errs.New(errs.CodeInvalidArgument,"orchestration.send","message must use inline content or payload ref, not both")};if len(message.Inline)>16<<10{return AgentMessage{},false,errs.New(errs.CodeResourceExhausted,"orchestration.send","large message must use object reference")};inserted,err:=s.store.PutMessage(ctx,message,account.Policy.MailboxMessages,account.Policy.MailboxBytes);return message,inserted,err}
func (s *Service) Mailbox(ctx context.Context,agentID id.AgentID,limit int)([]AgentMessage,error){if limit<=0{limit=64};if limit>256{limit=256};return s.store.Mailbox(ctx,agentID,limit)}
func (s *Service) Consume(ctx context.Context,agentID id.AgentID,messageID id.MessageID)(bool,error){return s.store.ConsumeMessage(ctx,agentID,messageID,s.clock.Now().UTC())}

func (s *Service) WaitFor(ctx context.Context,parentID id.AgentID,children []id.AgentID,mode WaitMode,quorum int,failure FailurePolicy)(WaitSpec,process.State,error){if parentID==""||len(children)==0{return WaitSpec{},process.State{},errs.New(errs.CodeInvalidArgument,"orchestration.wait","parent and children are required")};parent,err:=s.processes.Inspect(ctx,parentID);if err!=nil{return WaitSpec{},process.State{},err};unique:=uniqueAgents(children);for _,childID:=range unique{child,e:=s.processes.Inspect(ctx,childID);if e!=nil{return WaitSpec{},process.State{},e};if child.RootAgentID!=parent.RootAgentID{return WaitSpec{},process.State{},errs.New(errs.CodePermissionDenied,"orchestration.wait","cannot wait on process from another root")};cycle,e:=s.wouldCycle(ctx,parentID,childID,4096);if e!=nil{return WaitSpec{},process.State{},e};if cycle{return WaitSpec{},process.State{},errs.Wrap(errs.CodeConflict,"orchestration.wait","durable wait would create a cycle",ErrWaitCycleDetected)}};if mode==WaitQuorum&&(quorum<=0||quorum>len(unique)){return WaitSpec{},process.State{},errs.New(errs.CodeInvalidArgument,"orchestration.wait","invalid quorum")};waitID,err:=s.ids.Wait();if err!=nil{return WaitSpec{},process.State{},err};spec:=WaitSpec{ID:waitID,ParentID:parentID,Children:unique,Mode:mode,Quorum:quorum,Failure:failure,CreatedAt:s.clock.Now().UTC()};if err:=s.store.PutWait(ctx,spec);err!=nil{return WaitSpec{},process.State{},err};meta,err:=s.commandMeta(parentID);if err!=nil{_ = s.store.DeleteWait(ctx,waitID);return WaitSpec{},process.State{},err};expected:=parent.Version;next,err:=s.processes.Wait(ctx,parentID,&expected,process.WaitingChild,waitID.String(),meta);if err!=nil{_ = s.store.DeleteWait(ctx,waitID);return WaitSpec{},process.State{},err};return spec,next,nil}
func (s *Service) EvaluateWait(ctx context.Context,wait WaitSpec)(bool,error){succeeded,failed,terminal:=0,0,0;for _,childID:=range wait.Children{child,err:=s.processes.Inspect(ctx,childID);if err!=nil{return false,err};if !child.Status.Terminal(){continue};terminal++;if child.Status==process.StatusCompleted{succeeded++}else{failed++}};done:=false;switch wait.Mode{case WaitOne:done=terminal>0;case WaitAll:done=terminal==len(wait.Children)||(wait.Failure==FailureFailFast&&failed>0);case WaitFirstSuccess:done=succeeded>0||(wait.Failure==FailureFailFast&&failed>0)||terminal==len(wait.Children);case WaitQuorum:done=succeeded>=wait.Quorum||(wait.Failure==FailureFailFast&&failed>0)||succeeded+(len(wait.Children)-terminal)<wait.Quorum;default:return false,errs.New(errs.CodeInvalidArgument,"orchestration.evaluate_wait","unknown wait mode")};if !done{return false,nil};parent,err:=s.processes.Inspect(ctx,wait.ParentID);if err!=nil{return false,err};if parent.Status==process.StatusWaiting&&parent.Wait!=nil&&parent.Wait.Reference==wait.ID.String(){meta,e:=s.commandMeta(wait.ParentID);if e!=nil{return false,e};expected:=parent.Version;if _,e=s.processes.SatisfyWait(ctx,wait.ParentID,&expected,wait.ID.String(),meta);e!=nil{return false,e}};if err:=s.store.DeleteWait(ctx,wait.ID);err!=nil{return false,err};return true,nil}
func (s *Service) CancelTree(ctx context.Context,root id.AgentID,reason string)error{rootState,err:=s.processes.Inspect(ctx,root);if err!=nil{return err};descendants,err:=s.store.Descendants(ctx,root);if err!=nil{return err};descendants=append(descendants,root);for _,agentID:=range descendants{state,e:=s.processes.Inspect(ctx,agentID);if e!=nil{return e};if state.Status.Terminal()||state.Cancel.Requested{continue};meta,e:=s.commandMeta(rootState.AgentID);if e!=nil{return e};expected:=state.Version;if _,e=s.processes.Cancel(ctx,agentID,&expected,"descendants",reason,meta);e!=nil{return e}};return nil}
func (s *Service) AdmitFair(ctx context.Context,globalSlots int)([]Admission,error){if globalSlots<=0{return nil,nil};return s.store.DequeueFair(ctx,globalSlots,0)}
func (s *Service) wouldCycle(ctx context.Context,parent,child id.AgentID,limit int)(bool,error){if parent==child{return true,nil};seen:=map[id.AgentID]struct{}{child:{}};queue:=[]id.AgentID{child};for len(queue)>0{if len(seen)>limit{return false,errs.New(errs.CodeResourceExhausted,"orchestration.wait","wait graph traversal bound exceeded")};current:=queue[0];queue=queue[1:];next,err:=s.store.WaitEdges(ctx,current);if err!=nil{return false,err};for _,candidate:=range next{if candidate==parent{return true,nil};if _,ok:=seen[candidate];ok{continue};seen[candidate]=struct{}{};queue=append(queue,candidate)}};return false,nil}
func (s *Service) commandMeta(actor id.AgentID)(process.CommandMeta,error){req,err:=s.ids.Request();if err!=nil{return process.CommandMeta{},err};cor,err:=s.ids.Correlation();if err!=nil{return process.CommandMeta{},err};return process.CommandMeta{RequestID:req,CorrelationID:cor,Actor:ledger.ActorRef{Kind:ledger.ActorAgent,ID:actor.String()}},nil}
func authoritySubset(child,parent []CapabilityGrant)bool{for _,requested:=range child{matched:=false;for _,grant:=range parent{if requested.Domain==grant.Domain&&scopeSubset(requested.Scope,grant.Scope){matched=true;break}};if !matched{return false}};return true}
func scopeSubset(child,parent string)bool{child,parent=strings.TrimSpace(child),strings.TrimSpace(parent);if parent=="*"{return child!=""};if child==parent{return true};if parent==""||child==""{return false};return strings.HasPrefix(child,strings.TrimSuffix(parent,"/")+"/")}
func canonicalAuthority(in []CapabilityGrant)[]CapabilityGrant{out:=append([]CapabilityGrant(nil),in...);sort.Slice(out,func(i,j int)bool{if out[i].Domain!=out[j].Domain{return out[i].Domain<out[j].Domain};return out[i].Scope<out[j].Scope});return out}
func taskKey(spec SpawnSpec)string{var b strings.Builder;b.WriteString(strings.ToLower(strings.Join(strings.Fields(spec.TaskIntent)," ")));for _,grant:=range canonicalAuthority(spec.Authority){b.WriteByte('|');b.WriteString(grant.Domain);b.WriteByte(':');b.WriteString(grant.Scope)};sum:=sha256.Sum256([]byte(b.String()));return hex.EncodeToString(sum[:])}
func related(a,b process.State)bool{if a.ParentAgentID!=nil&&*a.ParentAgentID==b.AgentID{return true};if b.ParentAgentID!=nil&&*b.ParentAgentID==a.AgentID{return true};return a.ParentAgentID!=nil&&b.ParentAgentID!=nil&&*a.ParentAgentID==*b.ParentAgentID}
func uniqueAgents(in []id.AgentID)[]id.AgentID{seen:=make(map[id.AgentID]struct{},len(in));out:=make([]id.AgentID,0,len(in));for _,v:=range in{if v==""{continue};if _,ok:=seen[v];ok{continue};seen[v]=struct{}{};out=append(out,v)};sort.Slice(out,func(i,j int)bool{return out[i]<out[j]});return out}
