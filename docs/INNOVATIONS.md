# Innovation Catalog / Research Agenda

## Purpose

This document collects the concepts discussed as potential differentiators for the project.

Some ideas have precedents in research, systems engineering or emerging agent frameworks. The goal is **not** to claim that every primitive is globally unprecedented. The opportunity is to define a coherent execution model, useful semantics and production-grade implementation around combinations that are not yet standardized in the agent ecosystem.

The project should be judged by whether its abstractions become useful and defensible — not by whether a keyword has literally never appeared before.

---

## 1. Agent Process

### Problem

Current agents are often identified with a chat/session object tied to a provider or local process.

### Proposed primitive

A durable logical intelligent process with:

- identity;
- lifecycle;
- intent;
- authority;
- budget;
- model policy;
- world binding;
- memory scope;
- event history.

### Innovation opportunity

Define stable process semantics independent from provider threads and OS-process lifetime.

---

## 2. Agent Syscalls

### Problem

Agent frameworks expose heterogeneous tool/function/plugin APIs without a common low-level execution model.

### Proposed primitive

A compact kernel vocabulary:

```text
observe
recall
execute
spawn
message
signal
checkpoint
fork
verify
commit
rollback
sleep
```

### Innovation opportunity

Create a provider/tool-independent ABI for intelligent processes, analogous in spirit to OS syscalls.

The syscall layer becomes the enforcement point for:

- capabilities;
- effects;
- budget;
- tracing;
- transactions;
- replay.

---

## 3. Cognitive MMU

### Problem

Context windows are commonly managed as prompt-construction utilities rather than as a runtime subsystem.

### Proposed technology

A Cognitive Memory Management Unit responsible for a bounded LLM working set.

Primitives:

- Context Page;
- working set;
- page-in;
- page-out;
- pinning;
- semantic address/reference;
- page dependency;
- Context Fault.

### Innovation opportunity

Move “context engineering” from ad-hoc prompt logic into an explicit, observable kernel component.

---

## 4. Context Fault

### Problem

When an agent needs information outside the current context, today it must manually remember to search/re-read it.

### Proposed primitive

A runtime-recognized missing-knowledge event:

```text
agent needs X
    ↓
Context Fault
    ↓
resolve semantic reference/query
    ↓
load pages
    ↓
continue
```

### Research questions

- explicit only (`recall`) or automatically detectable?
- how to avoid pathological fault loops?
- should page loads happen between model invocations only?
- can tool schemas expose lazy cognitive references?
- how does fault handling differ across streaming APIs?

---

## 5. Epistemic Memory

### Problem

Most agent memory systems store snippets/documents without representing why the agent believes them.

### Proposed primitive

A belief record containing:

```text
statement
scope
confidence
provenance
freshness
status
dependencies
contradictions
```

### Innovation opportunity

Make memory evidence-aware and suitable for long-lived evolving environments.

---

## 6. Causal Invalidation / Agent Truth Maintenance

### Problem

Long-lived memories become silently stale after source systems change.

### Proposed technology

Dependency graph between beliefs/decisions and their evidence.

```text
Evidence A changes
    ↓
Belief B invalidated
    ↓
Decision C marked needs_review
    ↓
Derived Belief D downgraded
```

### Innovation opportunity

A practical Truth Maintenance System specialized for autonomous agents and software/tool evidence.

---

## 7. Cognitive Fork

### Problem

Agents usually explore alternatives only as text branches while operating on a shared real world.

### Proposed primitive

Fork complete recoverable agent + environment state.

Potential forked state:

- process metadata;
- active context references;
- memory overlay;
- filesystem/world snapshot;
- processes/containers;
- DB snapshot where supported;
- budget reservation.

### Innovation opportunity

Make speculative reasoning and speculative execution one unified primitive.

---

## 8. Agent Transaction

### Problem

Autonomous agents can accumulate partial changes before discovering that an approach is wrong.

### Proposed primitive

```text
begin
  actions
  actions
verify
  success → commit
  failure → rollback
```

### Innovation opportunity

Define transaction semantics across heterogeneous agent effects rather than only database writes.

Important distinction: true rollback is impossible for many external effects, so the transaction model must understand effect classes.

---

## 9. Effect System

### Problem

Frameworks frequently reduce tool safety to permission yes/no or a generic “dangerous” flag.

### Proposed primitive

Typed actions:

```text
Pure
Read
Reversible
Compensatable
Irreversible
```

Potential extra metadata:

```text
idempotent
retryable
commutative
external-visible
requires-secret
cost-bearing
```

### Innovation opportunity

Use effect typing to automatically decide whether execution can be:

- parallelized;
- retried;
- speculated;
- rolled back;
- deferred;
- auto-approved.

---

## 10. Intent Lock

### Problem

Long-running autonomous agents can drift away from the original user goal, and prompt-injected content may manipulate planning.

### Proposed primitive

An immutable root Intent object carrying:

- goal;
- allowed effect domains;
- forbidden domains;
- acceptance criteria;
- constraints;
- originating authority.

Plans may change. Root intent does not.

### Innovation opportunity

Make user intent a runtime security object rather than merely initial conversation text.

---

## 11. Intent-Based Authority

### Problem

A capability can be technically valid but irrelevant to the current user request.

### Proposed rule

An action requires:

```text
capability authorization
AND
intent compatibility
```

### Innovation opportunity

A second security dimension that limits task drift and prompt injection even when a broad capability exists.

---

## 12. Authority Tree / Capability Leases

### Problem

Subagents often inherit the parent’s entire environment and credentials.

### Proposed primitive

Strictly decreasing authority inheritance with temporary leases.

```text
Parent capability set
      ↓ subset only
Child capability set
      ↓ subset only
Grandchild
```

