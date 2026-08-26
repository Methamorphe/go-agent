# Roadmap

## Strategy

The project uses two phases:

```text
A0  Architecture & Semantics
 ↓
G0… Implementation generations
```

**A0 is complete. G0 has not started.**

See:

- `A0_EXIT_REVIEW.md`;
- `ARCHITECTURE_GATE.md`;
- `ARCHITECTURE_DECISIONS.md`;
- `FOUNDATION_TECHNICAL_DECISIONS.md`.

A generation is complete only when its invariants, failure behavior and non-functional claims are tested and documented.

The working product name remains **TBD**.

---

# A0 — Architecture & Semantics ✅ COMPLETE

## Goal

Design the execution model deeply enough that implementation validates decisions instead of inventing fundamental semantics ad hoc.

## Closed workstreams

A0 now defines:

- Agent Process state machine and Agent Syscalls;
- Event Ledger ordering, reducers, snapshots and projections;
- canonical vs ephemeral state;
- SQLite/Object Store persistence contracts;
- failure and recovery semantics;
- concurrency ownership, bounded queues and backpressure;
- runtime daemon ↔ TUI protocol and history virtualization;
- World/action/effect contracts;
- capability + Intent-Based Authority model;
- Cognitive MMU pages/working-set/recall semantics;
- Context Fault semantics;
- Epistemic Memory + Truth Maintenance;
- recursive-agent budget/cancellation/messaging semantics;
- transaction/fork/restore/merge safety;
- Cognitive Scheduler + Agent Economy;
- Verified Continual Improvement extension semantics;
- testing, soak and benchmark gates.

## A0 result

**PASS.**

Remaining unknowns are isolated as empirical adapter/tuning work rather than semantic gaps.

Examples:

```text
SQLite driver soak/performance validation
Git WorkspaceWorld implementation details
Unix/Windows process-tree edge cases
MMU ranking thresholds
Memory consolidation heuristics
Scheduler utility weights
TUI library selection
```

These do not block G0 because their interfaces/guarantees are already defined.

---

# G0 — Foundations

## Goal

Create a tiny Go codebase implementing the architecture's foundational boundaries without agent behavior.

## Deliverables

- Go module and repository layout;
- CLI/runtime daemon entry points;
- structured logging;
- typed IDs/errors;
- configuration loading;
- SQLite storage implementation;
- content-addressed streaming Object Store;
- migration mechanism;
- clock/ID test abstractions where useful;
- local IPC skeleton;
- basic unit/integration/fault test setup;
- profiler/runtime metrics hooks;
- architecture package boundaries.

## Accepted implementation baseline

```text
Go
modernc.org/sqlite behind internal adapter (provisional/benchmarked)
database/sql + explicit SQL
SQLite WAL + foreign_keys + reliability-first synchronous mode
versioned JSON event payloads
versioned rebuildable JSON snapshots
SHA-256 content-addressed Object Store
Unix domain socket / Windows named pipe IPC
length-framed versioned JSON control protocol
```

## Key constraints

- no TUI-owned canonical state;
- no unbounded queues;
- large object APIs stream via `io.Reader`/`io.Writer`;
- storage health failures have explicit behavior;
- standard library preferred where adequate;
- no ORM in kernel storage;
- no new architecture semantics invented in code.

## Completion criteria

- `go test ./...` passes;
- race-suite baseline passes on supported platform set;
- runtime initializes/reopens DB safely;
- Object Store streams large objects with bounded memory;
- DB kill/reopen tests pass;
- snapshots are rebuildable from ledger;
- malformed IPC cannot crash daemon;
- storage adapter isolates SQLite driver-specific code;
- no agent-specific behavior leaked into foundation layers.

---

# G1 — Durable Agent Process + Event Ledger

## Goal

Prove the central process model before calling any LLM.

## Deliverables

### Agent Process

