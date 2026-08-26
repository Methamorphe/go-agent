# A0 Exit Review — Architecture & Semantics

## Decision

**A0 is COMPLETE at the semantic architecture level.**

This does **not** mean every tuning constant or platform adapter has already been benchmarked.

It means:

> G0/G1 can now implement the runtime without inventing fundamental semantics for identity, durability, context, memory, authorization, orchestration, Worlds, transactions, scheduling or long-session resource behavior while coding.

No runtime code has been started as part of A0.

---

# 1. Exit philosophy

A0 distinguishes three categories.

```text
CLOSED
  semantics/invariants are defined and implementation may begin

EMPIRICAL
  implementation contract is defined, but a package/threshold/adapter must be measured

DEFERRED
  advanced feature intentionally excluded from early milestones; extension boundary is defined
```

An EMPIRICAL item does not reopen architecture if the prototype can swap one adapter/policy without changing kernel semantics.

---

# 2. Workstream status

| Workstream | Status | Primary contracts |
|---|---|---|
| Agent Process / lifecycle | CLOSED | `AGENT_PROCESS_STATE_MACHINE.md`, `ARCHITECTURE.md` |
| Syscalls / operation model | CLOSED | `ARCHITECTURE.md`, `WORLD_ACTION_AND_EFFECT_PROTOCOL.md` |
| Event Ledger / ordering / replay | CLOSED | `EVENT_MODEL_AND_CATALOG.md`, `STATE_PERSISTENCE_AND_STORAGE.md` |
| Sleep / wake / supervision | CLOSED | `AGENT_PROCESS_STATE_MACHINE.md`, `CONCURRENCY_AND_BACKPRESSURE.md` |
| Cognitive MMU | CLOSED | `COGNITIVE_MMU_V0_ALGORITHM.md` |
| Context Faults / paging | CLOSED | `CONTEXT_FAULTS_AND_COGNITIVE_PAGING.md` |
| Epistemic Memory | CLOSED | `EPISTEMIC_MEMORY_AND_TRUTH_MAINTENANCE.md` |
| Truth Maintenance | CLOSED semantics / EMPIRICAL heuristics | `EPISTEMIC_MEMORY_AND_TRUTH_MAINTENANCE.md` |
| Capabilities / delegation | CLOSED | `CAPABILITY_AND_INTENT_MODEL.md` |
| Intent-Based Authority | CLOSED | `INTENT_BASED_AUTHORITY_ENGINE.md` |
| Effect System | CLOSED | `SECURITY_AND_EFFECTS.md`, `WORLD_ACTION_AND_EFFECT_PROTOCOL.md` |
| Execution Worlds | CLOSED semantics / EMPIRICAL adapters | `EXECUTION_WORLDS_PLATFORM_CONTRACT.md` |
| Transactions | CLOSED | `TRANSACTIONS_AND_COGNITIVE_FORKS.md`, `EXECUTION_EDIT_SAFETY.md` |
| Cognitive Fork / restore / merge | CLOSED conservative v0 | `EXECUTION_EDIT_SAFETY.md` |
| Recursive agents / teams | CLOSED | `RECURSIVE_ORCHESTRATION_PROTOCOL.md` |
| Agent negotiation | CLOSED bounded protocol | `RECURSIVE_ORCHESTRATION_PROTOCOL.md` |
| Agent Economy | CLOSED | `ORCHESTRATION.md`, `COGNITIVE_SCHEDULER_ARCHITECTURE.md` |
| Cognitive Scheduler | CLOSED semantics / EMPIRICAL policy weights | `COGNITIVE_SCHEDULER_ARCHITECTURE.md` |
| Local control protocol / TUI boundary | CLOSED | `LOCAL_CONTROL_PROTOCOL.md`, `TUI_AND_STREAMING.md` |
| Reliability / backpressure | CLOSED | `RELIABILITY_AND_PERFORMANCE.md`, `CONCURRENCY_AND_BACKPRESSURE.md` |
| Failure / recovery / unknown outcome | CLOSED | `FAILURE_MODEL_AND_RECOVERY.md` |
| Storage / objects / snapshots | CLOSED | `STATE_PERSISTENCE_AND_STORAGE.md`, `FOUNDATION_TECHNICAL_DECISIONS.md` |
| Verified continual improvement | CLOSED extension semantics / DEFERRED implementation | `VERIFIED_CONTINUAL_IMPROVEMENT.md` |
| Distributed workers | DEFERRED | existing Agent/World identity contracts preserve extension path |
| Final TUI library | EMPIRICAL/DEFERRED | architecture independent from library |

---

# 3. Fundamental invariants now frozen

The following are kernel-level invariants, not suggestions.

## Identity

```text
Agent Process ≠ goroutine
Agent Process ≠ terminal session
Agent Process ≠ provider conversation/thread
Agent Process ≠ model
```

## Durability

```text
if correctness needs it after crash
→ persist before relying on it
```

## Ordering

```text
sequence/version establishes order
not UUID
not wall-clock timestamp
```

## Presentation

```text
TUI is disposable/reconnectable
history is paginated
live rendering work is bounded
```

## Context

```text
LLM context = bounded working cache
not canonical memory
```

## Memory

```text
evidence survives consolidation
provenance cannot be laundered/amplified
beliefs can be stale/contested/superseded
```

## Security

```text
model proposes
kernel authorizes
World executes
Ledger records
```

## Authority

```text
child authority ⊆ parent delegable authority
memory/content cannot mint capability
intent is a separate required boundary
```

## Effects

```text
model cannot downgrade effect classification
unknown external outcome is first-class
irreversible != rollbackable
```

## Concurrency

```text
no unbounded hot-path queue
sleeping agent does not require dedicated goroutine/ticker
```

