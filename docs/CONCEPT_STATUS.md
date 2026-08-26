# Concept Status Registry

## Status authority

This document is the **canonical current status registry** for the concepts introduced during A0.

`CONCEPT_CONTRACTS.md` remains useful as the original detailed concept checklist, but some of its historical `DESIGNING` / `RESEARCH` labels predate the deeper A0 contracts and are therefore superseded by this file plus `A0_EXIT_REVIEW.md`.

Status meanings:

```text
CLOSED      semantics/invariants are defined; implementation may proceed
EMPIRICAL   semantics are closed; tuning/adapter mechanics require measurement
DEFERRED    intentionally excluded from early implementation; extension boundary exists
```

---

| Concept | Current status | Authoritative contract(s) |
|---|---|---|
| Agent Process | CLOSED | `AGENT_PROCESS_STATE_MACHINE.md`, `ARCHITECTURE.md` |
| Agent Syscalls | CLOSED | `ARCHITECTURE.md`, `WORLD_ACTION_AND_EFFECT_PROTOCOL.md` |
| Event Ledger | CLOSED | `EVENT_MODEL_AND_CATALOG.md`, `STATE_PERSISTENCE_AND_STORAGE.md` |
| Durable Sleep/Wake | CLOSED | `AGENT_PROCESS_STATE_MACHINE.md`, `CONCURRENCY_AND_BACKPRESSURE.md` |
| Cognitive MMU | CLOSED / EMPIRICAL tuning | `COGNITIVE_MMU_V0_ALGORITHM.md` |
| Context Page | CLOSED / EMPIRICAL sizing | `COGNITIVE_MMU_V0_ALGORITHM.md`, `CONTEXT_FAULTS_AND_COGNITIVE_PAGING.md` |
| Context Fault | CLOSED | `CONTEXT_FAULTS_AND_COGNITIVE_PAGING.md` |
| Epistemic Memory | CLOSED / EMPIRICAL heuristics | `EPISTEMIC_MEMORY_AND_TRUTH_MAINTENANCE.md` |
| Truth Maintenance / Causal Invalidation | CLOSED / EMPIRICAL heuristics | `EPISTEMIC_MEMORY_AND_TRUTH_MAINTENANCE.md` |
| Execution World | CLOSED / EMPIRICAL adapters | `EXECUTION_WORLDS_PLATFORM_CONTRACT.md` |
| World guarantee/capability profile | CLOSED | `EXECUTION_WORLDS_PLATFORM_CONTRACT.md` |
| Effect System | CLOSED | `SECURITY_AND_EFFECTS.md`, `WORLD_ACTION_AND_EFFECT_PROTOCOL.md` |
| Capability Authority Tree / Leases | CLOSED | `CAPABILITY_AND_INTENT_MODEL.md` |
| Intent Lock / Intent-Based Authority | CLOSED | `INTENT_BASED_AUTHORITY_ENGINE.md` |
| Agent Transaction | CLOSED | `TRANSACTIONS_AND_COGNITIVE_FORKS.md`, `EXECUTION_EDIT_SAFETY.md` |
| Cognitive Fork | CLOSED conservative v0 | `TRANSACTIONS_AND_COGNITIVE_FORKS.md`, `EXECUTION_EDIT_SAFETY.md` |
| Checkpoint / Restore / Merge | CLOSED conservative v0 | `EXECUTION_EDIT_SAFETY.md` |
| Agent Economy | CLOSED | `ORCHESTRATION.md`, `COGNITIVE_SCHEDULER_ARCHITECTURE.md` |
| Recursive Agent Process | CLOSED | `RECURSIVE_ORCHESTRATION_PROTOCOL.md` |
| Adaptive Team Formation | CLOSED | `RECURSIVE_ORCHESTRATION_PROTOCOL.md` |
| Agent Negotiation | CLOSED | `RECURSIVE_ORCHESTRATION_PROTOCOL.md` |
| Cognitive Scheduler | CLOSED / EMPIRICAL routing policy | `COGNITIVE_SCHEDULER_ARCHITECTURE.md` |
| Model-independent Agent Identity | CLOSED | `ARCHITECTURE.md`, `COGNITIVE_SCHEDULER_ARCHITECTURE.md` |
| Replayable Cognition / Causal Trace | CLOSED extension semantics | `EVENT_MODEL_AND_CATALOG.md`, `EXECUTION_EDIT_SAFETY.md` |
| Verified Continual Improvement | CLOSED extension semantics / DEFERRED implementation | `VERIFIED_CONTINUAL_IMPROVEMENT.md` |
| Learned MMU ranking | DEFERRED | `COGNITIVE_MMU_V0_ALGORITHM.md`, `VERIFIED_CONTINUAL_IMPROVEMENT.md` |
| Learned model routing | DEFERRED | `COGNITIVE_SCHEDULER_ARCHITECTURE.md`, `VERIFIED_CONTINUAL_IMPROVEMENT.md` |
| Provider-specific mid-token Context Fault resume | DEFERRED | `CONTEXT_FAULTS_AND_COGNITIVE_PAGING.md` |
| Non-quiescent execution editing | DEFERRED research | `EXECUTION_EDIT_SAFETY.md` |
| Distributed workers/control plane | DEFERRED | roadmap G13 |
| Final TUI framework | EMPIRICAL / DEFERRED selection | `TUI_AND_STREAMING.md`, `FOUNDATION_TECHNICAL_DECISIONS.md` |

---

# A0 interpretation rule

If an older document contains a status label that conflicts with this registry:

```text
A0_EXIT_REVIEW.md
        +
CONCEPT_STATUS.md
        +
latest subsystem contract
```

are authoritative.

The older text remains design history and rationale, not the current gate status.

---

# Current project phase

```text
A0 — PASS / COMPLETE
G0 — READY, NOT STARTED
```

No implementation work should reinterpret a CLOSED concept silently. A high-coupling semantic change requires an explicit contract/decision update before code changes.
