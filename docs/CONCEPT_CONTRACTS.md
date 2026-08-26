# Core Concept Architecture Contracts

## Purpose

This document is the master checklist for the concepts currently proposed for the runtime.

It does **not** replace deeper subsystem documents. It ensures every idea has, at minimum:

- a concrete problem;
- architectural placement;
- proposed semantics;
- canonical state boundary;
- invariants;
- technical needs;
- failure/performance concerns;
- validation criteria;
- dependencies.

Status labels:

```text
DEFINED       semantics sufficiently clear for dependent architecture work
DESIGNING     direction is clear but important contracts remain open
RESEARCH      concept needs experiments/prior-art/evaluation before freezing
LATE          intentionally postponed; only extension boundaries matter now
```

---

# C01 — Agent Process

**Status:** DESIGNING

### Problem

Chat sessions/provider threads are not durable runtime identities and are poor ownership boundaries for long-running autonomous work.

### Role

Primary logical unit of durable intelligent execution.

### Semantics

Owns stable ID, root lineage, lifecycle, intent reference, authority, budgets, World bindings, model policy and memory scope. It is reconstructed from durable state and may be active without being tied to one provider or OS process.

### Canonical state

Persist: identity, lifecycle/version, lineage, root intent, capabilities/leases, budgets/reservations, pending work, checkpoints and references. Do not persist goroutine/socket handles.

### Invariants

- stable identity across daemon restart;
- terminal/model identity is not Agent identity;
- lifecycle transitions validated;
- one parent lineage after creation;
- terminal states do not become runnable without explicit new/fork semantics.

### Technical needs

Typed IDs/state machine, event reducer, optimistic versioning, supervisor activation, snapshot support.

### Failure/performance

Sleeping/waiting processes must not require dedicated goroutines. Startup must use projections/snapshots rather than replay every lifetime event.

### Validation

Kill daemon, restore same process state/version/lineage, continue work.

### Depends on

Event Ledger, storage, supervisor.

---

# C02 — Agent Syscalls

**Status:** DESIGNING

### Problem

Direct coupling to bash/MCP/browser/provider-specific tools prevents uniform policy, replay, effects and observability.

### Role

Small semantic ABI between harness/agent decisions and kernel services.

### Candidate operations

```text
observe recall execute spawn message signal checkpoint fork verify commit rollback sleep
```

### Canonical state

Syscall request/authorization/outcome metadata when it changes or depends on canonical state. High-volume payloads referenced as objects.

### Invariants

- state-changing syscalls pass kernel policy boundary;
- syscall semantics independent of provider/tool implementation;
- operation IDs stable for retries/reconciliation;
- unknown outcomes representable.

### Technical needs

Versioned request/response envelopes, typed action descriptors, effect and capability hooks, async work-unit IDs.

### Failure/performance

Syscall routing overhead should stay tiny; large results must stream/reference objects.

### Validation

Same logical syscall executed through two World adapters has consistent policy/audit semantics.

### Depends on

Agent Process, Authority, Effects, Worlds, Ledger.

---

# C03 — Event Ledger

**Status:** DEFINED direction / DESIGNING catalog

### Problem

Long-running state cannot be reliably understood/recovered from mutable in-memory objects and chat transcripts.

### Role

Append-only history of meaningful canonical transitions.

### Canonical state

The ledger itself plus state projections/snapshots. Token/byte streams excluded from canonical event granularity.

### Invariants

- committed event immutable;
- ordering/version semantics explicit;
- causation/correlation preserved;
- reducer deterministic;
- event schema versioned.

### Technical needs

SQLite WAL, transactional append, process sequence/version, event catalog, upcasters, snapshots.

### Failure/performance

Never persist one event per token. Snapshot so recovery cost is bounded.

### Validation

Replay snapshot + tail equals stored current projection; randomized crash/replay tests.

### Depends on

Storage/object model.

---

# C04 — Durable Sleep / Wake

**Status:** DESIGNING

### Problem

Waiting agents should not consume resident compute for minutes/days.

### Role

Persist wake condition/time as process state and release execution resources.

### Canonical state

Wake type, deadline/time, condition reference, process state, schedule version.

### Invariants

