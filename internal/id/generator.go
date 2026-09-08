package id

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
)

type Generator struct{ random io.Reader }

func NewGenerator() *Generator                            { return &Generator{random: rand.Reader} }
func NewGeneratorWithReader(random io.Reader) *Generator { return &Generator{random: random} }

func (g *Generator) generate(prefix string) (string, error) {
	var bytes [16]byte
	if _, err := io.ReadFull(g.random, bytes[:]); err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	return prefix + "_" + hex.EncodeToString(bytes[:]), nil
}

func (g *Generator) Agent() (AgentID, error)                         { v, e := g.generate("agt"); return AgentID(v), e }
func (g *Generator) Event() (EventID, error)                         { v, e := g.generate("evt"); return EventID(v), e }
func (g *Generator) Invocation() (InvocationID, error)               { v, e := g.generate("inv"); return InvocationID(v), e }
func (g *Generator) Action() (ActionID, error)                       { v, e := g.generate("act"); return ActionID(v), e }
func (g *Generator) Transaction() (TransactionID, error)             { v, e := g.generate("tx"); return TransactionID(v), e }
func (g *Generator) World() (WorldID, error)                         { v, e := g.generate("wld"); return WorldID(v), e }
func (g *Generator) Operation() (OperationID, error)                 { v, e := g.generate("op"); return OperationID(v), e }
func (g *Generator) Verification() (VerificationID, error)           { v, e := g.generate("ver"); return VerificationID(v), e }
func (g *Generator) EffectRecord() (EffectRecordID, error)           { v, e := g.generate("efx"); return EffectRecordID(v), e }
func (g *Generator) Correlation() (CorrelationID, error)             { v, e := g.generate("cor"); return CorrelationID(v), e }
func (g *Generator) Request() (RequestID, error)                     { v, e := g.generate("req"); return RequestID(v), e }
func (g *Generator) Intent() (IntentID, error)                       { v, e := g.generate("int"); return IntentID(v), e }
func (g *Generator) Sleep() (SleepID, error)                         { v, e := g.generate("slp"); return SleepID(v), e }
func (g *Generator) Lease() (LeaseID, error)                         { v, e := g.generate("lse"); return LeaseID(v), e }
func (g *Generator) RuntimeInstance() (RuntimeInstanceID, error)     { v, e := g.generate("run"); return RuntimeInstanceID(v), e }
func (g *Generator) Snapshot() (SnapshotID, error)                   { v, e := g.generate("snp"); return SnapshotID(v), e }
func (g *Generator) Checkpoint() (CheckpointID, error)               { v, e := g.generate("chk"); return CheckpointID(v), e }
func (g *Generator) ContextPage() (ContextPageID, error)             { v, e := g.generate("ctx"); return ContextPageID(v), e }
func (g *Generator) Spawn() (SpawnID, error)                         { v, e := g.generate("spn"); return SpawnID(v), e }
func (g *Generator) Message() (MessageID, error)                     { v, e := g.generate("msg"); return MessageID(v), e }
func (g *Generator) BudgetReservation() (BudgetReservationID, error) { v, e := g.generate("bgt"); return BudgetReservationID(v), e }
func (g *Generator) Wait() (WaitID, error)                           { v, e := g.generate("wgt"); return WaitID(v), e }
func (g *Generator) ForkGroup() (ForkGroupID, error)                 { v, e := g.generate("fkg"); return ForkGroupID(v), e }
func (g *Generator) Fork() (ForkID, error)                           { v, e := g.generate("frk"); return ForkID(v), e }
func (g *Generator) Evidence() (EvidenceID, error)                   { v, e := g.generate("evd"); return EvidenceID(v), e }
func (g *Generator) Belief() (BeliefID, error)                       { v, e := g.generate("blf"); return BeliefID(v), e }
func (g *Generator) Propagation() (PropagationID, error)             { v, e := g.generate("prp"); return PropagationID(v), e }
