# Vision and Product Thesis

## Working definition

`go-agent` is a research-driven attempt to build a **runtime for durable autonomous agents** rather than a monolithic coding assistant.

The project is inspired by the direction taken by modern agent harnesses such as Pi, Hermes and especially Prime Agent, but it aims to move one abstraction layer lower.

Prime’s central insight is powerful: a language model becomes much more capable when it is given a persistent programmable environment and can recursively delegate work to child agents. The opportunity here is to ask what runtime primitives are still missing if agents are expected to operate safely for hours, days or weeks.

## Problem statement

Current agent systems frequently conflate several concerns:

- conversation history;
- runtime state;
- memory;
- permissions;
- tool execution;
- sandboxing;
- orchestration;
- model selection;
- planning;
- background work;
- self-improvement.

This creates brittle systems where the LLM context becomes the de facto process memory and where the model itself is asked to enforce policies that should belong to the runtime.

The project instead treats an agent as an **intelligent process** managed by a kernel-like runtime.

## The core mental model

Traditional agent:

```text
LLM context ≈ process state
```

Target model:

```text
LLM context = hot cognitive working set
runtime state = durable source of truth
```

That distinction drives most of the architecture.

A long-lived agent should be able to:

- lose its current LLM context and recover;
- be killed and resumed;
- compact history without losing operational state;
- switch models without losing identity/state;
- spawn children with restricted authority;
- explore multiple hypotheses in isolated forks;
- rollback reversible actions;
- explain what evidence supports a belief;
- invalidate stale knowledge when the world changes;
- remain bounded by the user’s original intent;
- dynamically choose models, tools and parallelism;
- improve procedures without silently corrupting its own behavior.

## Product direction

The desired end state is not just a CLI. It is closer to a reusable runtime capable of hosting many agent applications:

```text
                 Agent Applications
              ┌─────────┼─────────┐
              ▼         ▼         ▼
           Coding    Research   DevOps
              │         │         │
              └─────────┼─────────┘
                        ▼
                 Harness Layer
                        │
                        ▼
                   Agent Kernel
                        │
              ┌─────────┼─────────┐
              ▼         ▼         ▼
           Worlds     Memory    Authority
```

The first application can absolutely be a coding agent, because coding provides excellent testability: repositories, builds, tests, diffs and benchmarks give objective verification signals. But the kernel must not hard-code assumptions that make it useful only for software engineering.

## What makes the project distinct

The differentiation should not be:

- “more providers”;
- “more tools”;
- “better prompt templates”;
- “another MCP client”;
- “another multi-agent graph library”.

The differentiation should come from the **execution model**.

### 1. Durable intelligent processes

An agent is addressable, resumable and replayable. It owns durable state independently from a particular terminal session or LLM request.

### 2. Context virtualization

The prompt window is managed like a constrained cache. The runtime determines which memories, objects, evidence and prior events become part of the active working set.

### 3. Explicit authority

An agent cannot infer its own permissions. It receives capabilities from a parent/user authority and may delegate only subsets of those capabilities.

### 4. Typed effects

The runtime understands whether an action is pure, read-only, reversible, compensatable or irreversible. That classification informs isolation, speculative execution and approval policy.

### 5. Reversible execution

Dangerous or uncertain work should happen inside forked worlds or transactions. Verify first, then commit.

### 6. Epistemic state

Memory should represent not only “what the agent remembers” but also “why it believes it”, how confident it is and what would invalidate it.

### 7. Cognitive scheduling

Models should be treated as heterogeneous compute resources. Not every task needs the largest or most expensive model.

### 8. Verified continual improvement

The system may propose changes to skills, prompts, routing policies or memory, but permanent promotion should be measurable, versioned and reversible.

## Design principles

### Runtime over prompt engineering

If a guarantee can be enforced structurally in the runtime, it should not be delegated to a system prompt.

Examples:

- filesystem isolation;
- network restrictions;
- budget limits;
- authority inheritance;
- irreversible-action approval;
- task cancellation;
- process durability.

### Small kernel, extensible worlds

The kernel should expose a compact set of stable primitives while execution environments remain pluggable.

```text
Kernel
  ├─ process
  ├─ state
  ├─ authority
  ├─ effects
  ├─ scheduling
  └─ syscalls

Worlds
  ├─ local OS
  ├─ Docker/OCI
  ├─ SSH
  ├─ browser
  ├─ Python
  ├─ Kubernetes
  └─ custom/MCP
```

### Deterministic state transitions where possible

LLMs are probabilistic; the runtime does not have to be.

Process lifecycle, transaction boundaries, capability inheritance, event recording, retries, timeouts and commits should have explicit deterministic semantics.

### Auditability by construction

Every meaningful state-changing action should be attributable to:

- a user intent;
- an agent/process;
- a model invocation;
- a tool/syscall;
- an authority grant;
- a set of evidence;
- an effect classification;
- a resulting event.

### Local-first, distributed-ready

The first version should work as one executable on a developer machine. The abstractions should later permit workers on remote hosts, GPU servers or Kubernetes without changing the agent-level programming model.

### Model/provider independence

No fundamental state should depend on one provider’s proprietary conversation representation. Provider-specific messages are an adapter concern.

## Why Go for the kernel

Go is a strong fit because the runtime is primarily an orchestration and systems problem rather than a numerical-computing problem.

The kernel will spend much of its time managing:

- network streams;
- child processes;
- cancellation trees;
- timers;
- concurrent tasks;
- event streams;
- resource accounting;
- persistent state;
- RPC;
- worker supervision.

Go offers these capabilities with relatively little accidental complexity and produces a simple distributable binary.

Python can remain a first-class guest environment rather than the host runtime.

## Non-goals

At least initially, the project should not attempt to:

- outperform model providers at inference;
- implement a vector database from scratch;
- replace Docker/Kubernetes;
- create a new programming language for agent prompts;
- build every integration itself;
- allow agents to silently bypass user intent;
- optimize microseconds in paths dominated by model/network latency.

## Success criteria

A convincing early version should prove that an agent can:

1. start a task;
2. create durable process state;
3. execute tools inside an explicit World;
4. record all state transitions in an event ledger;
5. stop completely;
6. resume without relying on the original in-memory process;
7. spawn a child with fewer capabilities;
8. fork an isolated alternative;
9. verify results;
10. commit one branch and discard another;
11. reconstruct only the necessary context to continue.

If those foundations are strong, richer agent behavior can be layered on top safely.