- sleeping does not require dedicated worker;
- wake is idempotent;
- cancelled process cannot be revived by stale wake;
- schedule survives restart.

### Technical needs

Central timer scheduler, durable wake table/heap rebuild, event-driven future conditions.

### Failure/performance

Avoid ticker/goroutine per sleeping process.

### Validation

Create thousands of sleepers, restart daemon, correct agents wake with stable idle memory/CPU.

### Depends on

Agent Process, supervisor, persistence.

---

# C05 — Cognitive MMU

**Status:** DESIGNING / RESEARCH algorithms

### Problem

Conversation history grows without bound while LLM context remains finite.

### Role

Build a bounded task-relevant working set for every invocation.

### Canonical state

Context-page metadata/references, pinning, dependencies, persisted retrieval artifacts; active packed prompt is invocation data rather than long-term truth.

### Invariants

- hard token budget never exceeded;
- root intent/safety constraints retained;
- eviction never equals deletion;
- context trace explains selected pages;
- ranking does not rely only on embeddings.

### Technical needs

Token estimator, page store, ranking/packing algorithm, scopes, explicit recall, dependency graph, metrics.

### Failure/performance

Metadata search must not scan/load all page bodies. Caches bounded. Fault loops bounded.

### Validation

Task history > model window continues correctly with bounded active context and retrieved old facts.

### Depends on

Object store, memory, model invocation abstraction.

---

# C06 — Context Page

**Status:** DESIGNING

### Problem

Context needs an independently addressable/evictable unit smaller than “whole conversation”.

### Role

Semantic virtual-memory page for source, decisions, summaries, tool results, child reports, etc.

### Canonical state

ID, type, scope, source/evidence, object ref, token estimate, timestamps, importance/confidence, dependency references, pin policy.

### Invariants

- stable ID/reference;
- content integrity if immutable;
- source/provenance retained where relevant;
- body lazily loadable.

### Technical needs

Metadata schema, object storage, tokenization cache, page versioning.

### Failure/performance

Large page bodies remain outside relational hot path; oversized pages may need segmentation.

### Validation

Load/rank metadata for large corpus without materializing all bodies.

### Depends on

Cognitive MMU, object store.

---

# C07 — Context Fault

**Status:** RESEARCH

### Problem

An agent can need durable knowledge not currently materialized in active context.

### Role

Runtime-visible missing-knowledge/retrieval operation.

### v0 semantics

Explicit `recall(query/ref)` between model invocations.

### Future semantics

Symbolic references and automated page-in when safe/observable.

### Canonical state

Fault/query, selected page IDs, ranking reason, invocation relation; page bodies already stored separately.

### Invariants

- bounded number of fault/repair loops;
- no hidden authority change;
- retrieval observable/replayable;
- failure to resolve represented explicitly.

### Technical needs

Reference grammar, recall resolver, loop budget, invocation restart semantics.

### Failure/performance

Avoid thrashing where pages constantly page-in/page-out. Track hit rate/fault rate.

### Validation

Synthetic task deliberately dereferences evicted old knowledge; correct page restored without context overflow.

### Depends on

Cognitive MMU, Context Pages.

---

# C08 — Epistemic Memory

**Status:** RESEARCH / DESIGNING

### Problem

Flat snippets do not represent why knowledge is trusted, whether it is stale or what evidence supports it.

### Role

Evidence-aware durable belief system.

### Canonical state

Statement/structured fact, scope, status, provenance, confidence metadata, freshness, dependencies, contradictions, verification timestamps.

### Invariants

- conflicting new evidence does not silently erase old belief/history;
- scope enforced;
- evidence references retained for important beliefs;
- invalidated/stale beliefs cannot appear fully trusted.

### Technical needs

Belief schema, graph edges, evidence refs, retrieval index, verifier policies.

### Failure/performance

Graph algorithms must avoid unbounded synchronous cascades; invalidation can use bounded queues/background work with durable status.

### Validation

Repository fact changes; derived knowledge gets downgraded/reviewed and old evidence remains inspectable.

### Depends on

Context MMU, object/event evidence, scopes.

---

# C09 — Truth Maintenance / Causal Invalidation

**Status:** RESEARCH

### Problem

Long-lived agents accumulate decisions based on facts that later change.

### Role

Propagate staleness/invalidity through knowledge dependencies.

