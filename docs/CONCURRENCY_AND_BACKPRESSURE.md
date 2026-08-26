# Concurrency, Supervision and Backpressure

## Purpose

Go makes concurrency easy to start and easy to misuse.

This runtime will contain many naturally concurrent activities:

- model streams;
- tool processes;
- recursive agents;
- world operations;
- persistence;
- UI subscriptions;
- approvals;
- timers;
- background maintenance;
- scheduler work.

The concurrency model must therefore be explicit before implementation.

---

# 1. Fundamental rule

> **Durable Agent Processes are not goroutines. Goroutines are temporary execution workers for durable state machines.**

A process can be `WAITING`, `SLEEPING` or `SUSPENDED` with zero resident goroutines dedicated to it.

A goroutine can disappear after restart while the Agent Process continues to exist.

---

# 2. Structured concurrency

Every asynchronous operation belongs to a scope.

Conceptual hierarchy:

```text
Runtime
 ├─ Storage service
 ├─ Scheduler
 ├─ IPC server
 └─ Agent execution scope A
      ├─ Model invocation 1
      ├─ Tool execution 4
      └─ Child scheduling request
```

No detached anonymous background task should be started without an explicit lifecycle owner.

For request-scoped work, Go `context.Context` carries:

- cancellation;
- deadline;
- trace correlation;
- request-local metadata where appropriate.

But durable lifecycle state is persisted separately.

---

# 3. Supervisor responsibilities

The runtime supervisor owns ephemeral execution.

Responsibilities:

- activate runnable Agent Processes;
- enforce maximum concurrency;
- create execution scopes;
- catch/report worker failures;
- propagate cancellation;
- release resources;
- transition durable state;
- avoid duplicate activation of the same exclusive step;
- recover interrupted work after restart.

The supervisor does not contain cognitive policy.

---

# 4. Work units

The scheduler/supervisor should operate on explicit work units rather than arbitrary goroutines.

Possible work-unit classes:

```text
ModelInvocation
WorldAction
ContextBuild
Verification
ChildActivation
MemoryMaintenance
BlobTransfer
```

Each work unit has:

```text
ID
AgentID
class
priority
resource reservation
deadline
idempotency/retry semantics
state
```

This allows fairness and observability.

---

# 5. Global concurrency limits

Recursive agents can otherwise create a fork bomb made of API calls.

Runtime-level semaphores/queues should bound at least:

```text
active model invocations
active tool processes
active World mutations
active CPU-heavy jobs
active child activations
```

Limits can be global plus per-root-agent.

Example:

```text
Global max model calls: 16
Root agent max:          6
Single child max:        2
```

A child may have budget available but still wait for a global execution slot.

---

# 6. Fair scheduling

One large recursive tree must not starve interactive work.

Candidate scheduling signals:

- user-interactive vs background;
- root-agent fair share;
- task priority;
- deadline;
- waiting time;
- resource cost;
- risk/approval state.

Initial implementation should use deterministic weighted queues rather than sophisticated optimization.

Fairness decisions should be observable.

---

# 7. Bounded channels

Channels used as subsystem boundaries should have deliberately chosen capacities.

Bad:

```go
ch := make(chan Event, 1000000) // hides overload
```

Also bad: implementing an effectively unbounded queue behind a channel.

For every channel document:

```text
owner
writers
reader
capacity
what happens when full
what data may be lost/coalesced
shutdown semantics
```

---

# 8. Stream classes

## Canonical command/event stream

Loss: forbidden.

Backpressure: block/reject/fail explicitly.

Examples:

- lifecycle transition commands;
- capability grants;
- transaction commits;
- approval decisions.

## Reconstructable live stream

Loss/coalescing: allowed because final canonical result exists elsewhere.

Examples:

- model token previews;
- stdout previews;
- progress counters.

## Telemetry stream

Loss: acceptable under overload if counted.

Examples:

- high-frequency metrics;
- debug traces not required for audit.

The type system/API should make these categories hard to confuse.

---

# 9. Provider streaming pipeline

Recommended pipeline:

```text
Provider reader
    │
    ▼
bounded decoder
    │
    ├── usage/tool-call assembler
    ├── text coalescer → presentation bus
    └── canonical response builder → object store
```

A slow TUI does not slow provider consumption until bounded presentation policy is reached.

A slow object store is correctness-sensitive and may legitimately backpressure/cancel the invocation rather than buffer indefinitely.

---

# 10. Tool execution pipeline

```text
OS process
  ├─ stdout reader ─┬─ blob sink
  │                 └─ bounded preview tail
  └─ stderr reader ─┬─ blob sink
                    └─ bounded preview tail
```