- stable `AgentID`;
- parent/child relationship metadata;
- lifecycle states from accepted state machine;
- root Intent object;
- versioned status transitions;
- durable sleep/wait representation foundations.

### Event Ledger

- append-only meaningful events;
- causation/correlation IDs;
- accepted global sequence + process-version model;
- pure deterministic reducer;
- versioned snapshot format;
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
- command output streams to Object Store with bounded preview/tail.

### UI

A basic attachable client/CLI is enough. Do not build final TUI yet.

## Completion criteria

Agent can inspect a small repository, run a command and answer while every meaningful action is attributable in Ledger and large output cannot grow hot memory without bound.

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
- World lifecycle/Profile;
- streamed output/cancellation contract;
- platform process-tree adapter.

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

plus traits such as idempotency/retryability.

### Intent-Based Authority foundation

- immutable/versioned root Intent;
- acceptance criteria;
- allowed/forbidden effect domains;
- Purpose-Carrying Actions;
- Action Proof;
- typed authorization outcomes.

## Killer tests

- child cannot acquire missing parent capability;
- read-only agent cannot write even if model requests it;
- expired lease fails deterministically;
- denied action never reaches World;
- prompt/tool content cannot mint authority;
- model cannot downgrade Effect classification;
- every security decision is causally visible.

---

# G4 — Cognitive MMU v0

## Goal

Stop treating conversation history as canonical memory.

## Deliverables

- semantic Context Pages;
- token estimates;
- persisted metadata/object content;
- deterministic tiered working-set builder;
- explicit `recall()`;
- Context Manifest;
- basic structured compaction;
- bounded eviction/cache behavior.

## Killer tests

- accumulated history larger than model context still allows useful work;
- hard token budget never exceeded;
- old relevant fact explicitly recalled;
- 100k-page corpus does not materialize all bodies;
- hot memory/context remains bounded as history grows.

---

# G5 — Recursive Agent Processes

## Goal

Bring recursive-agent benefits into the durable process/security/economy model.

## Deliverables

- durable `spawn()` validate/reserve/create protocol;
- child Task Intent and authority subset;
- result/evidence contract;
- bounded messaging/mailboxes;
- durable parent waits;
- cancellation tree;
- wait-for cycle detection;
- fan-out/depth/fairness controls;
- budget reservations/settlement.

## Killer test

Root delegates three repository investigations in parallel, survives daemon restart with child pending, and receives structured evidence without importing whole child transcripts.

---

# G6 — Cognitive Scheduler v0

## Goal

Stop binding an Agent Process to one model.

## Deliverables

- model registry/Profile;
- Cognitive Task descriptor;
- hard eligibility filtering;
- cost/latency/quality score policy;
- runtime provider health/circuit breaker;
- budget reservation/settlement;
- fallback;
- privacy/locality constraints;
- fairness/global/per-root slots;
- routing decision events/metrics.

## Initial policy

Deterministic rules first.

No learned router in v0.

## Evaluation

Compare against always-default/always-strongest/always-cheapest baselines on verified quality, cost and latency.

---

# G7 — Workspace/OCI World + Agent Transactions

## Goal

Make speculative work isolated and reversible where guarantees permit.

## Deliverables

### WorkspaceWorld

- Git-aware isolated workspace;
- captured dirty/untracked base policy;
- target divergence detection;
- three-way promotion.

### OCI World

- controlled mounts;
- restricted network default;
- CPU/memory/time limits;
- secret binding;
- snapshot/fork Profile.

### Transaction API

```text
begin
execute
verify
prepare
commit / rollback / reconcile
```

## Killer tests

- breaking multi-file change fails verification and rolls back exact defined state;
- kill runtime at each transaction boundary;
- crash during APPLY enters reconciliation, never false commit;
- irreversible effect cannot be falsely rolled back.

---

# G8 — Cognitive Fork / Safe Execution Editing

## Goal

Explore alternative futures in parallel without rewriting history.

## Deliverables