### Semantics

Evidence version/change invalidates directly derived beliefs, which mark dependents `needs_review` according to edge semantics rather than immediately deleting them.

### Canonical state

Dependency edges, invalidation reason/source event, status transitions, verification outcome.

### Invariants

- invalidation is monotonic until explicit re-verification/supersession;
- cycles handled safely;
- old causal history preserved.

### Technical needs

Directed graph traversal, cycle detection/visited sets, edge types, bounded work queue.

### Failure/performance

Huge graphs require incremental propagation. One file change must not freeze interactive runtime.

### Validation

Large synthetic dependency graph with cycles/branches; deterministic affected-set and bounded processing.

### Depends on

Epistemic Memory, evidence versioning.

---

# C10 — Execution World

**Status:** DESIGNING

### Problem

Agents should not be coupled to unrestricted local shell/filesystem semantics.

### Role

Explicit execution boundary describing environment abilities and isolation/recovery properties.

### Canonical state

World identity/type/config references, lifecycle, capability level, snapshot/checkpoint refs when durable.

### Invariants

- action goes through kernel before World;
- World advertises capabilities honestly;
- local host access never implied by abstract World;
- resource/network/filesystem policies enforced by adapter/environment where possible.

### Technical needs

Minimal World/action/result interfaces, lifecycle manager, local adapter, later OCI/Python/SSH.

### Failure/performance

Large I/O streamed. Process descendants cancelled. World loss reconciled.

### Validation

Same forbidden action denied before reaching both Local and OCI World; kill/recovery tests by World level.

### Depends on

Syscalls, Authority, Effects.

---

# C11 — World Capability Levels

**Status:** DESIGNING

### Problem

Not all execution environments support snapshot/fork/recovery equally.

### Role

Runtime-discoverable feature set rather than pretending every World is transactional.

### Candidate abilities

```text
Execute
ReadFS
WriteFS
NetworkPolicy
ResourceLimits
Snapshot
Fork
Rollback
DurableIdentity
ReconcileAction
```

### Invariants

Kernel never schedules an operation requiring a World ability that is absent.

### Technical needs

Capability descriptor/version negotiation.

### Validation

Transaction/fork request rejected or degraded deterministically on unsupported World.

### Depends on

World abstraction.

---

# C12 — Effect System

**Status:** DESIGNING

### Problem

Binary “safe/dangerous” tool permissions do not capture retry/rollback/speculation semantics.

### Role

Typed metadata about observable consequences.

### Core classes

```text
Pure
Read
Reversible
Compensatable
Irreversible
```

Additional traits:

```text
idempotent retryable commutative external-visible requires-secret cost-bearing
```

### Canonical state

Effect descriptor attached before execution; actual observed effect/outcome metadata after execution.

### Invariants

- action cannot self-declare weaker effect than kernel/tool contract allows;
- irreversible unknown outcomes are never blindly retried;
- speculative policies use effect metadata.

### Technical needs

Descriptor schema, static tool declaration + dynamic refinement, policy engine.

### Validation

Matrix tests of retry/speculate/approval/rollback decisions by effect traits.

### Depends on

Actions/World, Authority.

---

# C13 — Capability Authority Tree / Leases

**Status:** DESIGNING

### Problem

Subagents often inherit excessive credentials/permissions.

### Role

Strict delegable authority with scope and expiry.

### Canonical state

Grant ID, subject agent, capability expression, delegable flag, issuer, parent grant, validity/lease conditions, revocation.

### Invariants

- child grant subset of active delegable parent authority;
- no authority created from prompt/tool content;
- expiry/revocation survives restart;
- secret use does not imply secret disclosure to model.

### Technical needs

Capability grammar, subset/intersection algorithm, policy evaluator, secret broker.

### Failure/performance

Policy checks must be fast/cacheable but correctness cannot rely on stale cache.

### Validation

Property tests generate random grant trees and prove no child exceeds ancestor authority.

### Depends on

Agent Process, Intent, Actions.

---

# C14 — Intent Lock / Intent-Based Authority

**Status:** RESEARCH / DESIGNING

### Problem

A technically permitted action can still be unrelated to what the user asked; long-running agents may drift or obey injected instructions.

### Role

