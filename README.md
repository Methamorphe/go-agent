# go-agent

> **Product name: TBD** — `go-agent` is the working repository name only.

An experimental **durable runtime / kernel for autonomous AI agents**, written in Go.

The goal is not to build “another coding-agent CLI”, nor to port Prime Agent line-for-line to Go. The project explores a lower-level execution model for long-running intelligent processes: durable state, managed context, recursive agents, isolation, explicit authority, reversible actions, speculative execution, model scheduling, and verifiable self-improvement.

## Current phase

```text
A0  Architecture & Semantics                  COMPLETE
G0  Foundations                               COMPLETE
G1  Durable Agent Process + Event Ledger      COMPLETE
G2  Minimal Agent Loop + Agent Syscalls       COMPLETE
G3  Worlds + Authority + Effect System        COMPLETE
G4  Cognitive MMU v0                          COMPLETE
G5  Recursive Agent Processes                 COMPLETE
G6  Cognitive Scheduler v0                    COMPLETE
G7  Workspace/OCI World + Agent Transactions  READY
```

G6 is closed and validated. The integrated G0–G6 runtime passes cross-platform tests/vet/builds and the race detector, including restart-safe SQLite scheduler budget accounting. The next implementation generation is **G7 — Workspace/OCI World + Agent Transactions**.

See [Current Generation](docs/CURRENT_GENERATION.md), the generation exit reviews, and the [Implementation Roadmap](docs/ROADMAP.md).

The runtime is explicitly designed for **long-lived stability**: agent history may grow for hours or days, but hot memory, active LLM context and terminal rendering work must remain bounded.

## Vision

Most current agents are built around a loop:

```text
User → LLM → Tool → LLM → Tool → … → Answer
```

This project starts from a different question:

> **What would the operating model of a durable intelligent process look like if we designed it deliberately from first principles?**

The intended architecture is closer to:

```text
Applications / Harnesses
  ├─ Coding agent
  ├─ Research agent
  ├─ DevOps agent
  └─ Recursive/RLM harness
             │
             ▼
       Agent Runtime API
             │
             ▼
┌──────────────────────────────┐
│         Agent Kernel         │
├──────────────────────────────┤
│ Durable Agent Processes      │
│ Cognitive MMU / Context VM   │
│ Epistemic Memory             │
│ Authority & Intent Engine    │
│ Effect System                │
│ Transaction / Fork Manager   │
│ Cognitive Scheduler          │
│ Event Ledger                 │
└──────────────┬───────────────┘
               │
        Execution Worlds
               │
   Local / OCI / SSH / K8s
   Browser / Python / MCP / …
```

The interactive architecture deliberately separates the terminal from the runtime:

```text
Thin TUI / future IDE/Web clients
              │
          local IPC
              │
              ▼
      Durable Runtime Daemon
              │
     SQLite + Object Store
```

Closing or crashing the TUI must not kill an active agent.

## Why Go?

Go is the accepted kernel/runtime language.

It is deliberately chosen for:

- excellent concurrency primitives (`goroutine`, `channel`, `context.Context`);
- strong networking/process tooling;
- simple deployment as a single binary;
- fast compilation and iteration;
- low operational overhead;
- strong fit for daemons, schedulers, orchestration and distributed systems;
- sufficient performance for an agent runtime where model/network latency dominates;
- a good ecosystem for HTTP, SQLite, MCP and local tooling.

Python remains useful as an **Execution World** when a persistent REPL, notebooks or scientific/data tooling are needed. It does not need to be the kernel language.

## Core principles

1. **The LLM context is a cache, not the source of truth.**
2. **Agent state must survive process death and context compaction.**
3. **Authority is explicit, scoped, delegable and revocable.**
4. **Actions have typed effects.** The runtime must know what can be reversed, compensated or must require approval.
5. **Potentially dangerous work happens in Worlds whose guarantees are explicit.**
6. **Speculation is a first-class primitive.** Fork alternatives, verify them, then promote/commit the best one.
7. **Memory stores evidence-aware beliefs with provenance, freshness and invalidation — not just text snippets.**
8. **Subagents are processes with budgets and capabilities, not just prompts.**
9. **Models are schedulable compute resources.** Model identity is not Agent identity.
10. **Self-improvement must be evaluated, versioned and reversible.**
11. **The runtime should remain useful beyond coding agents.**
12. **The product name and final UX are intentionally deferred until the runtime is real.**
13. **Hot state is bounded.** History can grow; RAM/context/TUI work must not scale linearly with lifetime history.
14. **Backpressure is explicit.** No hidden unbounded queue is allowed in runtime hot paths.
15. **Unknown is a real state.** Partial external failures are never rewritten as false success/failure certainty.
16. **Presentation is disposable.** The terminal is a client, never the owner of canonical agent state.
17. **Restore never rewrites history.** Execution edits create new timelines and preserve already-observed effects.
18. **Evidence survives consolidation.** Memory transformations cannot launder provenance.
19. **Intent and capability are separate authorization dimensions.**
20. **Implementation follows contracts.** High-coupling semantic changes after A0 require explicit architecture updates.

