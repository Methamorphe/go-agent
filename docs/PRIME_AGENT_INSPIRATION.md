# Prime Agent Inspiration and Deliberate Departures

## Why Prime Agent matters

Prime Agent is the strongest conceptual inspiration discussed for this project because it reframes the agent from a chat loop into a persistent programmable system.

The most important ideas to preserve are not implementation details, but the mental model:

- a persistent execution environment;
- recursive delegation to child agents;
- long-running tasks;
- persistent state outside the immediate prompt;
- continual refinement of reusable agent behavior.

The goal of `go-agent` is not to clone Prime Agent. It is to generalize and harden the runtime concepts underneath it.

## Prime-style programmable environment

A conventional coding agent exposes many individual tools:

```text
LLM
 ├─ read_file
 ├─ write_file
 ├─ grep
 ├─ bash
 ├─ browser
 └─ git
```

Prime’s important shift is to give the model a persistent programmable environment, commonly represented by a Python/IPython kernel:

```text
LLM
  │
  ▼
Persistent programmable environment
  ├─ variables
  ├─ files
  ├─ shell
  ├─ parsing
  ├─ skills
  └─ child agents
```

That lets the model keep intermediate objects outside the prompt and manipulate large datasets with code before selecting what deserves LLM attention.

### What we keep

- persistent execution state;
- programmable manipulation of external context;
- long-lived handles to intermediate results;
- explicit child-agent creation;
- ability to continue after prompt compaction.

### What we change

The programmable REPL should be **one execution world**, not the kernel itself.

```text
Agent Kernel
  ├─ Python World
  ├─ Shell World
  ├─ Browser World
  ├─ Docker World
  ├─ SSH World
  └─ MCP World
```

This separates durable agent semantics from Python-specific runtime behavior.

## Recursive Language Model / subagent model

The second major Prime idea is recursive delegation.

A parent can create specialist children:

```text
Main Agent
  ├─ Architecture Agent
  ├─ Test Agent
  ├─ Security Agent
  └─ Database Agent
```

Each child gets its own context and can investigate a subproblem without polluting the parent’s working set.

This provides two advantages:

1. **parallelism** — independent investigations can progress concurrently;
2. **context isolation** — the parent receives summaries/evidence rather than every intermediate token.

### What we keep

- children as separate logical sessions/processes;
- parallel recursive work;
- explicit messaging/results;
- children able to create descendants when allowed.

### What we extend

A child should not merely receive a prompt. It should receive a complete runtime contract:

```text
Child Process
  ├─ task / intent
  ├─ capability subset
  ├─ budget
  ├─ model policy
  ├─ context policy
  ├─ world access
  ├─ deadline
  └─ parent/peer communication policy
```

This makes subagents governed processes rather than unconstrained prompt branches.

## Context as external state

One of Prime’s strongest ideas is that large objects do not need to live inside the model’s prompt.

Instead of placing a huge log file directly in context:

```text
LLM context
  └─ 2 GB log dump  ← impossible / wasteful
```

an execution environment can keep it externally:

```text
logs = parse(...)
errors = filter(logs)
stats = aggregate(errors)
```

and only feed relevant slices or conclusions to the model.

### Our extension: managed context virtualization

Prime demonstrates that state can live outside the context. `go-agent` proposes to make this a first-class runtime subsystem:

- context pages;
- working sets;
- pinning;
- eviction;
- summaries;
- evidence references;
- context faults;
- provenance-aware retrieval.

The model should not need to manually remember every object handle or rebuild context policy itself.

## Persistence and long-running agents

Prime demonstrates that the agent lifecycle does not have to equal the terminal lifecycle. A persistent supervisor/worker model allows an agent to remain alive and later be reattached.

That direction is essential.

### Our extension: process durability rather than worker longevity

The strongest guarantee is not “the worker stays alive”. It is:

> **The agent can be reconstructed after the worker dies.**

Therefore, durable state should be based on persisted events/snapshots, not only on an in-memory language runtime.

Desired property:

```text
kill -9 runtime
restart
resume <agent-id>
→ deterministic reconstruction of operational state
```

## Continual Harness / refinement

Prime’s continual refinement concept is another major inspiration: successful patterns can become reusable memories, skills, prompts or specialized subagents.

The opportunity is to make this improvement process safer and more scientific.

### Proposed extension: verified refinement

Instead of:

```text
experience → looks useful → permanently mutate harness
```

prefer:

```text
experience
   ↓
proposed refinement
   ↓
versioned candidate
   ↓
evaluation against baseline
   ↓
measurable improvement?
   ├─ yes → promote
   └─ no  → discard
```

Every promoted refinement should have:

- origin session;
- hypothesis;
- evaluation set;
- measured impact;
- version;
- rollback path.

## Where Prime is intentionally not copied

### 1. Python is not the privileged kernel

Python is powerful for dynamic code execution, data processing and RLM experiments, but the durable runtime should not inherit Python’s process model or package/runtime assumptions.

### 2. Security is not delegated to the model

A system prompt saying “do not access secrets” is not security.

The runtime should enforce:

- filesystem scopes;
- network scopes;
- secret isolation;
- child authority inheritance;
- effect approvals;
- container/world boundaries.

### 3. Context is not manually curated only by the agent

The kernel should actively manage context pressure and retrieval.

### 4. Subagents are resource-accounted

Every child should consume an explicit budget and inherit limits.

### 5. Side effects are typed

The runtime should know whether actions can be retried, rolled back or speculated.

### 6. Forking includes the world

Exploring two strategies should be able to create two independent filesystem/process/database states, not merely two LLM conversations.

## Relation to Pi

Pi is valuable as an example of a small, hackable agent harness with a narrow core. That simplicity is worth preserving as a cultural principle:

> Keep the kernel small and composable.

Where this project differs is that the small core is meant to be a **runtime kernel**, not simply a minimal coding-agent loop.

## Relation to Hermes-style agents

Hermes-like systems emphasize a broad operational agent experience: memory, skills, scheduled tasks, channels and integrations.

Those features are useful applications of a runtime, but they are deliberately not the initial innovation target here.

The project should first solve the substrate:

```text
Durability
Context
Authority
Effects
Worlds
Forks
Transactions
Scheduling
```

Then rich assistant/product integrations can be built above it.

## Summary

The core lesson retained from Prime Agent is:

> **Give the model a persistent, programmable world and let it recursively delegate work.**

The core hypothesis of this project is:

> **That idea becomes significantly more powerful when the persistent programmable world is backed by an explicit agent process model, virtualized context, typed effects, capability security, transactional execution and durable state recovery.**
