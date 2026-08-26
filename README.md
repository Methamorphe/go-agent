# go-agent

> **Product name: TBD** — `go-agent` is the working repository name only.

An experimental **durable runtime / kernel for autonomous AI agents**, written in Go.

The goal is not to build “another coding-agent CLI”, nor to port Prime Agent line-for-line to Go. The project explores a lower-level execution model for long-running intelligent processes: durable state, managed context, recursive agents, isolation, explicit authority, reversible actions, speculative execution, model scheduling, and verifiable self-improvement.

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

- [Vision and product thesis](docs/VISION.md)
- [Prime Agent inspiration and what we change](docs/PRIME_AGENT_INSPIRATION.md)
- [Target architecture and kernel model](docs/ARCHITECTURE.md)
- [Cognitive runtime, context and memory](docs/COGNITIVE_RUNTIME.md)
- [Security, authority, intents and effects](docs/SECURITY_AND_EFFECTS.md)
- [Orchestration, subagents and model scheduling](docs/ORCHESTRATION.md)
- [Innovation catalog / research agenda](docs/INNOVATIONS.md)
- [Implementation roadmap](docs/ROADMAP.md)

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

**Design / research phase.**

The repository currently documents the execution model first. Implementation should begin with a minimal vertical slice proving durable Agent Processes, Worlds, the Event Ledger, a small syscall surface and a model/tool loop before adding advanced cognition.

## Naming

The previous placeholder `Veyra` was discussed but is **not retained**. The final product name is intentionally open.
