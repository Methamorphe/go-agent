# Roadmap

## Strategy

The project uses two phases:

```text
A0  Architecture & Semantics
 ↓
G0… Implementation generations
```

**The project is currently in A0.**

No runtime implementation should begin until the high-coupling semantics required by G0/G1 are sufficiently specified. See `ARCHITECTURE_GATE.md`.

A generation is complete only when its invariants, failure behavior and non-functional claims are tested and documented.

The working product name remains **TBD**.

---

# A0 — Architecture & Semantics

## Goal

Design the execution model deeply enough that implementation validates decisions instead of inventing fundamental semantics ad hoc.

## Required architecture workstreams

- Agent Process state machine and Agent Syscalls;
- Event Ledger ordering, reducers, snapshots and projections;
- canonical vs ephemeral state;
- SQLite/object-store persistence contracts;
- failure and recovery semantics;
- concurrency ownership, bounded queues and backpressure;
- runtime daemon ↔ TUI protocol and history virtualization;
- World/action/effect contracts;
- capability/intent policy model;
- Cognitive MMU pages/working-set/recall semantics;
- recursive-agent budget and cancellation semantics;
- transaction/fork state machines;
- testing, soak and benchmark gates.

## A0 outputs already established

- `ARCHITECTURE_GATE.md`;
- `CONCEPT_CONTRACTS.md` covering all current core concepts;
- `RELIABILITY_AND_PERFORMANCE.md`;
- `TUI_AND_STREAMING.md`;
- `CONCURRENCY_AND_BACKPRESSURE.md`;
- `STATE_PERSISTENCE_AND_STORAGE.md`;
- `FAILURE_MODEL_AND_RECOVERY.md`;
- `TESTING_BENCHMARKS_AND_QUALITY_GATES.md`;
- `ARCHITECTURE_DECISIONS.md`.

## A0 completion criteria

Before G0:

- every G0/G1 primitive has explicit canonical state;
- Agent Process lifecycle/state transition table is frozen enough to implement;
- event ordering/version semantics are decided;
- snapshot/event compatibility strategy exists;
- storage failure behavior is specified;
- daemon/TUI ownership boundary is fixed;
- no unbounded hot-path queues are allowed;
- long-session performance tests are defined;
- initial architecture decisions required by G0/G1 are accepted;
- implementation tasks can be written without deciding core semantics in code review.

The architecture gate does **not** require late-stage algorithms such as learned scheduling or full Truth Maintenance to be solved before G0. Their extension boundaries and invariants must simply be protected.

---

# G0 — Foundations

## Goal

Create a tiny Go codebase implementing the architecture's foundational boundaries without agent behavior.

## Deliverables

- Go module and repository layout;
- CLI/runtime entry points;
- structured logging;
- typed IDs/errors;
- configuration loading;
- SQLite storage implementation;
- object-store implementation;
- migration mechanism;
- deterministic clock/ID test abstractions where useful;
- basic unit/integration/fault test setup;
- profiler/runtime metrics hooks;
- architecture package boundaries.

## Key constraints

- no TUI-owned canonical state;
- no unbounded queues;
- large object APIs stream via `io.Reader`/`io.Writer`;
- storage health failures have explicit behavior;
- standard library preferred where adequate.

## Completion criteria

- `go test ./...` and race suite baseline pass;
- executable/runtime can initialize/reopen DB safely;
- object store can stream large objects with bounded memory;
- database crash/reopen tests pass;
- no agent-specific product behavior leaked into storage/CLI foundations.

---

# G1 — Durable Agent Process + Event Ledger

## Goal

Prove the central process model before calling any LLM.

## Deliverables

### Agent Process

- stable `AgentID`;
- parent/child relationship metadata;
- lifecycle states;
- root Intent object;
- versioned status transitions;
- durable sleep/wait representation foundations.

### Event Ledger

- append-only meaningful events;
- causation/correlation IDs;
- explicit sequence/version model;
- process event stream;
- pure deterministic reducer;
- periodic/versioned snapshot format;
- reconstruct process from snapshot + tail events;
- current process projection.

### Runtime supervision foundation

- activation of runnable durable processes;
- canonical state independent from goroutine lifetime;
- clean shutdown/recovery skeleton.

### CLI inspection

```text
go-agent process create
go-agent process inspect <id>
go-agent process suspend <id>
go-agent process resume <id>
go-agent events <id>
```

## Killer tests

1. create process;
2. append state changes;
3. hard-kill executable;
4. restart;
5. reconstruct exact logical state/version/lineage;
6. verify no in-memory singleton was required;
7. create thousands of waiting processes without one permanent goroutine each.

---

# G2 — Minimal Agent Loop + Agent Syscalls

## Goal

Run the smallest useful intelligent process through kernel boundaries.

## Deliverables

### Provider interface

- provider-independent request/event types;
- streaming;
- cancellation;
- token/cost usage accounting when available;
- one frontier provider adapter;
- one OpenAI-compatible adapter for local/vLLM/Ollama-style endpoints;
- deterministic fake provider for tests.