Immutable root security/product object representing goal, constraints and effect domains.

### Canonical state

Root goal, constraints, acceptance criteria, allowed/forbidden domains, origin/user authority, amendments explicitly approved by user.

### Invariants

- agent cannot silently rewrite root intent;
- child intent narrows/refines parent purpose;
- capability is necessary but not sufficient for sensitive action;
- semantic model judgement alone is never sole security boundary.

### Technical needs

Intent schema, deterministic domain rules, action-to-intent metadata, optional semantic review for ambiguity.

### Failure/performance

False-positive denial must be explainable/appealable. Keep evaluation bounded/cached where deterministic.

### Validation

Prompt-injection corpus where broad network capability exists but unrelated exfiltration is denied.

### Depends on

Authority, Effects, Agent Process.

---

# C15 — Agent Transaction

**Status:** RESEARCH / DESIGNING

### Problem

Agents often discover an approach is wrong after many partial mutations.

### Role

Isolated staged work with verification and promotion/rollback semantics.

### Canonical state

Transaction ID/state, World snapshot/base, staged effects, verification results, commit/rollback/reconciliation state.

### Invariants

- reversible changes isolated until promotion where World supports it;
- irreversible effects never falsely modeled as rollbackable;
- commit state explicit and recoverable;
- verification evidence recorded.

### Technical needs

Transaction state machine, World snapshots/overlay, commit protocol, effect barriers, reconciliation.

### Failure/performance

Crash during commit is critical; minimize multi-step promotion and represent `NEEDS_RECONCILIATION`.

### Validation

Fault injection at every commit/rollback boundary; observable state exactly restored after failed reversible workflow.

### Depends on

World, Effect System, Ledger.

---

# C16 — Cognitive Fork

**Status:** RESEARCH

### Problem

Reasoning branches are weak if alternatives mutate the same real environment.

### Role

Fork recoverable cognitive + environmental state into isolated alternative branches.

### Canonical state

Fork lineage/base checkpoint, branch Agent IDs, memory/context overlay refs, World fork refs, budget reservations, evaluator/promotion result.

### Invariants

- sibling World mutation isolation;
- branch authority cannot exceed base;
- branch budgets reserved/bounded;
- promotion rationale/evidence durable.

### Technical needs

Checkpoint format, copy-on-write metadata, World fork, overlay memory, evaluator contract, cleanup.

### Failure/performance

Forking multiplies compute/storage; strict fan-out and retention limits required. Prefer COW over copying large state.

### Validation

Two implementations built/benchmarked in parallel; only winning branch promoted, loser cannot leak mutations.

### Depends on

Transactions, checkpoints, Worlds, Agent Process.

---

# C17 — Agent Economy

**Status:** DESIGNING

### Problem

Recursive agents can explode in token cost, wall time, storage and concurrent operations.

### Role

Hierarchical accounting and reservation of finite resources.

### Resources

```text
money
tokens
wall deadline
model slots
tool slots
children
forks
storage/artifacts
context tokens
```

### Canonical state

Budget accounts, reservations, settlement/usage, issuer/parent relation.

### Invariants

- child cannot reserve more than available parent/global amount;
- reservations prevent oversubscription;
- cancelled/unused reservation released exactly once;
- restart does not reset usage.

### Technical needs

Atomic ledger/accounting transaction, units/money decimal semantics, scheduler integration.

### Failure/performance

High-frequency token updates can be aggregated; final accounting canonical. Avoid lock contention with global budget.

### Validation

Concurrent reservation property tests; no negative available budget or double release.

### Depends on

Agent Process, Scheduler, Ledger.

---

# C18 — Cognitive Scheduler

**Status:** DESIGNING / later RESEARCH

### Problem

One model per session wastes cost/latency and reduces resilience.

### Role

Route cognitive work to eligible models/providers based on task and policy.

### v0 semantics

Deterministic rules over capability, context size, privacy/locality, provider health, cost ceiling and task class.

### later semantics

Learn policy from verified outcomes.

### Canonical state

Routing request, eligible models, selected model, policy version, estimates, fallback events, observed metrics/outcome.

### Invariants

- privacy/locality is a hard filter;
- scheduler cannot exceed budget;
- fallback does not change Agent identity;
- selection explainable/replayable by policy version.