Leases can expire by task, time, transaction or revocation.

### Innovation opportunity

Treat authority as a resource that flows through an agent tree and can be audited causally.

---

## 13. Execution Worlds

### Problem

Agents are tightly coupled to local shell/filesystem assumptions.

### Proposed primitive

Uniform environment contract over:

- local workspace;
- OCI/container;
- SSH host;
- Kubernetes pod/job;
- browser;
- Python kernel;
- external MCP service.

### Innovation opportunity

Let agent reasoning operate against an abstract World while the kernel chooses isolation/location semantics.

---

## 14. World Forking

### Problem

Speculative branches require isolated side effects.

### Proposed technology

World implementations expose snapshot/fork semantics where possible.

Examples:

```text
git worktree
OverlayFS snapshot
OCI layer/container clone
DB snapshot/transaction
copy-on-write workspace
remote ephemeral VM
```

### Innovation opportunity

Common snapshot/fork semantics across heterogeneous environments.

---

## 15. Cognitive Scheduler

### Problem

Many agents bind a session to one selected model, wasting cost/latency and losing resilience.

### Proposed technology

A scheduler routes each cognitive task based on:

- expected quality;
- cost;
- latency;
- context requirement;
- privacy;
- provider health;
- historical performance;
- task class.

### Innovation opportunity

Treat LLMs as heterogeneous compute units and build learned scheduling policy from actual agent outcomes.

---

## 16. Agent Economy

### Problem

Recursive agents can explode in cost and concurrency.

### Proposed primitive

Hierarchical resource accounting:

```text
money
tokens
time
children
parallelism
tools
storage
external quotas
```

### Innovation opportunity

Budget becomes delegable authority. Child creation reserves finite parent resources.

---

## 17. Adaptive Team Formation

### Problem

Static multi-agent role graphs encode workflows prematurely.

### Proposed technology

Generate a temporary organization based on the problem:

```text
Lead
  ├─ DB
  ├─ Runtime
  └─ Network
```

and dissolve it after task completion.

### Innovation opportunity

The topology itself becomes a runtime decision optimized by past performance and available resources.

---

## 18. Agent Negotiation Protocol

### Problem

Reviewer/implementer patterns often terminate after one response, missing the value of adversarial evidence exchange.

### Proposed technology

Structured peer dialogue:

```text
claim
challenge
evidence
counterexample
revision
agreement/escalation
```

### Innovation opportunity

A bounded protocol that improves correctness without routing every dispute through a root agent.

---

## 19. Verified Continual Improvement

### Problem

Self-modifying prompts/skills can cause silent regressions and behavioral drift.

### Proposed technology

Treat improvements like software changes:

```text
hypothesis
candidate version
evaluation
baseline comparison
promotion
rollback
```

### Innovation opportunity

A/B testing and evidence-based promotion for agent cognition/configuration.

---

## 20. Cognitive Artifact Versioning

### Proposed extension

Skills, routing rules, memory policies and prompts become versioned artifacts with lineage:

```text
skill/auth-debugger@v8
prompt/reviewer@v4
routing/code-review@v3
```

Each version records provenance and evaluation metrics.

This creates a foundation for reproducible agent behavior.

---

## 21. Replayable Cognition

### Problem

Agent failures are hard to debug from chat transcripts alone.

### Proposed technology

Replay the deterministic runtime envelope while optionally substituting model outputs.

Possible modes:

```text
exact event replay
replay until model call N
replace model result
fork from historical checkpoint
compare outcomes
```

### Innovation opportunity

“Time-travel debugging” for intelligent processes.

---

## 22. Causal Agent Trace

Every action can form a causal graph:

```text
User Intent
  ↓
Plan decision
  ↓
Child spawn
  ↓
Evidence
  ↓
Belief
  ↓
Action request
  ↓
Capability authorization
  ↓
World effect
```

A UI can answer “why did this happen?” by walking edges rather than reconstructing reasoning from raw text.

---

## 23. Durable Agent Sleep

### Problem

Long-lived agents should not consume memory/processes while waiting days for something.

### Proposed primitive

`sleep()` persists wake conditions and releases active compute.

```text
Agent → sleep(condition/time/event)
      → no resident inference process required
      → wake event reconstructs process
```

### Innovation opportunity

Treat time/event waiting as durable process state, not a daemon polling loop embedded in the agent.

---

## 24. Model-independent Agent Identity

An Agent Process should retain continuity even if it moves:

```text
Model A → Model B → local model → frontier model
```

Memory, intent, authority and process state remain unchanged.

The model is an execution resource, not the agent’s identity.

---

## 25. Safety via Architecture, not Personality

A foundational research/product position:

```text
LLM behavior = probabilistic
Kernel boundaries = deterministic
```

The project should continuously look for guarantees that can be moved from prompts into:

- type systems;
- policy engines;
- capability checks;
- execution isolation;
- transactional semantics;
- event constraints.

---

# Highest-priority invention set

If only five novel primitives are pursued initially, prioritize:

1. **Agent Process** — durable/resumable intelligent process.
2. **Cognitive MMU + Context Faults** — bounded managed context.
3. **Execution World** — explicit isolated world boundary.
4. **Cognitive Fork + Agent Transaction** — speculate, verify, commit/rollback.
5. **Authority + Intent + Effect model** — deterministic safety substrate.

These five form a coherent kernel. Model scheduling, epistemic memory, swarms and self-improvement become substantially stronger once those primitives exist.

# Research discipline

For every claimed innovation, track:

```text
problem
existing approaches
proposed abstraction
semantics/invariants
prototype
benchmark/evaluation
failure modes
comparison
```

Avoid marketing claims such as “first ever” until a dedicated prior-art review has been performed.
