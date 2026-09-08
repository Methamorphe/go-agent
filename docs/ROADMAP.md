# Roadmap

## Strategy

The project uses two phases:

```text
A0  Architecture & Semantics
 ↓
G0… Implementation generations
```

A generation is complete only when its invariants, failure behavior and non-functional claims are implemented, tested and documented. Detailed historical closure evidence lives in the corresponding `G*_EXIT_REVIEW.md`; this roadmap is the current status and forward plan.

## Implementation status

```text
A0  COMPLETE  Architecture & Semantics
G0  COMPLETE  Foundations
G1  COMPLETE  Durable Agent Process + Event Ledger
G2  COMPLETE  Minimal Agent Loop + Agent Syscalls
G3  COMPLETE  Worlds + Authority + Effect System
G4  COMPLETE  Cognitive MMU v0
G5  COMPLETE  Recursive Agent Processes
G6  COMPLETE  Cognitive Scheduler v0
G7  COMPLETE  Workspace/OCI World + Agent Transactions
G8  COMPLETE  Cognitive Fork / Safe Execution Editing
G9  COMPLETE  Epistemic Memory + Truth Maintenance
G10 COMPLETE  Context Faults + Cognitive MMU v2
G11 COMPLETE  Adaptive Teams + Agent Negotiation
G12 COMPLETE  Verified Continual Improvement
G13 COMPLETE  Production TUI / Interactive Agent Workspace
G14 READY     Distributed Worlds / Workers
G15 PLANNED   Production Observability / Time-Travel Debugger
```

See:

- `CURRENT_GENERATION.md`;
- `A0_EXIT_REVIEW.md`;
- `G0_EXIT_REVIEW.md`;
- `G1_EXIT_REVIEW.md`;
- `G2_EXIT_REVIEW.md`;
- `G3_EXIT_REVIEW.md`;
- `G4_EXIT_REVIEW.md`;
- `G5_EXIT_REVIEW.md`;
- `G6_EXIT_REVIEW.md`;
- `G7_EXIT_REVIEW.md`;
- `G8_EXIT_REVIEW.md`;
- `G9_EXIT_REVIEW.md`;
- `G10_EXIT_REVIEW.md`;
- `G11_EXIT_REVIEW.md`;
- `G12_EXIT_REVIEW.md`;
- `G13_EXIT_REVIEW.md`;
- `PRODUCTION_TUI_WORKSPACE.md`;
- `LONG_DURATION_BENCHMARKS.md`;
- `ARCHITECTURE_GATE.md`;
- `ARCHITECTURE_DECISIONS.md`;
- `FOUNDATION_TECHNICAL_DECISIONS.md`.

The working product name remains **TBD**.

---

# A0 — Architecture & Semantics ✅ COMPLETE

A0 fixed the execution semantics before implementation: durable Agent Processes, Event Ledger, Worlds, authority/effects, Cognitive MMU, Context Faults, epistemic memory, recursive agents, transactions/forks, scheduler/economy, verified continual improvement, bounded queues and recovery rules.

**Result: PASS.** See `A0_EXIT_REVIEW.md`.

---

# G0 — Foundations ✅ COMPLETE

G0 established the Go runtime substrate: typed configuration/IDs/errors, SQLite + migrations, content-addressed Object Store, local cross-platform control transport, structured diagnostics, bounded protocol framing, race baseline and cross-platform CI.

**Result: PASS.** See `G0_EXIT_REVIEW.md`.

---

# G1 — Durable Agent Process + Event Ledger ✅ COMPLETE

G1 implemented durable process identity/lifecycle/lineage, append-only causal events, deterministic replay/snapshots, idempotent commands, optimistic versioning and restart-safe supervision without one goroutine per waiting process.

**Result: PASS.** See `G1_EXIT_REVIEW.md`.

---

# G2 — Minimal Agent Loop + Agent Syscalls ✅ COMPLETE