### Initial syscalls

Start tiny:

```text
observe
execute
checkpoint
```

### Built-in actions

- read file;
- list directory;
- run command with timeout/output limits.

### Streaming architecture

- model tokens use bounded/coalesced live stream;
- final response persisted as object/artifact;
- canonical events record invocation lifecycle, not one event per token;
- command output streams to object store with bounded preview tail.

### UI

A basic attachable client/CLI is enough. Do not build the final TUI yet.

## Completion criteria

The agent can inspect a small repository, run a command and answer while every meaningful action is attributable in the ledger and large output cannot grow hot memory without bound.

---

# G3 — Worlds + Authority + Effect System

## Goal

Make execution structurally controlled.

## Deliverables

### World API

- `LocalWorld`;
- action/result protocol;
- process execution abstraction;
- filesystem abstraction;
- World lifecycle/capability descriptor;
- streamed output/cancellation contract.

### Capability system

- filesystem read/write scopes;
- process execution scopes;
- network policy foundation;
- delegation subset validation;
- capability leases.

### Effect system

```text
Pure
Read
Reversible
Compensatable
Irreversible
```

plus traits such as idempotency/retryability where required.

### Intent Lock

- immutable root intent;
- acceptance criteria;
- allowed/forbidden effect domains;
- action-intent policy hook.

## Killer tests

- child cannot acquire missing parent capability;
- read-only agent cannot write even if LLM requests it;
- expired lease fails deterministically;
- denied action never reaches World;
- prompt/tool content cannot mint authority;
- every security decision is causally visible.

---

# G4 — Cognitive MMU v0

## Goal

Stop treating conversation history as canonical memory.

## Deliverables

### Context Page

- page identity/type/source/scope;
- token estimate;
- importance/relevance metadata;
- persistence;
- object-backed large content.

### Working-set builder

Pack bounded context from:

- intent;
- current task state;
- pinned pages;
- recent causal events;
- explicitly retrieved pages;
- tool/syscall schemas;
- reserved output budget.

### `recall()` syscall

Explicit retrieval before automatic Context Faults.

### Context trace

Every model call records page selection/reason/budget.

### Basic eviction

Remove low-value pages from active context without deleting durable state.

## Killer tests

- accumulated history larger than model context still allows useful work;
- hard token budget is never exceeded;
- old relevant fact can be explicitly recalled;
- hot memory/context remains bounded as synthetic history grows.

---

# G5 — Recursive Agent Processes

## Goal

Bring recursive-agent benefits into the durable process/security/economy model.

## Deliverables

### `spawn()`

Child contract includes:

- task/child intent;
- authority subset;
- budget reservation;
- deadline;
- model policy;
- World policy;
- result/evidence contract.

### Messaging

- request/response;
- status;
- result;
- evidence reference;
- cancellation.

### Supervisor

- bounded concurrent children;
- durable child states;
- restart-safe waiting;
- cancellation tree;
- fairness across roots.

### Budget hierarchy

- token/cost reservation;
- max children;
- max parallelism;
- release unused reservation exactly once.

## Killer test

Root delegates three repository investigations in parallel, survives daemon restart with a child pending, and receives structured evidence without importing whole child transcripts.

---

# G6 — Cognitive Scheduler v0

## Goal

Stop binding an Agent Process to one model.

## Deliverables

- model registry/capabilities;
- cost/latency estimates;
- rule-based task routing;
- provider health/circuit behavior;
- fallback policy;
- privacy/locality constraints;
- per-task override;
- routing events/metrics.

## Initial policy

Deterministic rules first:

```text
cheap extraction → small/cheap model
large-context synthesis → context-capable model
high-risk architecture → strong reasoning model
provider unhealthy → eligible fallback
private/local-only → local compatible model
```

## Evaluation

Compare against always-default/always-strongest/always-cheapest baselines on cost-quality-latency.

---

# G7 — OCI World + Agent Transactions

## Goal

Make speculative work isolated and reversible where semantics allow.

## Deliverables

### OCI World

- isolated workspace mount;
- restricted network mode;
- CPU/memory/time limits;
- controlled environment;
- secret binding without prompt exposure;
- snapshot/fork capability description.

### Transaction API

```text
begin
execute
verify
commit / rollback / reconcile
```

### Verification contract

- commands/tests;
- policies;
- acceptance criteria mapping;
- optional model evaluator only when objective checks are insufficient.

## Killer tests

- multi-file breaking change fails verification and rollback restores defined observable state;
- runtime killed at multiple transaction boundaries recovers correct explicit state;
- irreversible effect cannot be falsely rolled back.

---

# G8 — Cognitive Fork / Speculative Execution

## Goal

Explore alternative futures in parallel.

## Deliverables

- named checkpoint;
- fork Agent Process state;
- fork World state;
- branch-local memory/context overlay;
- independent budget reservations;
- concurrent branches;
- evaluator compares objective results/artifacts;
- promote winner;
- clean/discard/retain loser according to policy.

