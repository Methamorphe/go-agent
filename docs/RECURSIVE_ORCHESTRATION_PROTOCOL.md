# Recursive Orchestration Protocol

## Status

**A0 architecture contract — ACCEPTED semantic baseline.**

This document defines how durable Agent Processes delegate, communicate, wait and converge without turning recursive agents into an uncontrolled prompt swarm.

Core rule:

> **A child agent is a durable bounded process with a contract. `spawn()` is resource allocation + authority delegation + task creation, not “start another prompt”.**

---

# 1. Child contract

A child is created from an immutable/versioned spawn request.

```go
type SpawnSpec struct {
    ParentAgentID   AgentID
    TaskIntent      TaskIntentSpec
    Authority       []CapabilitySpec
    Budget          BudgetAllocation
    ModelPolicy     ModelPolicyRef
    WorldPolicy     WorldPolicyRef
    ResultContract  ResultContract
    Deadline        *time.Time
    ChildPolicy     ChildPolicy
}
```

Creation succeeds only after:

```text
typed intent subset validation
capability subset validation
budget reservation
parallelism admission
World policy validation
child-count/depth validation
spawn event/process row durability
```

Execution scheduling happens after durable creation.

---

# 2. Spawn state machine

```text
REQUESTED
  ↓
VALIDATING
  ├─→ REJECTED
  ↓
RESERVING
  ├─→ REJECTED
  ↓
CREATING
  ↓
CREATED
  ↓
READY
```

No child is considered created merely because a goroutine was launched.

---

# 3. Child lineage

Persist:

```text
root_agent_id
parent_agent_id
generation/depth
spawn_request_id
root_intent_id
parent_task_intent_id
fork lineage separately if applicable
```

This enables:

- budget ancestry;
- capability ancestry;
- cancellation propagation;
- causal trace;
- team visualization.

---

# 4. Budget reservation

Before child creation:

```text
parent available budget
       ↓ reserve
child budget allocation
       ↓
parent available decreases immediately
```

On child terminal state:

```text
actual usage settled
unused reservation released exactly once
```

Dimensions can include:

```text
money
tokens
wall-clock deadline
model concurrency slots
child count
storage quota
tool-call quota
World compute quota
```

No recursive oversubscription by promise.

---

# 5. Parallelism admission

`MaxChildren` and `MaxParallelism` are different.

Example:

```text
10 durable children exist
3 may execute model/tool work concurrently
7 READY/WAITING
```

This keeps durable topology independent from active resource consumption.

Scheduler uses fair bounded admission queues.

---

# 6. Result contract

Children should not return arbitrary transcript dumps by default.

```go
type ResultContract struct {
    Kind             ResultKind
    EvidenceRequired bool
    ArtifactTypes    []ArtifactType
    MaxInlineTokens  int
    VerificationRef  *VerificationPolicyID
}
```

Candidate structured result:

```yaml
status: completed
summary: "refresh race reproduced"
findings:
  - claim: "stale refresh can overwrite newer token"
    status: verified
    evidence:
      - evidence://test/race-42
      - evidence://file/auth.go@abc#L80-L119
artifacts:
  - object://reproducer.patch
uncertainties: []
```

Large data stays object-referenced.

---

# 7. Parent waiting semantics

Parent can wait on:

```text
one child
all selected children
first successful child
quorum
condition over results
```

Wait is durable process state.

No blocked goroutine is required while parent waits for hours.

Example:

```text
WAITING_CHILDREN
  wait_spec_ref
  wake_generation
```

Child terminal/result events wake the parent through centralized scheduling.

---

# 8. Cancellation tree

Default:

```text
root cancellation
→ descendants cancel unless explicitly detached by policy
```

Cancellation is durable intent/state first, runtime `context.Context` second.

Active operations follow their effect/outcome cancellation semantics.

Child cancellation never rewrites already observed external effects.

---

# 9. Detached children

v0 recommendation:

> **No implicit detached child execution.**

A child cannot outlive root ownership merely because parent finishes.

Future detached services/watchers require explicit lifecycle authority/policy and resource owner.

This avoids orphan autonomous processes.

---

# 10. Agent messaging

Messages are durable logical objects when they affect task state.

```go
type AgentMessage struct {
    ID          MessageID
    From        AgentID
    To          AgentID
    Type        AgentMessageType
    PayloadRef  ObjectRefOrInline
    Evidence    []EvidenceRef
    Correlation CorrelationID
    CreatedAt   time.Time
}
```

Types:

```text
Request
Response
Evidence
Proposal
Challenge
Counterexample
Status
Result
Escalation
CancelRequest
```

Do not copy large context into messages; use refs.

---

# 11. Messaging authority

An agent must have communication authority to address another process outside ordinary parent/child response flow.

Policy can restrict:

```text
parent ↔ child
siblings in same team
named peer set
root only
```

A child cannot message arbitrary agents from another root/session by guessing IDs.

---

# 12. At-least-once transport, idempotent logical message

Internal delivery/recovery may retry.

Logical MessageID is stable.

Receiver records consumption/handling state so a daemon restart does not duplicate semantic handling.

Do not require exactly-once physical delivery.

---

# 13. Mailbox backpressure

Every process mailbox is bounded.

Policies:

- result/control messages receive priority;
- telemetry/status coalesces;
- large evidence via object refs;
- sender may block/wait/retry under bounded policy;
- overloaded child can be marked `MailboxBackpressure` and parent informed.

No unbounded `[]Message` per child.

---

# 14. Team object

Adaptive team formation creates a durable lightweight Team record.

