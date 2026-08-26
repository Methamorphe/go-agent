# Event Model, Ordering and Initial Catalog

## Purpose

The Event Ledger is the durable audit/recovery backbone of the runtime.

This document defines its semantics before schema implementation.

---

# 1. Event vs command vs stream chunk

## Command

A request to change or inspect state.

Commands can be rejected.

Examples:

```text
CreateAgent
RequestCancel
GrantCapability
StartModelInvocation
AuthorizeWorldAction
CommitTransaction
```

## Event

An immutable canonical fact that a state transition or significant outcome happened.

Examples:

```text
AgentCreated
CancelRequested
CapabilityGranted
ModelInvocationStarted
WorldActionAuthorized
TransactionCommitted
```

## Stream chunk

High-frequency transient data that is not individually canonical.

Examples:

- model text token/chunk;
- stdout bytes/lines;
- progress counters.

Chunks may be previewed live and assembled into durable objects, but they do not become one ledger row each.

---

# 2. Initial ordering decision

For local-first v0:

1. each Agent Process stream has an integer `process_version` incremented exactly once per accepted process-affecting canonical event;
2. ledger rows also receive a database/global monotonic `ledger_sequence` for local query order;
3. cross-process causality is represented with `causation_id`, `correlation_id`, lineage IDs and explicit references;
4. timestamps are metadata, never the correctness ordering source.

This gives:

```text
process_version
→ optimistic concurrency/state reconstruction

global ledger_sequence
→ operational local ordering/query convenience

causation graph
→ semantic cross-agent ordering
```

Distributed semantics can evolve later without changing AgentID/event IDs.

---

# 3. Event identity

Each event has an immutable globally unique `EventID`.

Conceptual envelope:

```go
type EventEnvelope struct {
    ID              EventID
    LedgerSequence  uint64
    AgentID         AgentID
    RootAgentID     AgentID
    ProcessVersion  uint64
    Type            EventType
    SchemaVersion   uint32
    Timestamp       time.Time
    CausationID     *EventID
    CorrelationID   CorrelationID
    Actor            ActorRef
    Payload         []byte
}
```

`ProcessVersion` can be omitted/treated differently for runtime-global events, but all Agent-affecting events use it.

---

# 4. Actor model

Events should identify the authority/source that caused a command where meaningful.

Candidate actors:

```text
UserActor
AgentActor
SupervisorActor
SchedulerActor
PolicyActor
SystemActor
ExternalActor
```

An actor is not the same as a capability grant.

Security-sensitive events additionally reference the capability/policy/approval that authorized them.

---

# 5. Correlation

A `CorrelationID` groups one logical chain of work.

Examples:

- one user turn;
- one root task;
- one transaction;
- one recovery operation.

Do not overload correlation with causation.

Causation should answer:

> Which prior event/accepted command directly caused this event?

---

# 6. Atomic append semantics

Canonical transition transaction should generally:

```text
validate current projection/version
append event(s)
update correctness-critical projections/reservations
commit SQLite transaction
return accepted state/version
```

External effects happen only according to operation protocol after required pre-execution transition is durable.

---

# 7. Event payload design

Payloads should be small and typed.

Prefer:

```text
summary metadata
IDs/references
hashes
status
usage numbers
```

rather than huge text/output bodies.

Large content uses `ObjectRef`.

Every payload type is versioned.

---

# 8. Event categories

## Process lifecycle

```text
AgentCreated
AgentInitialized
AgentReadied
AgentActivated
AgentYielded
AgentWaitStarted
AgentWaitSatisfied
AgentSleepStarted
AgentWoken
AgentSuspendRequested
AgentSuspended
AgentResumed
CancelRequested
AgentCancelled
AgentCompleted
AgentFailed
```

Not all of these must remain separate after implementation review; the rule is to preserve semantic facts needed for recovery/audit without creating noise.

## Intent

```text
RootIntentBound
ChildIntentBound
IntentAmendmentRequested
IntentAmendmentApproved
```

Root intent is never silently overwritten.

## Authority

```text
CapabilityGranted
CapabilityLeaseRenewed
CapabilityRevoked
CapabilityExpired
ActionAuthorizationGranted
ActionAuthorizationDenied
ApprovalRequested
ApprovalGranted
ApprovalDenied
ApprovalExpired
```

## Budget/economy

```text
BudgetCreated
BudgetReserved
BudgetReservationReleased
BudgetUsageSettled
BudgetLimitReached
```

## Model invocation

```text
ModelInvocationPlanned
ModelInvocationStarted
ModelInvocationInterrupted
ModelInvocationFailed
ModelInvocationCompleted
ModelFallbackSelected
```

The completed event references final response object and usage metadata.

## Syscall / World actions

```text
SyscallRequested
SyscallRejected
WorldActionAuthorized
WorldActionStarted
WorldActionOutcomeKnown
WorldActionOutcomeUnknown
WorldActionReconciled
```

Avoid duplicating redundant lifecycle events if syscall and action are 1:1 in implementation; keep conceptual distinction available.

## Context

```text
ContextPageCreated
ContextPageSuperseded
ContextPagePinned
ContextPageUnpinned
RecallRequested
RecallResolved
RecallFailed
ContextWorkingSetBuilt
ContextFaultDetected
ContextFaultResolved
```

Working-set event should reference a compact manifest/object rather than copy full prompt.

## Memory

```text
BeliefAdded
BeliefVerified
BeliefContested
BeliefMarkedStale
BeliefInvalidated
BeliefSuperseded
BeliefDependencyAdded
BeliefContradictionAdded
```

## Child/subagent