G2 added provider-neutral streaming, deterministic fake/provider adapters, bounded model/tool output, `observe`/`execute`/`checkpoint`, workspace read/list/process actions and an attachable headless client.

**Result: PASS.** See `G2_EXIT_REVIEW.md`.

---

# G3 — Worlds + Authority + Effect System ✅ COMPLETE

G3 made execution structurally controlled through typed Worlds, scoped capabilities/leases/delegation, canonical Effect classification, versioned Intent, Purpose-Carrying Actions, Action Proofs and fail-closed authorization before World execution.

**Result: PASS.** See `G3_EXIT_REVIEW.md`.

---

# G4 — Cognitive MMU v0 ✅ COMPLETE

G4 replaced flat conversation-history memory with persisted semantic Context Pages, bounded working-set planning, explicit recall, Context Manifests, structured compaction, lazy materialization and bounded caches.

**Result: PASS.** See `G4_EXIT_REVIEW.md`.

---

# G5 — Recursive Agent Processes ✅ COMPLETE

G5 added bounded recursive delegation with durable spawn/wait, authority subsets, hierarchical budget reservation/settlement, bounded mailboxes, cancellation trees, wait-cycle rejection, fan-out/depth limits and structured result/evidence contracts.

**Result: PASS.** See `G5_EXIT_REVIEW.md`.

---

# G6 — Cognitive Scheduler v0 ✅ COMPLETE

G6 decoupled Agent identity from a single model using hard eligibility filtering, deterministic utility scoring, provider health/circuit breaking, bounded fallback, privacy/locality constraints, durable model budgets and fairness/global/root/provider slots.

**Result: PASS.** See `G6_EXIT_REVIEW.md` and `COGNITIVE_SCHEDULER_V0.md`.

---

# G7 — Workspace/OCI World + Agent Transactions ✅ COMPLETE

G7 introduced isolated Git `WorkspaceWorld`, conservative OCI execution and durable transactions:

```text
begin
execute
verify
prepare
commit / rollback / reconcile
```

Promotion is three-way, target divergence is detected, unknown dispatched outcomes preserve uncertainty and commit policy is revalidated instead of trusting the client.

**Result: PASS.** See `G7_EXIT_REVIEW.md`.

---

# G8 — Cognitive Fork / Safe Execution Editing ✅ COMPLETE

G8 added quiescent forkable checkpoints, explicit Execution Frontier, isolated Agent/World futures, branch-local cognitive overlays, independent budget reservations, deterministic objective evaluation, selective winner promotion and idempotent cleanup/retention.

Historical checkpoint authority cannot resurrect revoked rights and speculative irreversible effects fail closed.

**Result: PASS.** See `G8_EXIT_REVIEW.md`.

---

# G9 — Epistemic Memory + Truth Maintenance ✅ COMPLETE

G9 replaced flat memory snippets with immutable/versioned Evidence, Belief lifecycle/history, structured confidence, scope/temporal validity, contradiction/dependency edges and bounded localized truth-maintenance propagation.

Speculative branch knowledge remains isolated until explicit promotion and epistemic confidence never mints authority.

**Result: PASS.** See `G9_EXIT_REVIEW.md`.

---

# G10 — Context Faults + Cognitive MMU v2 ✅ COMPLETE

G10 made missing cognitive state an explicit provider-independent runtime paging event with stable semantic references and typed:

```text
ReferenceFault
RecallFault
EvidenceFault
FreshnessFault
DependencyFault
RepresentationFault
```

It added context leases, dependency-driven loading, fault budgets/storm protection and evidence-aware working-set planning.

**Result: PASS.** See `G10_EXIT_REVIEW.md`.

---

# G11 — Adaptive Teams + Agent Negotiation ✅ COMPLETE

G11 added durable temporary specialist teams plus bounded evidence-oriented disagreement. Team formation is scheduler/policy admitted and uses the existing G5 spawn/authority/budget boundaries. Negotiation has finite rounds/turns/deadlines and deterministic escalation.