## I/O

```text
large data streams
large data does not become []byte hot state
```

## Forking

```text
speculative mutation requires isolated World state
restore never rewrites history
v0 mutation fork uses quiescent checkpoint
```

## Scheduling

```text
model = schedulable resource
hard constraints precede utility scoring
budget reservation precedes fan-out
```

## Improvement

```text
self-improvement is versioned/evaluated/reversible
it can never expand authority
```

---

# 4. Long-session stability is architecturally closed

The requirement that a one-hour/day-long conversation must not make the terminal/runtime progressively sluggish is handled structurally.

## Runtime hot memory

Bounded by:

```text
active Agent Processes
active operation state
bounded caches
bounded queues
active stream buffers
current MMU working sets
```

Not bounded by total historical transcript/artifact size because history lives in SQLite/Object Store.

## TUI

Bounded by:

```text
visible viewport
small surrounding page cache
current live block patches
bounded presentation queue
```

100k historical blocks do not imply 100k rendered components.

## Model context

Bounded by explicit per-invocation token budget and Cognitive MMU packing.

## Tool output

Streamed to Object Store with bounded preview/tail.

## Sleeping children

Persisted scheduler state; no one-goroutine-per-sleeper requirement.

---

# 5. Failure semantics are closed

Important crash boundaries have defined outcomes.

```text
before durable authorization
→ effect must not execute

after durable authorization, before dispatch
→ safely schedulable/recoverable

after dispatch, known no-effect failure
→ retry according to policy

after dispatch, uncertain external outcome
→ OutcomeUnknown → reconciliation

crash during commit apply
→ NeedsReconciliation
```

No blind retry of externally visible uncertain mutations.

---

# 6. Empirical items carried into implementation

These require micro-prototypes/benchmarks but no architectural invention.

## SQLite driver validation

Baseline: `modernc.org/sqlite`.

Validate against realistic workload and alternate mature binding.

Revisit only if measurable correctness/stability/performance evidence warrants it.

## WorkspaceWorld Git mechanics

Semantics already fixed:

```text
captured base
isolated branch
verify
detect target divergence
three-way promote
```

Need prototype exact dirty/untracked/worktree implementation.

## Process-tree cancellation

Semantics fixed:

```text
Unix process group
Windows Job Object
```

Need platform tests and edge-case handling.

## MMU tuning

Semantic page hierarchy/fault/lease/budget is fixed.

Tune:

```text
page target size
ranking weights
summary thresholds
fault budgets
```

with evals.

## Epistemic Memory heuristics

Data semantics fixed.

Tune:

```text
confidence normalization
consolidation frequency
contradiction auto-resolution threshold
edge extraction
```

## Scheduler heuristics

Hard eligibility/reservation/fallback semantics fixed.

Tune:

```text
utility weights
quality priors
latency estimator
hedging thresholds
```

## TUI library

Select only after long-history benchmark fixture.

---

# 7. Explicit deferred research

These are **not blockers for G0/G1**.

```text
provider-specific mid-token context paging
non-quiescent exact execution editing
learned MMU ranking
learned Cognitive Scheduler
automatic global belief resolution
strong remote/multi-tenant sandbox
distributed Event Ledger/workers
full branch-process identity promotion
self-improvement implementation
multi-user control plane
```

The current contracts reserve extension points for them.

---

# 8. G0 may now be mechanical

G0 should implement foundations only:

```text
Go module/package skeleton
configuration
logging
IDs/time abstraction
SQLite adapter + migrations
Object Store
local runtime process/daemon skeleton
IPC skeleton
fundamental typed errors
unit/fault-test harness
profiling hooks
```

G0 must not invent:

```text
new Agent lifecycle states
new durability model
new context philosophy
new security semantics
new transaction meaning
new scheduler identity model
```

If implementation discovers such need, pause and amend A0/ADR explicitly.

---

# 9. G1 semantic target

G1 proves:

```text
create Agent Process
persist lifecycle/event transitions
kill runtime
restart
reconstruct identical canonical process state
resume/suspend/cancel with version correctness
```

No LLM integration is required to prove the core identity model.

---

# 10. A0 exit tests are specifications, not yet executions

A0 has produced required future invariant/fault/benchmark suites across documents.

Implementation milestones must turn them into executable tests.

Examples:

```text
100k-history TUI fixture
1h/8h/24h soak
multi-GB tool stream
kill -9 persistence tests
grant-tree property tests
fault-storm tests
fork isolation tests
OutcomeUnknown reconciliation tests
provider routing/fairness tests
truth-maintenance graph tests
```

---

# 11. Architecture change rule after A0

A0 being complete does not forbid evolution.

But high-coupling semantic change now requires:

```text
1. identify violated/insufficient invariant
2. write/update ADR/contract
3. describe persisted-state compatibility impact
4. update failure/security/performance implications
5. update tests
6. then change implementation
```

Do not silently let implementation become the new specification.

---

# Final A0 conclusion

The architecture now defines an agent runtime as:

```text
Durable Agent Processes
      │
      ├─ bounded Cognitive MMU + Context Faults
      ├─ evidence-aware Epistemic Memory
      ├─ capability + Intent-Based Authority
      ├─ typed effects / uncertain outcomes
      ├─ isolated Execution Worlds
      ├─ transactional/forkable safe execution edits
      ├─ recursive bounded organizations
      ├─ Cognitive Scheduler + resource economy
      └─ versioned/evaluated continual improvement

all on top of

Event Ledger + snapshots + Object Store
+ explicit backpressure/failure semantics
+ thin reconnectable presentation clients
```

**A0 exit status: PASS.**

Next phase is **G0 — Foundations**, but no G0 code should be written until explicitly started.