## Killer demonstration

Implement two solutions to a performance problem, benchmark both in isolated forks, explain comparison and promote only winner with no mutation leakage.

---

# G9 — Epistemic Memory

## Goal

Replace flat snippets with evidence-aware beliefs.

## Deliverables

- belief store;
- scope;
- provenance;
- confidence metadata;
- freshness/status lifecycle;
- contradiction edges;
- dependency graph;
- retrieval integrated with Cognitive MMU.

## Causal invalidation v1

When source evidence changes:

- mark directly derived beliefs stale;
- propagate `needs_review` through dependency graph;
- schedule/recommend verification according to policy.

## Killer test

Agent learns repository architecture fact; source changes; old belief is downgraded and cannot silently rank as trusted while original evidence/history remains inspectable.

---

# G10 — Context Faults + Cognitive MMU v2

## Goal

Make retrieval a runtime primitive rather than only a manual search command.

## Deliverables

- symbolic context references;
- dependency-driven page loading;
- Context Fault events;
- fault-loop/thrashing protection;
- smarter working-set planning;
- structured compaction;
- summary ↔ raw evidence links.

## Research requirement

Automatic faults remain observable/reproducible and cannot trigger hidden unbounded loops.

---

# G11 — Adaptive Teams + Agent Negotiation

## Goal

Move beyond static multi-agent graphs.

## Deliverables

- team proposal/decomposition format;
- scheduler approval against budgets;
- temporary specialist profiles;
- structured claim/challenge/evidence/revision protocol;
- disagreement round/deadline limits;
- escalation to parent/evaluator/user.

## Killer demonstration

Reviewer finds race condition, implementer disputes it, reviewer supplies reproducer and they converge—or escalate deterministically—without unlimited dialogue.

---

# G12 — Verified Continual Improvement

## Goal

Allow system improvement without uncontrolled self-modification.

## Deliverables

Versioned cognitive artifacts:

```text
skills
prompts
agent profiles
routing policies
context policies
```

Lifecycle:

```text
hypothesis → candidate → evaluate → compare baseline → promote/reject → rollback
```

## Hard invariants

- self-improvement cannot expand capabilities;
- cannot rewrite immutable root security/user policy;
- historical invocations remain attributable to exact artifact versions;
- security regression rejects candidate even if task score improves.

---

# G13 — Distributed Worlds / Workers

## Goal

Run same durable Agent Process abstraction across local and remote compute.

## Candidates

- SSH worker;
- remote Go worker protocol;
- Kubernetes jobs/workspaces;
- GPU/local inference node routing;
- distributed object/blob storage;
- worker leases/heartbeats/reconciliation.

Agent identity remains independent from worker location.

---

# G14 — Production Observability / Time-Travel Debugger

## Goal

Make complex agent behavior understandable and replayable.

## Deliverables

- process tree UI;
- causal graph;
- context inspector;
- authority/effect inspector;
- resource/cost timeline;
- World diff viewer;
- historical checkpoint replay;
- fork from past event;
- OpenTelemetry export.

The user should be able to answer:

> “Why did agent A make change X, based on what evidence, under which authority, and what happens if I fork from before that choice?”

---

# Cross-generation reliability requirements

Every generation inherits these requirements:

```text
bounded hot memory
bounded queues
streamed large I/O
explicit cancellation/deadlines
crash-safe canonical state
fault injection
race/leak tests
inspectable causality
no prompt-only security
```

A feature is not “done” if its happy path works but it leaks memory/goroutines or has undefined crash behavior.

---

# Product slices

A credible first coding-agent experience may emerge around G5:

```text
single Go binary distribution
local durable daemon
attachable terminal
persistent Agent Processes
model streaming
filesystem/shell/git actions
Event Ledger
capabilities/effects
bounded context
recursive subagents
```

But visual polish must not compromise progression toward G7–G10, where the strongest differentiation begins.

---

# Testing philosophy

Most kernel semantics must be testable without an LLM.

Use deterministic fake providers and fake Worlds to test:

- process recovery;
- authority subset;
- denied effects;
- transaction rollback;
- fork isolation;
- budget accounting;
- context budget;
- unknown outcomes;
- slow consumers;
- crash recovery.

Real-model tests evaluate harness/model quality separately.

---

# Immediate next step

**Do not implement G0 yet. Continue A0.**

The next concrete architecture tasks are:

1. exact Agent Process state-transition table;
2. event catalog + ordering/version semantics;
3. syscall request/outcome protocol;
4. storage schema/projection boundaries for G1;
5. daemon ↔ TUI IPC/projection contract;
6. World action + Effect descriptor contract;
7. capability grammar/subset semantics;
8. Cognitive MMU v0 packing/recall algorithm;
9. transaction/fork state machines;
10. benchmark acceptance rules and initial ADR closure.

When these high-coupling contracts are stable, G0/G1 can begin without architectural guesswork.
