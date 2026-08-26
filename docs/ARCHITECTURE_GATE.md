# Architecture Gate — A0 Architecture & Semantics

## Status

**PASS — A0 semantic architecture is complete.**

See [`A0_EXIT_REVIEW.md`](A0_EXIT_REVIEW.md) for the final exit review, remaining empirical validation items and explicit deferred research.

No runtime code was required to complete A0.

---

# 1. Purpose of A0

The project deliberately inserted an architecture phase before G0 because it combines difficult systems concerns:

- durable state machines;
- long-running concurrency;
- probabilistic/untrusted model output;
- external side effects;
- context/memory virtualization;
- recursive process graphs;
- local persistence;
- interactive streaming UI;
- sandbox/security boundaries;
- speculative execution;
- self-improvement.

The goal was to decide **what each primitive means** before implementation accidentally froze semantics.

---

# 2. Definition of architected

A concept is considered architected when its contract specifies:

```text
problem / why
semantic primitive
canonical vs ephemeral state
state machine / lifecycle
invariants
concurrency ownership
backpressure
failure / recovery
security / authority / effects
performance/resource bounds
observability
validation tests/benchmarks
evolution/deferred research
```

A0 does not require every tuning constant to be known before measurement.

It requires empirical choices to be isolated behind already-defined semantics.

---

# 3. A0 workstream results

## A0.1 Runtime/process semantics — CLOSED

Defined:

- durable Agent Process identity;
- lifecycle/state machine;
- parent/child lineage;
- cancellation;
- waiting/sleeping;
- model-independent identity;
- activation vs durable state distinction.

Primary docs:

- `AGENT_PROCESS_STATE_MACHINE.md`
- `ARCHITECTURE.md`
- `CONCURRENCY_AND_BACKPRESSURE.md`

---

## A0.2 Persistence/event semantics — CLOSED

Defined:

- append-only meaningful Event Ledger;
- global/local sequence/version semantics;
- optimistic process versions;
- deterministic reducers;
- snapshots + tail replay;
- projections;
- object finalization/GC;
- versioned payloads.

Primary docs:

- `EVENT_MODEL_AND_CATALOG.md`
- `STATE_PERSISTENCE_AND_STORAGE.md`
- `FOUNDATION_TECHNICAL_DECISIONS.md`

---

## A0.3 Context virtualization — CLOSED

Defined:

- Context Pages;
- bounded working-set algorithm;
- tiered packing;
- explicit recall;
- Context Fault classes;
- semantic references;
- context leases;
- structured compaction;
- fault-storm prevention;
- observable Context Manifests.

Primary docs:

- `COGNITIVE_MMU_V0_ALGORITHM.md`
- `CONTEXT_FAULTS_AND_COGNITIVE_PAGING.md`

Empirical tuning remains for ranking/page-size thresholds only.

---

## A0.4 Epistemic Memory — CLOSED semantics

Defined:

- immutable/versioned evidence;
- platform-maintained provenance;
- provenance non-amplification;
- belief lifecycle;
- typed evidence/dependency edges;
- contradiction preservation;
- temporal/scope validity;
- localized Truth Maintenance propagation;
- episodic-first consolidation;
- branch-local memory overlays.

Primary doc:

- `EPISTEMIC_MEMORY_AND_TRUTH_MAINTENANCE.md`

Confidence/consolidation heuristics remain empirical.

---

## A0.5 Authority, Intent and Effects — CLOSED

Defined:

- typed capabilities;
- subset/delegation;
- leases/revocation;
- Effect System;
- immutable/versioned Intent Envelope;
- delegated Task Intent;
- Purpose-Carrying Actions;
- Action Proof;
- deterministic + risk-sensitive semantic gates;
- scoped approvals;
- safe-World redirection;
- prompt-injection boundary.

Primary docs:

- `CAPABILITY_AND_INTENT_MODEL.md`
- `INTENT_BASED_AUTHORITY_ENGINE.md`
- `SECURITY_AND_EFFECTS.md`
- `WORLD_ACTION_AND_EFFECT_PROTOCOL.md`

---

## A0.6 Execution Worlds — CLOSED semantics

Defined:

- World Profile/guarantees;
- Local vs Workspace vs OCI vs Python semantics;
- optional snapshot/fork/promotion capabilities;
- streaming I/O;
- process-tree ownership;
- filesystem/symlink policy;
- network enforcement compatibility;
- secret binding;
- World loss/reconciliation.

Primary doc:

- `EXECUTION_WORLDS_PLATFORM_CONTRACT.md`

Exact Git/OCI/platform adapter mechanics are empirical prototypes, not semantic blockers.

---

## A0.7 Transactions / Cognitive Forks — CLOSED conservative v0

Defined:

- transaction state machine;
- reversible/irreversible barriers;
- verification;
- checkpoint classes;
- Execution Frontier;
- quiescent mutation-capable fork baseline;
- restore-as-new-timeline;
- three-way World merge;
- selective cognitive merge;
- promotion lease;
- reconciliation after uncertain commit.

