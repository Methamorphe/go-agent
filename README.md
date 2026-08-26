# go-agent

> **Product name: TBD** — `go-agent` is the working repository name only.

An experimental **durable runtime / kernel for autonomous AI agents**, written in Go.

The goal is not to build “another coding-agent CLI”, nor to port Prime Agent line-for-line to Go. The project explores a lower-level execution model for long-running intelligent processes: durable state, managed context, recursive agents, isolation, explicit authority, reversible actions, speculative execution, model scheduling, and verifiable self-improvement.

## Current phase

**A0 — Architecture & Semantics. No runtime development yet.**

Before implementing G0, every important primitive must be architected in terms of:

- problem and rationale;
- semantics/state machine;
- canonical vs ephemeral state;
- invariants;
- concurrency ownership and backpressure;
- failure/recovery behavior;
- authority/effects;
- performance/resource bounds;
- observability;
- tests and benchmarks.

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
   Local / Docker / SSH / K8s
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

Go is deliberately chosen for the runtime/kernel layer:

- excellent concurrency primitives (`goroutine`, `channel`, `context.Context`);
- strong networking/process tooling;
- simple deployment as a single binary;
- fast compilation and iteration;
- low operational overhead;
- good fit for daemons, schedulers, orchestration and distributed systems;
- enough performance for an agent runtime, where network/model latency dominates;
- official or mature integrations exist for modern model APIs and MCP-style tooling.

Python remains useful as an **execution world** when a persistent REPL, notebooks or scientific/data tooling are needed. It does not need to be the kernel language.

## Core principles

1. **The LLM context is a cache, not the source of truth.**
2. **Agent state must survive process death and context compaction.**
3. **Authority is explicit, scoped, delegable and revocable.**
4. **Actions have typed effects.** The runtime must know what can be reversed, compensated or must require approval.
5. **Potentially dangerous work happens in isolated worlds.**
6. **Speculation is a first-class primitive.** Fork alternatives, verify them, then commit the best one.
7. **Memory stores beliefs with provenance, confidence and invalidation — not just text snippets.**
8. **Subagents are processes with budgets and capabilities, not just prompts.**
9. **Models are schedulable compute resources.** Pick the cheapest/fastest model able to solve a given task.
10. **Self-improvement must be evaluated, versioned and reversible.**
11. **The runtime should remain useful beyond coding agents.**
12. **The product name and UX are intentionally deferred until the execution model is solid.**
13. **Hot state is bounded.** History can grow; RAM/context/TUI work must not scale linearly with lifetime history.
14. **Backpressure is explicit.** No hidden unbounded queue is allowed in runtime hot paths.
15. **Unknown is a real state.** Partial external failures are never rewritten as false success/failure certainty.
16. **Presentation is disposable.** The terminal is a client, never the owner of canonical agent state.

## Primary innovations under exploration

The project currently focuses on these proposed primitives:

- **Agent Process** — durable, resumable intelligent process.
- **Agent Syscalls** — stable low-level interface such as `observe`, `recall`, `spawn`, `delegate`, `execute`, `checkpoint`, `fork`, `commit`, `signal`, `sleep`.
- **Cognitive MMU / Context Virtualization** — context pages, working sets, page-in/page-out, pinning and context faults.
- **Context Fault** — runtime-mediated retrieval when required knowledge is outside the current LLM context.
- **Epistemic Memory** — beliefs + provenance + confidence + dependencies + contradiction/invalidation tracking.
- **Execution World** — isolated environment in which an agent can act (local, container, SSH, browser, Python, Kubernetes…).
- **Agent Fork / Cognitive Fork** — clone cognitive + environmental state to explore alternatives in parallel.
- **Agent Transaction** — speculative actions followed by verification and commit/rollback.
- **Effect System** — classify actions as pure/read/reversible/compensatable/irreversible.
- **Authority Tree / Capability Leasing** — children can only receive a subset of the parent’s authority, optionally with expiry.
- **Intent Lock / Intent-Based Authority** — immutable user goal and allowed effects constrain downstream actions.
- **Cognitive Scheduler** — route tasks across models/agents based on quality, cost, latency and prior success.
- **Agent Economy** — explicit budgets for tokens, money, time, tools and parallelism.
- **Adaptive Team Formation** — agents create temporary specialist teams based on the task.
- **Agent Negotiation** — peers can exchange evidence, challenge assumptions and escalate unresolved disagreements.
- **Verified Continual Improvement** — proposed skills/prompts/memories are evaluated before promotion.

## Documentation

### Start here

- [Architecture gate / pre-development A0 phase](docs/ARCHITECTURE_GATE.md)
- [Master architecture contracts for all 25 concepts](docs/CONCEPT_CONTRACTS.md)
- [Architecture decisions and open decisions](docs/ARCHITECTURE_DECISIONS.md)
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
- [Capability, delegation and Intent model](docs/CAPABILITY_AND_INTENT_MODEL.md)
- [Agent Transactions and Cognitive Forks](docs/TRANSACTIONS_AND_COGNITIVE_FORKS.md)

### Long-session UI/performance

- [Reliability, stability and long-session performance](docs/RELIABILITY_AND_PERFORMANCE.md)
- [TUI, streaming, attach/detach and history virtualization](docs/TUI_AND_STREAMING.md)
- [Local runtime control / IPC protocol](docs/LOCAL_CONTROL_PROTOCOL.md)
- [Testing, benchmarks and quality gates](docs/TESTING_BENCHMARKS_AND_QUALITY_GATES.md)

### Cognitive/security/orchestration architecture

- [Cognitive runtime, context and memory](docs/COGNITIVE_RUNTIME.md)
- [Cognitive MMU v0 algorithm](docs/COGNITIVE_MMU_V0_ALGORITHM.md)
- [Security, authority, intents and effects](docs/SECURITY_AND_EFFECTS.md)
- [Orchestration, subagents and model scheduling](docs/ORCHESTRATION.md)

### Research and roadmap

- [Prime Agent inspiration and what we change](docs/PRIME_AGENT_INSPIRATION.md)
- [Innovation catalog / research agenda](docs/INNOVATIONS.md)
- [Implementation roadmap](docs/ROADMAP.md)

## Long-session reliability objective

A session should be able to run for hours/days without becoming progressively heavier simply because it is old.

The architecture therefore requires:

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

The anti-regression suite is designed around 1h/8h/24h soak tests, daemon kill/restart tests, slow-TUI tests, multi-GB tool-output tests and synthetic 100k-message histories.

## Scope

### In scope

- local-first agent runtime;
- persistent sessions/processes;
- recursive/subagent execution;
- model/provider abstraction;
- shell/filesystem/git/process tooling;
- MCP and external-tool integration;
- isolated execution worlds;
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

**A0 — architecture / semantics / research phase.**

The repository is intentionally staying out of implementation until the high-coupling runtime contracts are strong enough that G0/G1 can be coded without inventing fundamental semantics on the fly.

## Naming

The previous placeholder `Veyra` was discussed but is **not retained**. The final product name is intentionally open.