### Technical needs

Model registry, health/cost metadata, task class, rule engine, telemetry/evals.

### Failure/performance

Provider outage should trip health state rather than cause retry storms.

### Validation

Evaluation corpus comparing scheduler quality/cost/latency frontier to fixed-model baselines.

### Depends on

Provider abstraction, Agent Economy, verification metrics.

---

# C19 — Recursive Subagents

**Status:** DESIGNING

### Problem

Large tasks benefit from independent contexts and parallel specialization, but naive subagents lack durable/budget/security boundaries.

### Role

Child Agent Processes with bounded authority, budget and independent context.

### Canonical state

Child process, parent relation, delegated intent/task, grants, budget, model policy, result contract, messages.

### Invariants

- child is a real Agent Process;
- authority/budget subset rules;
- cancellation/wait semantics defined;
- structured evidence can be returned without copying whole child transcript.

### Technical needs

`spawn`, supervisor, result/evidence schema, messaging, reservations.

### Failure/performance

Fan-out bounded; parent wait does not retain child transcripts in RAM.

### Validation

Parallel child investigation survives restart and returns evidence references.

### Depends on

Agent Process, Authority, Economy, Scheduler.

---

# C20 — Adaptive Team Formation

**Status:** LATE / RESEARCH

### Problem

Static multi-agent graphs bake one organization into every problem.

### Role

Propose temporary specialist topology based on task complexity and resources.

### Canonical state

Team proposal, roles/tasks, approved budget, resulting child lineage, performance outcome.

### Invariants

- topology cannot bypass spawn authority/budgets;
- finite fan-out/depth;
- roles are harness hints, not security permissions.

### Technical needs

Task decomposition schema, topology policy, historical evals.

### Validation

Compare adaptive topology against fixed baseline on multi-domain tasks.

### Depends on

Subagents, Scheduler, Economy.

---

# C21 — Agent Negotiation Protocol

**Status:** LATE / RESEARCH

### Problem

One-shot reviewer/implementer handoffs do not exploit structured disagreement and evidence exchange.

### Role

Bounded peer protocol for claim/challenge/evidence/revision/agreement/escalation.

### Canonical state

Negotiation thread, claims/evidence refs, participants, budget/deadline, resolution/escalation.

### Invariants

- peers cannot gain authority through messaging;
- bounded rounds/tokens/time;
- evidence references preferred to transcript dumping;
- unresolved disputes escalate rather than loop forever.

### Technical needs

Message schema, protocol state machine, deadlock/round limit.

### Validation

Seeded race-condition disagreement scenario converges or escalates within fixed budget.

### Depends on

Subagent messaging, evidence model, Economy.

---

# C22 — Verified Continual Improvement

**Status:** LATE / RESEARCH

### Problem

Self-modifying prompts/skills can silently regress or drift.

### Role

Treat cognitive improvements as versioned, evaluated changes.

### Canonical state

Hypothesis, baseline/candidate artifact versions, eval suite/version, metrics, promotion/rejection, rollback lineage.

### Invariants

- no autonomous authority/security weakening;
- all promoted artifacts versioned/revertible;
- promotion requires declared evidence policy;
- original user intents/history not rewritten.

### Technical needs

Artifact registry, eval runner, comparison metrics, promotion gates.

### Failure/performance

Evaluation can be expensive; schedule offline/background within explicit budget.

### Validation

Candidate with task improvement but security regression is rejected.

### Depends on

Evals, artifact versioning, Scheduler.

---

# C23 — Cognitive Artifact Versioning

**Status:** LATE but extension boundary needed early

### Problem

Behavior cannot be reproduced if prompts/skills/routing policies mutate invisibly.

### Role

Immutable/versioned identity for cognitive configuration.

### Canonical state

Artifact type/name/version/content ref/hash/parents/status/eval refs.

### Invariants

- invocation records exact artifact versions used;
- promoted version never mutates in place;
- rollback selects prior version.

### Technical needs

Artifact store and manifests.

### Validation

Replay identifies exact harness/prompt/policy versions from historical invocation.

### Depends on

Object store, Ledger.

---

# C24 — Replayable Cognition / Time-Travel Debugging

**Status:** LATE architecture enabled early

### Problem

