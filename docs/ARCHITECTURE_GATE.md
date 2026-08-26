# Architecture Gate — Pre-Development Design Phase

## Status

**Mandatory before implementation.**

The project deliberately inserts an architecture phase before `G0`.

Working name for this phase:

```text
A0 — Architecture & Semantics
```

No production runtime code should be started merely because a concept sounds good.

---

# 1. Why an architecture gate?

This project combines several difficult systems problems:

- durable state machines;
- long-running concurrency;
- untrusted probabilistic model output;
- side effects and external systems;
- context/memory management;
- recursive work graphs;
- local persistence;
- interactive streaming UI;
- sandboxing/security;
- speculative execution;
- self-modification.

Implementing these directly from a high-level roadmap would lock accidental semantics into code.

A0 exists to decide **what each primitive means** before choosing how to implement it.

---

# 2. Definition of “architected”

A concept is ready for implementation only when its specification answers all of the following.

## Problem

- What concrete failure/limitation of current agents does it solve?
- Is it kernel-level or harness/product-level?
- What happens if we do not build it?

## Semantics

- What is the primitive?
- What is it explicitly not?
- What lifecycle/state machine does it have?
- What operations exist?

## Canonical state

- What data must survive crash/restart?
- What is ephemeral?
- What belongs in SQLite?
- What belongs in object/blob storage?

## Invariants

- Which properties must never be violated?
- Which can be checked synchronously by the kernel?
- Which are eventual/best effort?

## Concurrency

- Who owns execution?
- Can operations overlap?
- What locks/version checks are required?
- What queue/buffer is used and how is it bounded?

## Failure model

- What can fail before/during/after an effect?
- Can outcome become unknown?
- Is retry safe?
- What happens after daemon crash?

## Security

- What capabilities are required?
- What effect class applies?
- What intent constraints apply?
- Can a child delegate the operation?

## Performance

- What grows with agent lifetime?
- What must remain bounded?
- What is the hot path?
- What data must stream instead of buffering?

## Observability

- Which canonical events are emitted?
- Which metrics matter?
- How does a user answer “why did this happen?”

## Validation

- Which unit/invariant tests prove semantics?
- Which integration/fault tests prove recovery?
- Which benchmark/soak test proves performance claims?

## Evolution

- What is deliberately deferred?
- Which interfaces are private/unstable?
- How can semantics evolve without breaking persisted state?

---

# 3. A0 workstreams

## A0.1 — Runtime/process semantics

Concepts:

- Agent Process;
- process lifecycle;
- parent/child lineage;
- Agent Syscalls;
- durable sleep/wake;
- model-independent identity.

Required outputs:

- complete process state machine;
- syscall request/response contracts;
- state/version concurrency model;
- cancellation semantics;
- event mapping;
- recovery table.

Status: **partially designed**.

Primary docs:

- `ARCHITECTURE.md`
- `STATE_PERSISTENCE_AND_STORAGE.md`
- `CONCURRENCY_AND_BACKPRESSURE.md`
- `FAILURE_MODEL_AND_RECOVERY.md`

---

## A0.2 — Persistence and event semantics

Concepts:

- Event Ledger;
- snapshots;
- projections;
- object store;
- replayable cognition;
- causal trace.

Required outputs:

- event ordering model;
- append transaction semantics;
- reducer contract;
- snapshot compatibility/versioning;
- object finalization and GC;
- replay modes;
- corruption behavior.

Status: **partially designed; schema/event catalog still required**.

---

## A0.3 — Context virtualization

Concepts:

- Cognitive MMU;
- Context Page;
- working set;
- Context Fault;
- compaction;
- context observability.

Required outputs:

- page schema;
- page identity/reference semantics;
- token-budget algorithm;
- pin/eviction rules;
- recall API;
- fault-loop prevention;
- ranking baseline;
- long-session benchmark.

Status: **conceptually strong; algorithms/contracts not frozen**.

---

## A0.4 — Epistemic memory

Concepts:

- working/episodic/semantic/procedural memory;
- belief model;
- provenance;
- confidence;
- contradiction;
- causal invalidation / truth maintenance.

Required outputs:

- belief lifecycle;
- evidence graph schema;
- invalidation algorithm;
- conflict-resolution policy;
- retrieval integration;
- scope/security rules;
- stale-memory eval corpus.

Status: **research/design required**.

---

## A0.5 — Authority, intent and effect model

Concepts:

- capabilities;
- Authority Tree;
- capability leases;
- Intent Lock;
- Intent-Based Authority;
- Effect System;
- risk/approval barriers;
- secret isolation.

Required outputs:

- capability grammar;
- delegation/subset algorithm;
- effect descriptor schema;
- policy evaluation order;
- intent representation;
- approval token semantics;
- secret-provider contract;
- adversarial test matrix.

Status: **conceptually strong; formal schemas and policy order required**.

---

## A0.6 — Execution Worlds

Concepts:

- World interface;
- Local World;
- OCI World;
- Python World;
- SSH/Kubernetes/browser later;
- World capability levels;
- world recovery.

Required outputs:

- minimal non-overabstract World contract;
- action/result protocol;
- resource limits;
- process-tree cancellation;
- filesystem/network semantics;
- snapshot/fork capability discovery;
- host-platform differences.

Status: **requires detailed design before G3/G7**.

---

## A0.7 — Transactions and Cognitive Forks

Concepts:

