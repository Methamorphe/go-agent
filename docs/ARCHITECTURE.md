# Target Architecture

## Architectural goal

The system is designed as a small **Agent Kernel** hosting durable intelligent processes.

The kernel should not contain product-specific behavior such as “coding workflow”, “research workflow” or “DevOps workflow”. Those belong to harnesses and applications above it.

```text
┌─────────────────────────────────────────────┐
│              Agent Applications             │
│ Coding / Research / DevOps / Personal / …  │
└──────────────────────┬──────────────────────┘
                       │
┌──────────────────────▼──────────────────────┐
│               Harness Layer                 │
│ prompts / roles / policies / workflows     │
└──────────────────────┬──────────────────────┘
                       │
┌──────────────────────▼──────────────────────┐
│                Agent Runtime                │
├─────────────────────────────────────────────┤
│ Agent Processes                             │
│ Agent Syscalls                              │
│ Event Ledger + Snapshots                    │
│ Cognitive MMU                               │
│ Epistemic Memory                            │
│ Authority / Intent / Effect Engine          │
│ Fork + Transaction Manager                  │
│ Cognitive Scheduler                         │
│ Resource/Budget Accounting                  │
└──────────────────────┬──────────────────────┘
                       │
┌──────────────────────▼──────────────────────┐
│              Execution Worlds               │
│ local / OCI / SSH / browser / Python / MCP │
└─────────────────────────────────────────────┘
```

## 1. Agent Process

An Agent Process is the core unit of execution.

It is not equivalent to:

- one Go goroutine;
- one OS process;
- one LLM conversation;
- one provider thread ID;
- one terminal session.

It is a durable logical process with an identity and replayable state.

### Proposed process state

```go
type AgentID string

type AgentProcess struct {
    ID          AgentID
    ParentID    *AgentID
    Status      ProcessStatus
    Intent      Intent
    Authority   AuthoritySet
    Budget      Budget
    ModelPolicy ModelPolicy
    WorldID     WorldID
    MemoryScope MemoryScope
}
```

Operational state should be reconstructed from persisted events and snapshots rather than treated as an opaque in-memory object.

### Lifecycle

```text
CREATED
  ↓
READY
  ↓
RUNNING
  ├─→ WAITING_TOOL
  ├─→ WAITING_CHILD
  ├─→ SLEEPING
  ├─→ SUSPENDED
  └─→ RUNNING
  ↓
COMPLETED / FAILED / CANCELLED
```

A suspended process is expected to resume later, potentially in another Go process or machine.

## 2. Agent Syscalls

The LLM/harness should interact with a stable kernel API instead of directly depending on all underlying technologies.

Candidate syscall vocabulary:

```text
observe()      inspect world/state
recall()       request knowledge/context
execute()      perform an action in a world
spawn()        create child process
message()      communicate with process
signal()       cancel/pause/wake/etc.
checkpoint()   persist a named recoverable state
fork()         create isolated alternative
verify()       run explicit acceptance checks
commit()       promote effects from transaction/fork
rollback()     discard reversible effects
sleep()        wait until time/condition
```

Not all syscalls need to be directly visible as LLM function calls. Harnesses can expose higher-level APIs while preserving these semantics underneath.

### Why syscalls matter

Without a kernel abstraction, every harness talks directly to:

```text
bash + MCP + Docker + browser + filesystem + provider SDK + …
```

That makes policy, replay, accounting and portability difficult.

With a kernel:

```text
Agent → execute(intent) → Kernel → chosen World/backend
```

The runtime can intercept every action for authority checks, effect classification, logging, budgets and transaction handling.

## 3. Event Ledger

All meaningful state transitions should become append-only events.

Examples:

```text
AgentCreated
IntentBound
CapabilityGranted
ModelInvocationStarted
ModelInvocationCompleted
SyscallRequested
SyscallAuthorized
WorldActionStarted
WorldActionCompleted
MemoryBeliefAdded
ContextPageLoaded
AgentForked
TransactionStarted
VerificationCompleted
TransactionCommitted
AgentSuspended
AgentResumed
```

### Event envelope

```go
type Event struct {
    ID        EventID
    AgentID   AgentID
    Type      EventType
    Timestamp time.Time
    Causation *EventID
    Correlation CorrelationID
    Payload   json.RawMessage
}
```

### Benefits

- replay;
- debugging;
- auditability;
- crash recovery;
- observability;
- causal tracing;
- reproducible tests;
- future distributed workers.