## Primary innovations

The architecture currently defines these primitives:

- **Agent Process** — durable, resumable intelligent process.
- **Agent Syscalls** — provider/tool-independent kernel execution vocabulary.
- **Cognitive MMU / Context Virtualization** — bounded working set, semantic pages, page-in/out.
- **Context Fault** — typed runtime-mediated retrieval when required knowledge is not hot.
- **Epistemic Memory** — versioned evidence, beliefs, provenance, contradiction and Truth Maintenance.
- **Execution World** — environment boundary with explicit isolation/snapshot/network/resource guarantees.
- **Agent Fork / Cognitive Fork** — isolated alternative successor timelines.
- **Agent Transaction** — stage, verify, promote/rollback/reconcile.
- **Effect System** — pure/read/reversible/compensatable/irreversible semantics.
- **Authority Tree / Capability Leasing** — monotonic delegated authority.
- **Intent-Based Authority** — versioned Intent Envelope + Purpose-Carrying Actions + Action Proof.
- **Cognitive Scheduler** — per-task model/resource scheduling based on hard constraints, quality, cost, latency and load.
- **Agent Economy** — hierarchical reservations for tokens, money, time and concurrency.
- **Adaptive Team Formation** — temporary bounded specialist organizations.
- **Agent Negotiation** — finite evidence-oriented claim/challenge/escalation protocol.
- **Verified Continual Improvement** — versioned hypothesis/evaluation/shadow/canary/promotion/rollback lifecycle.
- **Safe Execution Editing** — checkpoint/fork/restore/merge semantics with causal frontier and uncertain-effect handling.

## Documentation

### Start here

- [Current implementation generation](docs/CURRENT_GENERATION.md)
- [Implementation roadmap](docs/ROADMAP.md)
- [A0 Exit Review — architecture gate result](docs/A0_EXIT_REVIEW.md)
- [G0 Exit Review — foundation gate result](docs/G0_EXIT_REVIEW.md)
- [G1 Exit Review — Durable Agent Process + Event Ledger](docs/G1_EXIT_REVIEW.md)
- [G2 Exit Review — Minimal Agent Loop + Agent Syscalls](docs/G2_EXIT_REVIEW.md)
- [G3 Exit Review — Worlds + Authority + Effect System](docs/G3_EXIT_REVIEW.md)
- [G4 Exit Review — Cognitive MMU v0](docs/G4_EXIT_REVIEW.md)
- [G5 Exit Review — Recursive Agent Processes](docs/G5_EXIT_REVIEW.md)
- [G6 Exit Review — Cognitive Scheduler v0](docs/G6_EXIT_REVIEW.md)
- [Cognitive Scheduler v0 runtime configuration](docs/COGNITIVE_SCHEDULER_V0.md)
- [Long-duration benchmark suite](docs/LONG_DURATION_BENCHMARKS.md)
- [Architecture diagrams](docs/ARCHITECTURE_DIAGRAMS.md)
- [Canonical concept status registry](docs/CONCEPT_STATUS.md)
- [Architecture gate](docs/ARCHITECTURE_GATE.md)
- [Architecture decisions](docs/ARCHITECTURE_DECISIONS.md)
- [Foundation technical decisions for G0](docs/FOUNDATION_TECHNICAL_DECISIONS.md)
- [Original concept architecture checklist](docs/CONCEPT_CONTRACTS.md) — historical per-concept design notes; current status labels are superseded by `CONCEPT_STATUS.md` and the exit reviews
- [Vision and product thesis](docs/VISION.md)
- [Product and runtime requirements](docs/REQUIREMENTS.md)

### Core runtime contracts

- [Target architecture and kernel model](docs/ARCHITECTURE.md)
- [Agent Process state machine](docs/AGENT_PROCESS_STATE_MACHINE.md)
- [Event model, ordering and initial catalog](docs/EVENT_MODEL_AND_CATALOG.md)
- [State, persistence and storage](docs/STATE_PERSISTENCE_AND_STORAGE.md)
- [Concurrency, supervision and backpressure](docs/CONCURRENCY_AND_BACKPRESSURE.md)
- [Failure model and recovery semantics](docs/FAILURE_MODEL_AND_RECOVERY.md)
- [World action and Effect protocol](docs/WORLD_ACTION_AND_EFFECT_PROTOCOL.md)
- [Execution Worlds and platform contract](docs/EXECUTION_WORLDS_PLATFORM_CONTRACT.md)
- [Capability, delegation and Intent model](docs/CAPABILITY_AND_INTENT_MODEL.md)
- [Intent-Based Authority engine](docs/INTENT_BASED_AUTHORITY_ENGINE.md)
- [Agent Transactions and Cognitive Forks](docs/TRANSACTIONS_AND_COGNITIVE_FORKS.md)
- [Execution edit safety](docs/EXECUTION_EDIT_SAFETY.md)
- [Recursive orchestration protocol](docs/RECURSIVE_ORCHESTRATION_PROTOCOL.md)