Raw transcripts are insufficient to explain/reproduce agent behavior.

### Role

Replay deterministic runtime envelope, substitute model outcomes, fork historical checkpoints.

### Canonical state

Already provided by events, object refs, policy/artifact versions, model metadata and checkpoints.

### Invariants

- exact replay does not re-execute external effects;
- simulation/re-execution mode explicit;
- substituted model output creates a new branch/history, not rewritten past.

### Technical needs

Deterministic reducers, artifact versioning, replay engine, fixture provider/World.

### Failure/performance

Large history uses snapshots/indexes; replay does not materialize every blob unless needed.

### Validation

Replay to event N, substitute model result, create branch and compare causal graph.

### Depends on

Ledger, snapshots, versioned artifacts, forks.

---

# C25 — Causal Agent Trace

**Status:** DESIGNING as cross-cutting metadata

### Problem

Users need to know why an autonomous action happened without reading thousands of chat lines.

### Role

Graph linking intent → plan/task → agent → evidence/belief → action → authorization → effect → result.

### Canonical state

Causation/correlation IDs and typed references embedded in existing records/events; not necessarily a separate duplicated trace database.

### Invariants

- security-sensitive actions trace to authorizing capability and root intent;
- child/fork lineage traceable;
- evidence used by durable beliefs traceable;
- missing links reported, not fabricated.

### Technical needs

Reference types, causal edge conventions, projection/query API, later visualization.

### Failure/performance

Graph query must be lazy/paginated for long sessions.

### Validation

Given a committed file mutation, query path back to user intent, model invocation, capability, effect and verification result.

### Depends on

All core canonical IDs/events.

---

# Cross-concept dependency order

The concepts form layers rather than 25 independent features.

```text
Layer 0 — durability
  Agent Process
  Event Ledger
  State/Storage

Layer 1 — controlled execution
  Syscalls
  Worlds
  Effects
  Authority
  Intent

Layer 2 — bounded cognition
  Context Pages
  Cognitive MMU
  Explicit Recall

Layer 3 — recursion/resources
  Subagents
  Agent Economy
  Cognitive Scheduler

Layer 4 — safe speculation
  Transactions
  Checkpoints
  Cognitive Fork

Layer 5 — long-term knowledge
  Epistemic Memory
  Truth Maintenance
  Context Faults

Layer 6 — advanced organization/learning
  Adaptive Teams
  Negotiation
  Cognitive Artifact Versioning
  Verified Continual Improvement
  Replay/Time Travel
```

The dependency order is intentional. Advanced cognition should not be implemented before the durable/security/resource substrate it relies on.

---

# Cross-cutting architecture laws

All concepts must obey these laws.

## Law 1 — Bounded hot state

Lifetime history may grow; hot memory/context/UI state must remain bounded.

## Law 2 — Durable before relied upon

If correctness after crash depends on a state transition, persist it before treating it as canonical.

## Law 3 — Authority cannot emerge from content

Model/tool/web/repository content may influence reasoning but cannot mint capabilities.

## Law 4 — Effects are explicit

A mutation cannot hide behind a generic tool call.

## Law 5 — Unknown is a real state

The runtime never invents certainty after partial external failure.

## Law 6 — Presentation is disposable

TUI state is never the source of Agent Process truth.

## Law 7 — Recursive work is budgeted

Every child/fork consumes explicitly bounded resources.

## Law 8 — Observability without transcript archaeology

Important decisions/actions carry typed causal references.

## Law 9 — Model is compute, not identity

Provider/model may change while Agent identity remains stable.

## Law 10 — Self-improvement cannot rewrite security

Cognitive evolution never grants authority or mutates immutable root policy.

---

# What remains open after this contract pass

This document gives every current concept an architecture envelope, but several items still require deeper design before coding their milestone:

- exact Agent Process state-transition tables;
- formal capability grammar/subset rules;
- exact action/effect descriptor types;
- World capability negotiation;
- event catalog and payload schemas;
- IPC messages and conversation projections;
- Cognitive MMU ranking/packing algorithm;
- transaction commit protocol;
- belief dependency edge semantics;
- scheduler scoring/routing policy;
- eval metrics for adaptive/self-improving features.

These become explicit A0 design tasks rather than surprises during implementation.