- Forkable quiescent checkpoint;
- Execution Frontier;
- isolated branch Agent/World state;
- branch-local memory/context overlay;
- independent budget reservations;
- objective evaluator;
- three-way World merge;
- selective cognitive promotion;
- promotion lease;
- restore-as-new-timeline;
- cleanup/retention.

## Killer demonstration

Implement two solutions to a performance problem, benchmark both in isolated forks, explain comparison and promote only winner with no mutation leakage or history truncation.

---

# G9 — Epistemic Memory + Truth Maintenance

## Goal

Replace flat snippets with evidence-aware knowledge.

## Deliverables

- immutable/versioned Evidence store;
- platform provenance;
- Belief lifecycle;
- scope/temporal validity;
- structured confidence metadata;
- contradiction/dependency edges;
- localized causal invalidation;
- Cognitive MMU integration;
- branch-local memory overlays.

## Killer test

Agent learns repository architecture fact; source changes; old belief is downgraded and cannot silently rank as trusted while original evidence/history remains inspectable.

---

# G10 — Context Faults + Cognitive MMU v2

## Goal

Make missing knowledge a typed runtime paging event.

## Deliverables

- stable semantic cognitive references;
- Reference/Recall/Evidence/Freshness/Dependency/Representation faults;
- context leases;
- dependency-driven page loading;
- fault budgets/storm protection;
- structured compaction ↔ evidence paging;
- improved working-set planning.

## Requirement

Faults remain provider-independent at invocation/tool boundaries and observable/replayable.

---

# G11 — Adaptive Teams + Agent Negotiation

## Goal

Move beyond static multi-agent graphs.

## Deliverables

- team proposal/decomposition;
- scheduler admission;
- temporary specialist profiles;
- bounded claim/challenge/evidence/counterexample/revision protocol;
- disagreement deadlines/round limits;
- escalation.

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
memory policies
```

Lifecycle:

```text
hypothesis
→ candidate
→ evaluate
→ shadow
→ canary
→ promote/reject
→ rollback
```

## Hard invariants

- cannot expand capabilities;
- cannot rewrite root Intent/effect floor;
- historical invocations retain exact artifact versions;
- security regression rejects candidate even if quality improves.

---

# G13 — Distributed Worlds / Workers

## Goal

Run same durable Agent Process abstraction across local and remote compute.

Candidates:

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

- process/team tree UI;
- causal graph;
- context/fault inspector;
- belief/provenance graph;
- authority/Action Proof inspector;
- resource/cost timeline;
- World diff viewer;
- checkpoint/fork/restore/merge timeline;
- historical replay/fork;
- OpenTelemetry export.

---

# Cross-generation reliability requirements

Every generation inherits:

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

A feature is not done if happy path works but it leaks memory/goroutines, blocks on slow clients or has undefined crash behavior.

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
capabilities/effects/Intent
bounded context
recursive subagents
```

But polish must not prevent progression toward G7–G10, where the strongest differentiation begins.

---

# Testing philosophy

Most kernel semantics must be testable without an LLM.

Use deterministic fake providers and fake Worlds for:

- process recovery;
- authority subset;
- denied effects;
- transaction rollback;
- fork isolation;
- budget accounting;
- context budgets/faults;
- unknown outcomes;
- slow consumers;
- crash recovery;
- Truth Maintenance propagation;
- scheduler fairness.

Real-model tests evaluate harness/model quality separately.

---

# Immediate next step

**G0 is ready to start, but has not started.**

When explicitly requested, implement **G0 only** according to:

- `FOUNDATION_TECHNICAL_DECISIONS.md`;
- `A0_EXIT_REVIEW.md`;
- `ARCHITECTURE_DECISIONS.md`;
- reliability/failure/storage/control-protocol contracts.

If G0 implementation reveals a need to change a high-coupling semantic invariant, stop and amend the architecture first rather than embedding the change silently in code.
