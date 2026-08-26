# Implementation Roadmap

## Strategy

The project should avoid starting as a giant framework. Each milestone must prove one architectural property with tests and observable behavior.

The roadmap uses `Gx` generations. A generation is complete only when its invariants are tested and documented.

The working product name remains **TBD**.

---

# G0 — Foundations

## Goal

Create a tiny Go codebase with strong boundaries before implementing agent behavior.

## Deliverables

- Go module and repository layout;
- CLI entry point;
- structured logging;
- typed IDs/errors;
- configuration loading;
- SQLite storage abstraction;
- migration mechanism;
- clock/UUID abstractions where useful for deterministic tests;
- basic unit/integration test setup;
- architecture package boundaries.

## Suggested initial dependencies

Keep dependencies minimal.

Possible choices to evaluate rather than blindly adopt:

- CLI: standard `flag` first, Cobra only if commands become complex;
- SQLite: `modernc.org/sqlite` for pure Go or `mattn/go-sqlite3` if CGO is acceptable;
- migrations: lightweight embedded SQL migrations;
- logging: `log/slog`;
- IDs: standard/random UUID implementation or a small dependency;
- terminal UI later: Bubble Tea/Lip Gloss if the TUI justifies it.

## Completion criteria

- `go test ./...` passes;
- executable starts with no provider configured;
- DB can initialize/reopen safely;
- no agent-specific product logic leaked into storage/CLI foundations.

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
- status transitions with validation.

### Event Ledger

- append-only events;
- causation/correlation IDs;
- process event stream;
- state reducer;
- periodic snapshot format;
- reconstruct process from snapshot + events.

### CLI

```text
go-agent process create
go-agent process inspect <id>
go-agent process suspend <id>
go-agent process resume <id>
go-agent events <id>
```

## Killer test

1. create process;
2. append state changes;
3. terminate executable;
4. restart;
5. reconstruct exact logical state.

No in-memory singleton may be required for correctness.

---

# G2 — Minimal Agent Loop + Agent Syscalls

## Goal

Run the smallest useful intelligent process through kernel boundaries.

## Deliverables

### Provider interface

- provider-independent request/event types;
- streaming;
- cancellation;
- token/cost usage accounting when provider exposes it;
- one frontier provider adapter;
- one OpenAI-compatible adapter for local/vLLM/Ollama-style endpoints.

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

Every call must produce ledger events.

### Session/TUI

A basic interactive CLI is enough. Do not build the final TUI yet.

## Completion criteria

The agent can inspect a small repository, run a command and answer a question while every action is attributable in the ledger.

---

# G3 — Worlds + Authority + Effect System

## Goal

Make tool execution structurally controlled.

## Deliverables

### World API

Implement:

- `LocalWorld`;
- process execution abstraction;
- filesystem abstraction;
- world lifecycle.

Design interfaces so OCI can follow without over-generalizing prematurely.

### Capability system

- filesystem read/write scopes;
- process execution scopes;
- network policy placeholder;
- delegation subset validation;
- capability leases.

### Effect system

Initial classes:

```text
Pure
Read
Reversible
Compensatable
Irreversible
```

Actions declare effect metadata.

### Intent Lock

- immutable root intent;
- acceptance criteria;
- allowed/forbidden effect domains;
- action-intent policy hook.

## Killer tests

- child cannot acquire missing parent capability;
- read-only agent cannot write even if the LLM requests it;
- expired lease fails deterministically;
- denied action never reaches World implementation;
- every decision is visible in the ledger.

---

# G4 — Cognitive MMU v0

## Goal

Stop treating conversation history as canonical memory.

## Deliverables

### Context Page

- page identity/type/source;
- token estimate;
- importance/relevance metadata;
- persistence;
- blob-backed large content.

### Working-set builder

Pack bounded context from:

- intent;
- task state;
- pinned pages;
- recent events;
- retrieved pages;
- tool schemas.

### `recall()` syscall

Explicit query over context/memory store.

### Context trace

Every model call records which pages were included and why.

### Basic eviction

Remove low-value pages from active context without deleting durable state.

## Killer test

Run a task whose accumulated history is larger than the model context and demonstrate that useful work continues using bounded context plus recall.

---

# G5 — Recursive Agent Processes

## Goal

Bring the Prime-style recursive-agent benefit into the kernel process model.

## Deliverables

### `spawn()`

Child contract includes:

- task;
- authority subset;
- budget;
- deadline;
- model policy;
- world policy;
- result contract.

### Messaging

- request/response;
- status;
- result;
- evidence reference;
- cancellation.

### Supervisor

- concurrent children with goroutines;
- durable child states;
- restart-safe waiting;
- cancellation tree.

### Budget hierarchy

- token/cost reservation;
- max children;
- max parallelism;
- release unused reservations.

## Killer test

A root agent delegates three repository investigations in parallel, receives structured evidence and survives a runtime restart while one child is still pending.

---

# G6 — Cognitive Scheduler v0

## Goal

Stop binding an Agent Process to one model.

## Deliverables

- model registry;
- model capabilities metadata;
- cost/latency estimates;
- rule-based task routing;
- fallback on provider failure;
- privacy/locality constraints;
- per-task model override;
- routing events and metrics.

## First routing policy

Simple deterministic rules are preferable to premature ML:

```text
cheap extraction → small/cheap model
large-context synthesis → suitable context model
high-risk architecture → strong reasoning model
provider unavailable → fallback
private/local-only → local compatible model
```

## Later

Learn routing from task outcomes once enough telemetry exists.

---

# G7 — OCI World + Agent Transactions