**Result: PASS.** See `G11_EXIT_REVIEW.md` and `ADAPTIVE_TEAMS_AND_AGENT_NEGOTIATION.md`.

---

# G12 — Verified Continual Improvement ✅ COMPLETE

G12 added immutable versioned cognitive artifacts for skills, prompts, profiles, routing/context/memory policies and evaluators with the lifecycle:

```text
hypothesis
→ candidate
→ evaluate
→ shadow
→ canary
→ promote/reject
→ rollback
```

Hard invariants prevent capability expansion, root-Intent/effect-floor rewriting, scope widening and security/verification regression. Historical invocation attribution keeps exact artifact versions.

**Result: PASS.** See `G12_EXIT_REVIEW.md` and `VERIFIED_CONTINUAL_IMPROVEMENT.md`.

---

# G13 — Production TUI / Interactive Agent Workspace ✅ COMPLETE

## Goal

Turn the durable runtime into a fast, inspectable terminal product for pair-programming and supervising long-running multi-Agent work while keeping canonical state in the daemon.

## Delivered

### Production terminal client

- dedicated `go-agent-tui` fullscreen Bubble Tea v2 client;
- ASK / PLAN / ACT / REVIEW / OBSERVE modes;
- adaptive wide/medium/narrow layouts;
- keyboard-first workflow and command palette;
- optional mouse-wheel history navigation;
- dark/light themes;
- configurable keymaps, command aliases, refresh cadence, inspector cadence, FPS and cache limits;
- Unicode-safe text and resize behavior.

### Bounded workspace projection

- cursor-based attach/refresh;
- bounded process-tree and conversation viewport;
- on-demand older-history paging;
- explicit resynchronization when the client falls behind instead of unbounded presentation queues;
- bounded local block cache;
- large artifacts remain references;
- private model reasoning is never projected into visible blocks.

### Agent cockpit and review

- durable root/child Agent tree;
- focus switching without stopping siblings;
- steering versus queued follow-up semantics;
- suspend/resume through runtime APIs;
- plan-step review and targeted feedback;
- diff-file/hunk review and targeted feedback.

### G7/G8-native UX

Transactions are operable through the daemon/control protocol:

```text
verify
prepare
commit
rollback
reconcile
resolve uncertain effect
```

The daemon revalidates current process lifecycle/root Intent at PREPARE/COMMIT boundaries. `NEEDS_RECONCILIATION` is preserved honestly and never displayed as false success.

Fork inspection exposes candidate branches, spend, evaluation, selection state and winner rationale.

### Runtime inspectors

The workspace exposes bounded summaries for:

- Context/MMU and Context Faults;
- model/provider/scheduler routing and budgets;
- Intent/authority projection;
- G7 transactions;
- G8 forks;
- G11 teams;
- G12 verified improvements.

### Performance and platform gates

The Linux CI runs bounded G13 benchmarks in addition to tests/vet/builds. The final code closure run `34238313757` passed:

```text
test (ubuntu-latest)  ✅
test (macos-latest)   ✅
test (windows-latest) ✅
race                   ✅
```

The matrix builds:

```text
./cmd/go-agent
./cmd/go-agentctl
./cmd/go-agent-tui
```

Representative G13 benchmark results are approximately:

```text
100k-history bounded retention   ~0.08 ms/op
normal viewport redraw           ~2.75 ms/op
```

Both are well below the initial 50 ms local viewport engineering target.

## G13 result

**PASS.**

The TUI remains a replaceable client. Detach/crash does not cancel durable Agent work by itself, mutations retain headless/control-protocol parity, history/render memory is bounded, transaction uncertainty remains explicit, and authority cannot be minted by UI approval.

See `G13_EXIT_REVIEW.md` and `PRODUCTION_TUI_WORKSPACE.md`.

---

# G14 — Distributed Worlds / Workers 🟢 READY

## Goal

Run the same durable Agent Process abstraction across local and remote compute without binding Agent identity to worker location.

## Candidate deliverables