### Cognitive architecture

- [Cognitive runtime, context and memory](docs/COGNITIVE_RUNTIME.md)
- [Cognitive MMU v0 algorithm](docs/COGNITIVE_MMU_V0_ALGORITHM.md)
- [Context Faults and cognitive paging](docs/CONTEXT_FAULTS_AND_COGNITIVE_PAGING.md)
- [Epistemic Memory and Truth Maintenance](docs/EPISTEMIC_MEMORY_AND_TRUTH_MAINTENANCE.md)
- [Cognitive Scheduler architecture](docs/COGNITIVE_SCHEDULER_ARCHITECTURE.md)
- [Orchestration, subagents and model scheduling overview](docs/ORCHESTRATION.md)
- [Verified Continual Improvement](docs/VERIFIED_CONTINUAL_IMPROVEMENT.md)

### Long-session UI/performance

- [Reliability, stability and long-session performance](docs/RELIABILITY_AND_PERFORMANCE.md)
- [TUI, streaming, attach/detach and history virtualization](docs/TUI_AND_STREAMING.md)
- [Local runtime control / IPC protocol](docs/LOCAL_CONTROL_PROTOCOL.md)
- [Testing, benchmarks and quality gates](docs/TESTING_BENCHMARKS_AND_QUALITY_GATES.md)
- [Executable long-duration benchmarks](docs/LONG_DURATION_BENCHMARKS.md)

### Research and roadmap

- [Prime Agent inspiration and what we change](docs/PRIME_AGENT_INSPIRATION.md)
- [Innovation catalog / research agenda](docs/INNOVATIONS.md)
- [Implementation roadmap](docs/ROADMAP.md)

## Long-session reliability objective

A session should be able to run for hours/days without becoming progressively heavier simply because it is old.

The architecture requires:

```text
bounded LLM working context
bounded in-memory caches
bounded queues
streaming tool/model I/O
large payloads in object storage
snapshots + tail replay
paginated/virtualized TUI history
coalesced token rendering
centralized durable timers
explicit concurrency/resource budgets
```

The anti-regression plan targets 1h/8h/24h soak tests, daemon kill/restart tests, slow-TUI tests, multi-GB tool-output tests and synthetic large histories. The executable long-duration baseline covers real SQLite reopen/checkpoint durability, Cognitive MMU boundedness against a persisted large corpus, and restart-safe scheduler budget accounting; representative 1h/8h/24h runs remain empirical validation work.

## Implemented runtime baseline

The current G0–G6 implementation includes:

```text
Go kernel
modernc.org/sqlite behind internal adapter
database/sql + explicit SQL
SQLite WAL + foreign_keys + reliability-first synchronous mode
SHA-256 content-addressed Object Store
Unix domain socket / Windows named pipe IPC
length-framed versioned JSON control protocol
Durable Agent Process + Event Ledger
provider-neutral invocation layer + streaming
Agent Syscalls
LocalWorld + authority/effect gate
Cognitive MMU v0
Recursive Agent Processes + hierarchical budgets
Cognitive Scheduler v0 + durable root model budgets
```

## Scope

### In scope

- local-first agent runtime;
- persistent sessions/processes;
- recursive/subagent execution;
- model/provider abstraction;
- shell/filesystem/git/process tooling;
- MCP and external-tool integration;
- isolated execution Worlds;
- durable event/state model;
- context virtualization and structured memory;
- capability/security model;
- speculative forks and transactional workflows;
- observability and replay;
- distributed execution later.

### Explicitly not the initial goal

- cloning every feature of Prime/Hermes/Pi;
- supporting every model provider from day one;
- shipping dozens of chat integrations;
- building a generic no-code automation platform;
- inventing a GUI before the kernel is sound;
- allowing unconstrained autonomous production changes.

## Status

**A0–G6 complete. G7 — Workspace/OCI World + Agent Transactions ready.**

The next implementation step is to add isolated Workspace/OCI Worlds and transaction boundaries so speculative work can be verified, committed, rolled back or reconciled without weakening the durable process, authority, MMU, orchestration or scheduler semantics already established.

## Naming

The previous placeholder `Veyra` was discussed but is **not retained**. The final product name is intentionally open.