### Snapshots

Long-running processes should periodically persist compact snapshots:

```text
snapshot N
+
events N+1…current
=
current operational state
```

Snapshots are an optimization, not the canonical history.

## 4. World abstraction

A World is an execution environment with explicit boundaries.

```go
type World interface {
    ID() WorldID
    Execute(ctx context.Context, action Action) (Result, error)
    Snapshot(ctx context.Context) (WorldSnapshot, error)
    Fork(ctx context.Context) (World, error)
    Destroy(ctx context.Context) error
}
```

Potential implementations:

### Local World

Direct interaction with a local workspace. Useful for development but lower isolation.

### OCI / Container World

Filesystem/process/network isolation using Docker, containerd or another OCI runtime.

### SSH World

Remote host execution behind the same logical API.

### Kubernetes World

Ephemeral jobs/pods or long-running workspaces.

### Browser World

Browser automation as a controlled environment with explicit network/domain policy.

### Python World

Persistent Python/IPython/Jupyter-like execution for dynamic computation and data manipulation.

### MCP World / Adapter

External tools surfaced through MCP while still passing through authority/effect accounting.

## 5. Provider abstraction

The Agent Process must not depend directly on a provider’s conversation object.

```go
type Model interface {
    Stream(ctx context.Context, req ModelRequest) (<-chan ModelEvent, error)
}
```

The runtime-level request should include:

- working context;
- available syscall/tool schemas;
- reasoning policy;
- output constraints;
- budget/deadline;
- cancellation.

Provider adapters translate this into OpenAI, Anthropic, Gemini, OpenAI-compatible local servers, etc.

## 6. Resource accounting

Resources must be first-class.

```go
type Budget struct {
    MaxCost          Money
    MaxInputTokens   int64
    MaxOutputTokens  int64
    MaxWallTime      time.Duration
    MaxChildren      int
    MaxParallelism   int
    MaxToolCalls     int
}
```

Children inherit bounded subsets from their parent. The scheduler cannot silently exceed global/user limits.

## 7. Process supervision

Go is especially suitable here.

A runtime supervisor can map durable Agent Processes onto ephemeral execution goroutines/workers:

```text
Durable Agent A ──┐
Durable Agent B ──┼── Runtime Supervisor ── goroutines / OS workers
Durable Agent C ──┘
```

Cancellation should use hierarchical contexts, but durable cancellation state must also be persisted.

A Go `context.Context` is an execution mechanism, not the source of truth for agent lifecycle.

## 8. Storage layers

An initial local implementation can stay intentionally simple:

### SQLite

Good fit for:

- event ledger;
- process metadata;
- snapshots;
- beliefs/memory metadata;
- capability leases;
- budgets;
- indexes.

### Blob/Object directory

Good fit for:

- large tool outputs;
- context pages;
- file snapshots;
- logs;
- model artifacts;
- compressed event payloads.

Later, storage interfaces can support PostgreSQL/object storage/distributed systems.

## 9. Observability

The architecture should expose more than textual chat logs.

A trace should answer:

```text
Why did the agent do this?
What user intent allowed it?
Which process requested it?
Which evidence was in context?
Which capability authorized it?
What effect class was assigned?
What model made the decision?
How much did it cost?
What changed in the world?
Can it be rolled back?
```

OpenTelemetry-style tracing is a natural future integration.

## 10. Isolation between kernel and harness

The harness can be opinionated:

```text
“Use reviewer agents after edits.”
“Always run tests before completion.”
“Prefer model X for architecture.”
```

The kernel should stay neutral:

```text
spawn()
execute()
verify()
commit()
```

This allows experiments in prompting/orchestration without destabilizing core guarantees.

## 11. Initial package direction

Tentative Go module layout:

```text
cmd/
  agent/

internal/
  kernel/
    process/
    event/
    syscall/
    supervisor/
  contextvm/
  memory/
  authority/
  effects/
  transaction/
  scheduler/
  storage/
  provider/
  world/
    local/
    oci/
    python/

pkg/
  protocol/      # only if a stable public API emerges
```

Avoid prematurely publishing every internal package as a public SDK.

## 12. Core invariant

The central architectural invariant should be:

> **No state-changing action reaches a World without passing through kernel authority, effect, budget and event-recording boundaries.**

If that invariant stays true, many advanced features become possible later without losing control of the system.