- worker capability/profile protocol;
- SSH worker adapter;
- remote Go worker protocol;
- Kubernetes jobs/workspaces;
- GPU/local-inference node routing;
- distributed object/blob storage boundary;
- worker leases and heartbeats;
- disconnection/unknown-outcome reconciliation;
- remote World lifecycle and cancellation semantics;
- locality-aware scheduler integration;
- authority-preserving remote action dispatch;
- cross-worker transaction/fork compatibility where guarantees permit.

## Required invariants

- Agent identity remains independent from worker location;
- remote execution cannot widen capabilities or Intent;
- worker loss cannot silently become successful action completion;
- external-effect uncertainty enters explicit reconciliation;
- bounded queues/backpressure remain mandatory across network boundaries;
- durable state remains reconstructable without a live worker process;
- object transfer is streamed/content-addressed rather than hot-memory buffered;
- scheduler routing preserves privacy/locality requirements.

## Initial killer demonstration

Run a durable Agent with work split between a local World and a remote worker, terminate the remote worker during an action, restart/reconcile it, and prove that Agent identity/history remains stable while unknown external outcomes are never reported as false success.

---

# G15 — Production Observability / Time-Travel Debugger

## Goal

Make complex Agent behavior understandable and replayable beyond the everyday G13 workspace.

## Planned deliverables

- advanced process/team tree observability;
- causal graph;
- deep context/fault inspector;
- belief/provenance graph;
- authority/Action Proof inspector;
- resource/cost timeline;
- World diff viewer;
- checkpoint/fork/restore/merge timeline;
- historical replay/fork;
- OpenTelemetry export.

G15 builds forensic/debugging workflows on top of the product interaction primitives established by G13 rather than moving canonical state into an observability UI.

---

# Cross-generation reliability requirements

Every generation inherits:

```text
bounded hot memory
bounded queues
streamed large I/O
explicit cancellation/deadlines
crash-safe canonical state
fault injection
race/leak tests
inspectable causality
no prompt-only security
```

A feature is not done if its happy path works but it leaks memory/goroutines, blocks indefinitely on slow clients, widens authority, hides unknown outcomes or has undefined crash behavior.

---

# Product evolution

A credible coding-agent experience emerged around G5 with a durable daemon, attachable terminal, persistent Agent Processes, streaming models, filesystem/shell actions, Event Ledger, authority/effects, bounded context and recursive subagents.

G6 added model/resource scheduling. G7 added isolated workspace/OCI execution and durable transactions. G8 added safe alternative futures and objective winner promotion. G9 added evidence-aware memory and localized truth maintenance. G10 added typed cognitive paging. G11 added bounded adaptive teams/negotiation. G12 added controlled verified self-improvement.

**G13 is the completed productization generation:** it exposes these primitives through a production interactive terminal workspace while preserving daemon durability and headless parity.

**G14 is now the immediate implementation target:** distribute Worlds/workers without weakening identity, authority, boundedness or reconciliation semantics. G15 then adds deep production observability/time-travel debugging.

---

# Testing philosophy

Most kernel semantics must remain testable without an LLM.

Use deterministic fake providers and fake Worlds for:

- process recovery;
- authority subset and denied effects;
- transaction rollback/reconciliation;
- fork isolation;
- budget accounting;
- context budgets/faults;
- unknown outcomes;
- slow consumers/backpressure;
- crash recovery;
- Truth Maintenance propagation;
- scheduler fairness;
- TUI projection/virtualization/reconnect behavior;
- distributed-worker failure/reconciliation in G14.

Real-model tests evaluate model/harness quality separately from kernel correctness.

Long-duration validation uses tagged provider-free soak scenarios and explicit boundedness criteria. See `LONG_DURATION_BENCHMARKS.md`.

---

# Immediate next step

**G14 — Distributed Worlds / Workers is READY.**

The next implementation pass should preserve all G0–G13 invariants while introducing remote worker identity/profile, transport, leases/heartbeats, streamed object movement, scheduler locality and explicit failure/reconciliation semantics.