```go
type Team struct {
    ID          TeamID
    RootAgentID AgentID
    PurposeRef  ObjectRef
    MemberIDs   []AgentID
    BudgetRef   BudgetPoolID
    State       TeamState
}
```

Team is organizational metadata, not a security role.

Capabilities remain per Agent Process.

---

# 15. Adaptive team formation

Lead/harness proposes:

```text
problem decomposition
candidate specialist tasks
expected parallel benefit
budget allocation
```

Kernel/scheduler validates:

```text
authority
budget
fan-out/depth
parallelism
World requirements
```

The model can propose topology; runtime controls resource admission.

---

# 16. Fan-out limits

Hard root/global limits:

```text
max total descendants
max active descendants
max depth
max children per node
max parallel model calls
max concurrent Worlds
```

Limits are enforced on durable process creation/admission, not only in prompts.

---

# 17. Recursive explosion detection

Signals:

```text
spawn rate above threshold
repeated equivalent child tasks
children spawning identical grandchildren
budget consumed with no new evidence/progress
team topology growth without criterion progress
```

Response:

```text
SpawnThrottled
NeedsReplan
or parent/user escalation
```

---

# 18. Duplicate-work suppression

Before expensive spawn, scheduler MAY detect near-identical active/completed tasks in same root scope.

Options:

```text
reuse result
subscribe/wait existing child
spawn anyway if independent verification requested
```

Never deduplicate solely by embedding similarity when independent review is intentionally valuable.

---

# 19. Structured negotiation protocol

Negotiation is finite-state, not endless free chat.

```text
CLAIM
  ↓
ACCEPT | CHALLENGE
           ↓
        EVIDENCE
           ↓
     ACCEPT | COUNTEREXAMPLE
                  ↓
               REVISION
                  ↓
          AGREEMENT | ESCALATION
```

Each negotiation has:

```text
max rounds
wall-time budget
model/token budget
required evidence policy
escalation target
```

---

# 20. Negotiation statements

Typed records:

```go
type NegotiationTurn struct {
    ID          NegotiationTurnID
    SessionID   NegotiationID
    Actor       AgentID
    Kind        NegotiationKind
    ClaimRef    *ObjectRef
    Evidence    []EvidenceRef
    RespondsTo  *NegotiationTurnID
}
```

This feeds Epistemic Memory better than transcript-only debate.

---

# 21. Evidence beats rhetoric

Negotiation evaluator prioritizes:

```text
deterministic reproducer/test
source evidence
runtime observation
independent corroboration
model argument
```

A longer explanation does not win by itself.

---

# 22. Deadlock prevention

Potential logical cycle:

```text
A waits for B
B waits for C
C waits for A
```

Runtime maintains wait-for edges for durable waits.

On adding blocking dependency:

```text
check bounded graph cycle
```

If cycle would form:

```text
WaitCycleDetected
```

Harness can convert one edge to message/request, choose timeout, or escalate.

---

# 23. Child failure policy

Parent wait spec defines failure semantics:

```text
fail-fast
collect-all
minimum-success quorum
best-effort
retry child
reroute model
```

No universal "one child failed → root fails" rule.

---

# 24. Child crash/restart

Child process is durable.

Daemon crash:

```text
reconstruct child process
reconcile active operations
resume scheduler admission
parent durable wait remains
```

Parent does not need to replay a giant chat to rediscover children.

---

# 25. Result promotion

Child result is not automatically root truth.

Flow:

```text
child result
→ parent/harness evaluation
→ evidence/verification
→ selected beliefs/artifacts promoted
```

This aligns with Epistemic Memory and fork promotion.

---

# 26. Resource fairness

Recursive teams share system capacity.

Fairness policy includes:

```text
per-root active slots
weighted priority
interactive root reservation later
provider/backend quotas
World quotas
```

A root with 1000 READY children cannot starve a second user's/root process work.

---

# 27. Required tests

```text
ORCH-001 child authority is strict valid subset
ORCH-002 child budget reserved before process becomes READY
ORCH-003 1000 sleeping children do not require 1000 active goroutines/timers
ORCH-004 duplicate MessageID handled idempotently after restart
ORCH-005 mailbox slow consumer cannot cause unbounded heap growth
ORCH-006 wait-for cycle detected before durable blocking edge committed
ORCH-007 root cancel propagates to descendants exactly once logically
ORCH-008 child result large artifact stays object-referenced
ORCH-009 repeated recursive spawn hits deterministic fan-out limits
ORCH-010 parent waiting survives daemon restart
ORCH-011 failed child follows configured quorum/fail-fast semantics
ORCH-012 negotiation stops at max rounds and escalates
ORCH-013 child speculative belief not automatically promoted to root memory
ORCH-014 two roots receive fair execution under one root spawn storm
ORCH-015 completed duplicate task can be reused only when policy permits
```

---

# Accepted A0 decisions

1. `spawn()` is a durable validate/reserve/create operation.
2. Children own separate Agent Process state/context/authority/budget.
3. Parent waiting is durable state, not resident goroutine blocking.
4. No implicit detached children in v0.
5. Logical messaging is idempotent over retryable internal delivery.
6. Mailboxes/queues are bounded and large data uses refs.
7. Adaptive topology is model-proposed but kernel/scheduler-admitted.
8. Wait-for cycles are detected structurally.
9. Negotiation is a bounded typed protocol with evidence and escalation.
10. Child results are candidates/evidence, not automatically root truth.
11. Global/per-root fairness applies to recursive teams.
12. Fan-out/depth/resource limits are deterministic runtime policy.

> **Recursion creates an organization, not an explosion of prompts.**