Child process creation already produces `AgentCreated` with parent lineage. Additional orchestration events:

```text
ChildSpawnRequested
ChildSpawnAccepted
ChildResultPublished
ChildMessagePublished
```

## Checkpoint/transaction/fork

```text
CheckpointCreated
TransactionStarted
TransactionVerificationStarted
TransactionVerificationCompleted
TransactionCommitStarted
TransactionCommitted
TransactionRollbackStarted
TransactionRolledBack
TransactionNeedsReconciliation
ForkCreated
ForkEvaluationCompleted
ForkPromoted
ForkDiscarded
```

## Runtime/recovery

```text
RuntimeStarted
RuntimeDraining
RuntimeStoppedCleanly
RecoveryStarted
InterruptedOperationDetected
RecoveryActionApplied
StorageHealthChanged
```

Runtime-global events may live in a separate stream/table domain if that simplifies process versioning.

---

# 9. Event granularity rule

Create an event when at least one is true:

1. it changes canonical state;
2. it is required to recover safely after crash;
3. it is required for authority/audit explanation;
4. it establishes a durable relationship/reference;
5. it records a terminal outcome or unknown-outcome boundary;
6. it is needed to reproduce a scheduling/policy decision that affects behavior.

Do **not** create canonical events merely because data moved through a stream.

---

# 10. Model stream persistence

Canonical model flow:

```text
ModelInvocationStarted
       │
       ├── live chunks → bounded presentation stream
       ├── tool-call assembly → invocation state
       └── response body → streaming object writer
       │
       ▼
object finalized
       │
       ▼
ModelInvocationCompleted(response_ref, usage, finish_reason)
```

If crash occurs before completion:

```text
Started without terminal event
→ recovery detects Interrupted
```

Optional intermediate recovery checkpoints may be added later without changing public semantics.

---

# 11. Tool stream persistence

Canonical World action flow:

```text
WorldActionAuthorized
WorldActionStarted
    │
    ├── stdout/stderr → object writers
    └── preview → presentation stream
    │
WorldActionOutcomeKnown(
  exit/result metadata,
  stdout_ref,
  stderr_ref,
  effect evidence
)
```

If remote mutation outcome is uncertain, use `WorldActionOutcomeUnknown` rather than known failure.

---

# 12. Projection consistency

Some projections can be updated in same transaction as event append:

```text
agent_processes
budget_accounts
pending_approvals
capability_leases
```

Others can be rebuildable asynchronous indexes if staleness is acceptable:

```text
search index
analytics
some UI aggregates
```

Each projection must document whether it is correctness-critical.

---

# 13. Idempotency

Command handlers need stable request/operation IDs where repeated submission is possible.

Example:

```text
client sends RequestCancel request_id=X
connection drops
client retries request_id=X
```

Runtime returns already accepted result rather than append duplicate cancel request.

Idempotency storage may map request ID → accepted event/result for bounded retention or permanently for correctness-sensitive operations.

External World idempotency is separately modeled and cannot be assumed.

---

# 14. Event schema evolution

Each event type has a payload version.

Rules:

- never decode old JSON/bytes directly into current structs without version handling;
- prefer additive compatible fields;
- use upcasters to canonical current in-memory representation;
- preserve raw historical event bytes;
- snapshot schema version evolves separately.

---

# 15. Event validation

Before append:

- type known;
- payload validates;
- expected process version matches;
- state transition legal;
- referenced root/parent IDs coherent;
- actor/authority requirements satisfied where applicable.

Reducer validation remains a second line against impossible history.

---

# 16. Replay modes

## Projection replay

Apply events without any external/model execution.

Default recovery/debug behavior.

## Historical inspection

Read events/objects as facts.

## Simulation branch

Replay to checkpoint/event, then start a **new branch** using fake/replaced model or World behavior.

Historical past is never mutated.

---

# 17. Retention

Canonical ledger events are long-lived by default.

High-volume telemetry/presentation frames can have separate retention.

Object GC must respect references from retained events/snapshots/beliefs/checkpoints.

Do not delete audit-critical events because a session is old.

---

# 18. Indexes needed early

Likely SQLite indexes:

```text
(agent_id, process_version)
ledger_sequence
correlation_id
causation_id
root_agent_id
(type, timestamp) only if measured/query-needed
```

Index choices should follow real query plans after prototype.

---

# 19. G1 minimum event catalog

Do not implement full future catalog in G1.

Minimum:

```text
AgentCreated
RootIntentBound
AgentReadied
AgentActivated
AgentYielded
AgentWaitStarted
AgentWaitSatisfied
AgentSleepStarted
AgentWoken
CancelRequested
AgentCancelled
AgentSuspended
AgentResumed
AgentCompleted
AgentFailed
CheckpointCreated
RecoveryActionApplied
```

Budget/capability/model/action events enter their milestones.

---

# 20. Required tests

```text
E-001 duplicate process version rejected
E-002 timestamps cannot reorder reducer semantics
E-003 snapshot + tail equals full replay projection
E-004 duplicate idempotent command does not duplicate event
E-005 payload upcaster reads historical fixture
E-006 large model/tool content never appears inline above configured event payload threshold
E-007 interrupted Started operation detected after restart
E-008 causal references survive parent/child creation
E-009 event append + correctness projection update are atomic
E-010 corrupted/impossible transition fails loudly rather than producing valid projection
```

---

# Decision baseline

The v0 architecture will use:

```text
process-local monotonic version
+
local global ledger sequence
+
explicit causal references
```

This is now the preferred A0 answer to open decision `O002`, subject only to implementation proof that SQLite transaction semantics support it cleanly.
