package sqlite

import (
	"context"
	"sort"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/orchestration"
)

func (s *Store) Enqueue(ctx context.Context, admission orchestration.Admission) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO orchestration_admission(agent_id,root_id,priority,enqueued_at,active) VALUES(?,?,?,?,0) ON CONFLICT(agent_id) DO NOTHING`, admission.AgentID.String(), admission.RootID.String(), admission.Priority, formatTime(admission.EnqueuedAt))
	return err
}

func (s *Store) DequeueFair(ctx context.Context, globalSlots int, perRootSlots int) ([]orchestration.Admission, error) {
	if globalSlots <= 0 { return nil, nil }
	rows, err := s.db.QueryContext(ctx, `SELECT agent_id,root_id,priority,enqueued_at FROM orchestration_admission WHERE active=0 ORDER BY priority DESC,enqueued_at,root_id,agent_id LIMIT ?`, globalSlots*64)
	if err != nil { return nil, err }
	var candidates []orchestration.Admission
	for rows.Next() { var a orchestration.Admission; var aid,rid,at string; if err:=rows.Scan(&aid,&rid,&a.Priority,&at);err!=nil{rows.Close();return nil,err};a.AgentID=id.AgentID(aid);a.RootID=id.AgentID(rid);a.EnqueuedAt,err=time.Parse(time.RFC3339Nano,at);if err!=nil{rows.Close();return nil,err};candidates=append(candidates,a) }
	if err:=rows.Close();err!=nil{return nil,err};if err:=rows.Err();err!=nil{return nil,err}

	counts:=map[id.AgentID]int{}
	activeRows,err:=s.db.QueryContext(ctx,`SELECT root_id,COUNT(*) FROM orchestration_admission WHERE active=1 GROUP BY root_id`);if err!=nil{return nil,err}
	for activeRows.Next(){var rid string;var n int;if err:=activeRows.Scan(&rid,&n);err!=nil{activeRows.Close();return nil,err};counts[id.AgentID(rid)]=n};if err:=activeRows.Close();err!=nil{return nil,err}

	byRoot:=map[id.AgentID][]orchestration.Admission{};var roots []id.AgentID
	for _,candidate:=range candidates{if _,ok:=byRoot[candidate.RootID];!ok{roots=append(roots,candidate.RootID)};byRoot[candidate.RootID]=append(byRoot[candidate.RootID],candidate)}
	sort.Slice(roots,func(i,j int)bool{l,r:=byRoot[roots[i]][0],byRoot[roots[j]][0];if l.Priority!=r.Priority{return l.Priority>r.Priority};if !l.EnqueuedAt.Equal(r.EnqueuedAt){return l.EnqueuedAt.Before(r.EnqueuedAt)};return roots[i]<roots[j]})

	caps:=map[id.AgentID]int{}
	for _,rootID:=range roots{account,err:=s.Account(ctx,rootID);if err!=nil{return nil,err};cap:=account.Policy.MaxParallelism;if cap<=0{cap=1};if perRootSlots>0&&perRootSlots<cap{cap=perRootSlots};caps[rootID]=cap}
	picked:=make([]orchestration.Admission,0,globalSlots);indices:=map[id.AgentID]int{}
	for len(picked)<globalSlots{progress:=false;for _,rootID:=range roots{if counts[rootID]>=caps[rootID]{continue};index:=indices[rootID];if index>=len(byRoot[rootID]){continue};picked=append(picked,byRoot[rootID][index]);indices[rootID]=index+1;counts[rootID]++;progress=true;if len(picked)==globalSlots{break}};if !progress{break}}

	tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return nil,err};defer tx.Rollback();now:=formatTime(time.Now().UTC())
	for _,a:=range picked{result,err:=tx.ExecContext(ctx,`UPDATE orchestration_admission SET active=1,admitted_at=? WHERE agent_id=? AND active=0`,now,a.AgentID.String());if err!=nil{return nil,err};n,_:=result.RowsAffected();if n!=1{return nil,errs.New(errs.CodeConflict,"sqlite.orchestration.admission","candidate was concurrently admitted")}}
	if err:=tx.Commit();err!=nil{return nil,err};return picked,nil
}

func (s *Store) MarkAdmitted(ctx context.Context,agentID id.AgentID,active bool,at time.Time)error{value:=0;if active{value=1};_,err:=s.db.ExecContext(ctx,`UPDATE orchestration_admission SET active=?,admitted_at=? WHERE agent_id=?`,value,formatTime(at),agentID.String());return err}
func (s *Store) ActiveByRoot(ctx context.Context,root id.AgentID)(int,error){var n int;err:=s.db.QueryRowContext(ctx,`SELECT COUNT(*) FROM orchestration_admission WHERE root_id=? AND active=1`,root.String()).Scan(&n);return n,err}
var _ orchestration.Store=(*Store)(nil)