## Goal

Make speculative work safe and reversible.

## Deliverables

### OCI World

- isolated workspace mount;
- restricted network mode;
- CPU/memory/time limits;
- controlled environment variables;
- secret binding without prompt exposure.

### Transaction API

```text
begin
execute
verify
commit / rollback
```

### Verification contract

- command/test checks;
- policy checks;
- optional model evaluator;
- acceptance criteria mapping.

## Killer test

Agent makes a multi-file breaking change, verification fails, rollback restores the original observable workspace exactly.

---

# G8 — Cognitive Fork / Speculative Execution

## Goal

Explore alternative futures in parallel.

## Deliverables

- named checkpoint;
- fork Agent Process state;
- fork World state;
- fork memory overlay;
- allocate independent branch budgets;
- run branches concurrently;
- evaluator compares artifacts/results;
- promote winning branch;
- discard/retain losing branch history.

## Killer demonstration

Ask the agent to implement two solutions to the same performance problem, benchmark both in isolated forks, explain the comparison and commit only the winning solution.

---

# G9 — Epistemic Memory

## Goal

Replace flat memory snippets with evidence-aware beliefs.

## Deliverables

- belief store;
- scope;
- provenance;
- confidence;
- freshness;
- contradiction edges;
- dependency graph;
- status lifecycle;
- retrieval integrated with Cognitive MMU.

## Causal invalidation v1

When source evidence changes:

- mark directly derived beliefs stale;
- propagate `needs_review` through dependency edges;
- schedule verification when policy warrants it.

## Killer test

An agent learns a repository architecture fact, the source changes, and the old belief is automatically downgraded rather than silently reused.

---

# G10 — Context Faults + Cognitive MMU v2

## Goal

Make retrieval feel like a runtime primitive rather than a chat search command.

## Deliverables

- symbolic context references;
- dependency-driven page loading;
- context-fault events;
- loop protection;
- smarter working-set planning;
- structured compaction;
- summary ↔ raw evidence links.

## Research requirement

Keep automatic faults observable and debuggable. Never create hidden retrieval behavior that makes model decisions impossible to reproduce.

---

# G11 — Adaptive Teams + Agent Negotiation

## Goal

Move beyond static multi-agent graphs.

## Deliverables

- team proposal/decomposition format;
- scheduler approval against budget;
- temporary specialist profiles;
- structured challenge/evidence protocol;
- disagreement deadlines;
- escalation to parent/evaluator/user.

## Killer demonstration

A reviewer finds a race condition, the implementer disputes it, the reviewer produces a reproducer, and both converge on a corrected implementation without root-agent micromanagement.

---

# G12 — Verified Continual Improvement

## Goal

Allow the system to improve without uncontrolled self-modification.

## Deliverables

Versioned cognitive artifacts:

```text
skills
prompts
agent profiles
routing policies
context policies
```

Candidate lifecycle:

```text
propose → evaluate → compare baseline → promote/reject → rollback
```

Track:

- origin;
- hypothesis;
- evaluation count;
- quality metrics;
- regression metrics;
- version lineage.

## Hard invariant

Self-improvement cannot expand capabilities or rewrite immutable root security/user policy.

---

# G13 — Distributed Worlds / Workers

## Goal

Run the same Agent Process abstraction across local and remote compute.

## Candidates

- SSH worker;
- remote Go worker protocol;
- Kubernetes jobs/workspaces;
- GPU/local inference node routing;
- distributed object/blob storage;
- leases/heartbeats for active workers.

The durable process remains independent from worker location.

---

# G14 — Production Observability / Time-Travel Debugger

## Goal

Make complex agent behavior understandable.

## Deliverables

- process tree UI;
- causal event graph;
- context-page inspector;
- authority/effect inspector;
- cost/token timeline;
- world diff viewer;
- historical checkpoint replay;
- fork from past event;
- OpenTelemetry export.

A user should be able to answer:

> “Why did agent A make change X, based on what evidence, under which authority, and what would happen if I replayed from before that choice?”

---

# Suggested first product slice

Do not wait until G14 to have something usable.

A credible first coding-agent product can emerge around **G5**:

```text
single Go binary
persistent sessions
model streaming
filesystem/shell/git actions
Event Ledger
basic capabilities
bounded context
recursive subagents
```

But the project should resist polishing that CLI so heavily that it prevents development of G7–G10, where the strongest differentiation begins.

---

# Testing philosophy

Each kernel concept should have invariant tests.

Examples:

```text
process recovery is deterministic
child authority ⊆ parent authority
denied effect cannot reach a World
transaction rollback restores observable state
forks cannot mutate sibling worlds
budget cannot be oversubscribed
invalidated belief cannot rank as fully trusted
context builder never exceeds hard token budget
```

Use real model integration tests sparingly; most kernel semantics should be testable without an LLM.

---

# Benchmark/evaluation suite

As the system matures, maintain scenarios for:

- long-context repository exploration;
- multi-hour task recovery;
- provider failure mid-task;
- recursive-agent cost control;
- prompt-injection attempts;
- stale-memory invalidation;
- speculative implementation comparison;
- rollback correctness;
- model-router quality/cost tradeoff;
- continual-improvement regression detection.

This suite can become one of the project’s major assets.

---

# Immediate next step

Implement **G0 + G1 only** before adding provider integrations.

The most valuable first proof is not “the LLM can call `ls`”. Existing agents already prove that.

The first proof unique to this architecture is:

> **An Agent Process is a real durable runtime object whose state can be reconstructed, inspected and governed independently from a model conversation.**