Never wait to call `cmd.Wait()` while a pipe is unread and able to deadlock.

Process cancellation must terminate descendant processes according to platform/world guarantees, not only cancel the Go goroutine reading output.

---

# 11. Cancellation semantics

Distinguish:

```text
Cancel request
Execution cancellation signal
Durable cancelled state
```

Flow:

```text
user/parent requests cancel
        │
        ▼
persist CancelRequested
        │
        ▼
signal active execution context
        │
        ▼
wait/force according to policy
        │
        ▼
persist Cancelled / FailedCancellation
```

This makes cancellation recoverable if the runtime crashes mid-cancel.

---

# 12. Timeout semantics

Timeouts must exist at several levels:

- model request;
- tool command;
- World operation;
- approval lease;
- transaction;
- entire task/deadline.

A timeout is a policy event, not merely `context deadline exceeded` printed to the user.

The ledger records which limit fired.

---

# 13. Retry semantics

Retries depend on Effect metadata.

```text
Read + idempotent
→ usually retryable

Reversible mutation with idempotency key
→ potentially retryable

Compensatable action
→ retry only with explicit semantics

Irreversible unknown outcome
→ never blind retry
```

Critical case:

```text
request sent
connection lost
unknown whether external system applied effect
```

This becomes `OutcomeUnknown`, not `Failed`.

The process must verify/reconcile before another attempt.

---

# 14. Panic isolation

Kernel bugs should still surface loudly, but a panic in an isolated task callback/provider adapter should not silently kill the entire daemon when recovery is safe.

Boundary strategy:

- recover only at deliberate supervisor boundaries;
- convert panic to structured failure + stack trace;
- mark affected work unit failed/interrupted;
- never use blanket `recover()` to hide programming errors;
- allow configured fail-fast mode in development/testing.

---

# 15. Timers and sleeping agents

Do not create one permanent `time.Ticker` goroutine per sleeping agent.

Use a centralized durable timer scheduler:

```text
SQLite wake condition/time
        │
        ▼
central scheduler / min-heap
        │
        ▼
next wake timer
```

On restart, reload pending wakes.

For very large scale later, the implementation can change without changing Agent Process semantics.

---

# 16. Child-agent execution

`spawn()` creates durable child state first, then schedules execution.

It should not mean:

```go
go child.Run()
```

as the canonical operation.

Correct conceptual order:

```text
validate delegated authority/budget
        ↓
persist ChildCreated
        ↓
reserve resources
        ↓
mark READY
        ↓
supervisor schedules when capacity exists
```

This ensures a crash after child creation does not lose the child.

---

# 17. Fork concurrency

Branches must have explicit isolated resources.

A fork fan-out needs:

- maximum branch count;
- reserved cost/time budgets;
- world snapshot capacity;
- evaluator slot;
- cancellation of losers;
- cleanup lifecycle.

Speculative execution should be aggressively bounded because it multiplies resource use by design.

---

# 18. Storage writer architecture

A local runtime can benefit from a small controlled persistence pipeline.

Canonical writes should use transactions and acknowledgements.

Potential model:

```text
kernel commands
     │
     ▼
state transition service
     │
     ▼
SQLite transaction
  append events
  update projections/indexes if used
     │
     ▼
acknowledge transition
```

Do not place an unbounded async queue in front of canonical writes merely to make benchmarks appear fast.

Where write batching is introduced, durability semantics must remain explicit.

---

# 19. Event bus architecture

Avoid one magical global pub/sub bus for every data class.

Prefer typed/internal streams:

```text
canonical state notifications
presentation updates
telemetry
worker completions
```

Subscribers must not be able to block canonical state mutation indefinitely.

Presentation notifications should carry IDs/projections so clients can refetch state after missed updates.

---

# 20. Concurrency testing

Mandatory test families:

- `go test -race ./...`;
- deterministic state-machine tests;
- cancellation storms;
- rapid spawn/cancel loops;
- provider disconnect during stream;
- command produces stdout while cancelled;
- TUI subscriber stops consuming;
- SQLite busy/slow simulation;
- runtime shutdown with active children;
- duplicate activation attempt;
- goroutine leak detection;
- fork fan-out resource exhaustion.

---

# 21. Shutdown protocol

Daemon shutdown should be phased:

```text
1. stop accepting new interactive work
2. mark runtime draining
3. request cancellation or checkpoint according to configured policy
4. stop new scheduler dispatch
5. allow bounded grace period
6. persist interrupted states
7. flush canonical storage
8. close IPC
9. exit
```

A hard crash is separately handled through recovery logic.

---

# Core invariant

> **Concurrency is an execution optimization; it must never become the source of canonical state or unlimited resource growth.**