Primary docs:

- `TRANSACTIONS_AND_COGNITIVE_FORKS.md`
- `EXECUTION_EDIT_SAFETY.md`

Advanced non-quiescent execution editing is explicitly deferred.

---

## A0.8 Recursive orchestration — CLOSED

Defined:

- durable spawn protocol;
- resource reservation;
- result contracts;
- durable parent waits;
- cancellation tree;
- bounded mailboxes;
- adaptive teams;
- finite negotiation;
- wait-for cycle detection;
- fan-out/fairness limits.

Primary doc:

- `RECURSIVE_ORCHESTRATION_PROTOCOL.md`

---

## A0.9 Cognitive Scheduler / Economy — CLOSED semantics

Defined:

- per-Cognitive-Task routing;
- model profiles;
- hard eligibility filtering;
- cost/latency/quality objective;
- runtime load/health telemetry;
- budget reservation/settlement;
- fallback/circuit breaking;
- hedging/speculation admission;
- local inference as bounded resource;
- fairness;
- learned-routing lifecycle.

Primary doc:

- `COGNITIVE_SCHEDULER_ARCHITECTURE.md`

Weights/quality priors remain empirical.

---

## A0.10 TUI/local control — CLOSED semantics

Defined:

- durable runtime separated from TUI;
- local IPC envelope;
- attach/detach;
- cursor-based pagination;
- bounded live subscriptions;
- stream coalescing;
- slow-client disconnect/recovery;
- large artifact streaming;
- approval/control semantics.

Primary docs:

- `LOCAL_CONTROL_PROTOCOL.md`
- `TUI_AND_STREAMING.md`

TUI framework choice remains benchmark-selected.

---

## A0.11 Reliability/performance — CLOSED architecture

Defined:

- bounded hot memory;
- bounded queues;
- streaming outputs;
- backpressure;
- supervised goroutine ownership;
- resource budgets;
- crash/fault matrix;
- 1h/8h/24h soak tests;
- 100k-history fixture;
- multi-GB output tests.

Primary docs:

- `RELIABILITY_AND_PERFORMANCE.md`
- `CONCURRENCY_AND_BACKPRESSURE.md`
- `FAILURE_MODEL_AND_RECOVERY.md`
- `TESTING_BENCHMARKS_AND_QUALITY_GATES.md`

Numeric thresholds are calibrated during implementation but cannot change the bounded-resource invariants.

---

## A0.12 Verified Continual Improvement — CLOSED extension semantics / implementation deferred

Defined:

- immutable artifact versions;
- hypothesis/baseline;
- evaluation;
- shadow/canary;
- hard security non-regression;
- scoped promotion;
- rollback;
- exact artifact manifests for reproducibility.

Primary doc:

- `VERIFIED_CONTINUAL_IMPROVEMENT.md`

Implementation remains intentionally late-stage.

---

# 4. Architecture review laws

Every future change must still survive these horizontal questions.

## Crash at every boundary

```text
what if daemon dies here?
```

## Slow consumer at every stream

```text
what if consumer stops reading for 10 minutes?
```

## Huge payload

```text
what if output is 10 GB?
```

## Recursive explosion

```text
what if 100 children each request 100 children?
```

## Malicious model output

```text
what if model deliberately requests the most damaging valid-looking action?
```

## Long lifetime

```text
what changes after 1 hour, 1 day, 30 days?
```

If cost scales with lifetime history where it should scale with active state, redesign it.

---

# 5. A0 exit criteria result

| Criterion | Result |
|---|---|
| high-priority primitives have written contracts | PASS |
| core state machines documented | PASS |
| canonical vs ephemeral state known | PASS |
| persistence/event boundaries specified | PASS |
| concurrency/backpressure specified | PASS |
| critical failure/recovery semantics exist | PASS |
| deterministic security/effect boundaries exist | PASS |
| TUI is thin/reconnectable | PASS |
| long-session tests are specified | PASS |
| deferred research isolated from G0/G1 | PASS |
| foundation decisions recorded | PASS |
| G0 tasks derivable without new semantics | PASS |

**A0 status: PASS.**

---

# 6. What remains empirical

These are adapter/policy validations, not architecture blockers:

```text
modernc SQLite benchmark/soak vs alternate driver
Git worktree dirty-state implementation details
Unix/Windows process-tree cancellation edge cases
OCI runtime feature adapter
MMU ranking/page-size thresholds
Epistemic confidence/consolidation heuristics
Scheduler utility/latency weights
TUI library choice
```

If an empirical result proves a semantic invariant impossible, A0 must be amended explicitly through an ADR/contract change.

---

# 7. Next phase

The next phase is:

```text
G0 — Foundations
```

But G0 begins only when explicitly requested.

G0's job is now to **implement** the existing contracts, not invent them.