- Agent Transaction;
- checkpoint;
- World snapshot;
- Cognitive Fork;
- branch evaluation;
- promotion/rollback;
- irreversible barriers.

Required outputs:

- transaction state machine;
- effect staging model;
- commit protocol;
- unknown-outcome recovery;
- branch memory/context isolation;
- evaluator contract;
- cleanup/retention policy;
- rollback oracle tests.

Status: **research/design required; one of the main differentiators**.

---

## A0.8 — Recursive orchestration

Concepts:

- spawn;
- messaging;
- adaptive teams;
- agent negotiation;
- parent/child waiting;
- result/evidence contracts.

Required outputs:

- child creation protocol;
- structured result contract;
- messaging semantics;
- cancellation propagation;
- peer-communication authority;
- deadlock/loop prevention;
- fan-out limits.

Status: **partially designed**.

---

## A0.9 — Cognitive Scheduler and Agent Economy

Concepts:

- model registry;
- routing policy;
- provider health;
- cost/latency/quality tradeoffs;
- hierarchical budgets;
- resource reservations;
- fairness.

Required outputs:

- model capability schema;
- task classification format;
- deterministic v0 routing algorithm;
- fallback rules;
- accounting units;
- reservation/settlement semantics;
- fairness scheduler;
- evaluation baseline.

Status: **partially designed**.

---

## A0.10 — TUI and local control protocol

Concepts:

- daemon/runtime separation;
- attach/detach;
- presentation projections;
- viewport virtualization;
- stream coalescing;
- approval UX;
- multi-client future.

Required outputs:

- IPC envelope;
- subscription/cursor semantics;
- conversation block schema;
- viewport query API;
- slow-client overflow policy;
- reconnect protocol;
- performance benchmark fixtures.

Status: **architectural direction defined; protocol contract still required**.

---

## A0.11 — Reliability/performance

Concepts:

- bounded queues;
- backpressure;
- streaming object storage;
- snapshot/replay bounds;
- goroutine discipline;
- resource budgets;
- profiling;
- soak tests.

Required outputs:

- hard/soft budgets;
- stream capacities/overflow policies;
- initial performance baselines;
- memory growth acceptance rules;
- profiling endpoints/build options;
- CI benchmark strategy.

Status: **principles and target tests defined; numbers to calibrate with prototypes**.

---

## A0.12 — Verified continual improvement

Concepts:

- versioned cognitive artifacts;
- hypothesis/candidate evaluation;
- promotion;
- rollback;
- no authority expansion.

Required outputs:

- artifact version model;
- eval result schema;
- promotion policy;
- reproducibility requirements;
- security non-regression gate;
- rollback semantics.

Status: **late-stage design; architecture must reserve clean extension points**.

---

# 4. Architecture decision records (ADRs)

Important irreversible/expensive decisions should have an ADR before implementation.

Likely initial ADRs:

```text
ADR-001 Go as kernel/runtime language
ADR-002 runtime daemon separated from TUI
ADR-003 SQLite + content-addressed object store for local persistence
ADR-004 event-sourced process transitions + projections
ADR-005 canonical event granularity excludes token/byte streams
ADR-006 context window treated as bounded cache
ADR-007 capability/effect enforcement outside LLM
ADR-008 provider conversation state is non-canonical
ADR-009 Worlds mediate external execution
ADR-010 no unbounded queues in runtime hot paths
```

An ADR should contain:

- context;
- decision;
- alternatives;
- consequences;
- status;
- revisit trigger.

---

# 5. Architecture review checklist

Before declaring A0 complete, review the system horizontally.

## Crash at every boundary

Ask:

```text
what if daemon dies here?
```

for every arrow in the architecture.

## Slow consumer at every stream

Ask:

```text
what if this consumer stops reading for 10 minutes?
```

## Huge input at every payload

Ask:

```text
what if this output is 10 GB?
```

## Recursive explosion

Ask:

```text
what if 100 children each spawn 100 children?
```

## Malicious model output

Ask:

```text
what if the model deliberately requests the most damaging valid-looking action?
```

## Long lifetime

Ask:

```text
what changes after 1 hour, 1 day, 30 days?
```

If runtime cost scales with history where it should scale with active state, redesign it.

---

# 6. A0 completion criteria

A0 is complete when:

1. every high-priority primitive has a written contract;
2. core state machines are documented;
3. canonical vs ephemeral data is known;
4. persistence/event boundaries are specified;
5. concurrency ownership and backpressure are specified;
6. failure/recovery tables exist for critical operations;
7. security/effect rules are deterministic at kernel boundaries;
8. TUI remains a thin attachable client by design;
9. long-session performance requirements have concrete tests;
10. major unresolved research questions are explicitly isolated from G0/G1;
11. initial ADRs are accepted;
12. G0 implementation tasks can be derived without inventing new semantics during coding.

---

# 7. What A0 does not mean

A0 does not mean predicting every implementation detail perfectly.

We should still prototype uncertain low-level choices.

The distinction is:

```text
prototype to validate a documented question
```

rather than:

```text
write production code and discover what the architecture means afterward
```

---

# Immediate direction

The current repository is now in **A0**, not G0.

Next architecture work should deepen the highest-coupling primitives first:

1. exact Agent Process + syscall state machines;
2. exact event/catalog/persistence model;
3. World/action/effect protocol;
4. authority/intent policy model;
5. runtime↔TUI IPC/projection protocol;
6. Cognitive MMU page/working-set algorithms;
7. transaction/fork semantics.

Only after these contracts stabilize should implementation start.
