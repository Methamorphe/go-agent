# Roadmap

## Strategy

The project uses two phases:

```text
A0  Architecture & Semantics
 ↓
G0… Implementation generations
```

**A0 is complete. G0 is complete. G1 is complete. G2 is complete. G3 is complete. G4 is complete. G5 is complete. G6 is complete. G7 is complete. G8 is complete. G9 is ready.**

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
- `PRODUCTION_TUI_WORKSPACE.md`;
- `LONG_DURATION_BENCHMARKS.md`;
- `ARCHITECTURE_GATE.md`;
- `ARCHITECTURE_DECISIONS.md`;
- `FOUNDATION_TECHNICAL_DECISIONS.md`.

A generation is complete only when its invariants, failure behavior and non-functional claims are tested and documented.

The working product name remains **TBD**.

---

# A0 — Architecture & Semantics ✅ COMPLETE

## Goal

Design the execution model deeply enough that implementation validates decisions instead of inventing fundamental semantics ad hoc.

## Closed workstreams

A0 now defines:

- Agent Process state machine and Agent Syscalls;
- Event Ledger ordering, reducers, snapshots and projections;
- canonical vs ephemeral state;
- SQLite/Object Store persistence contracts;
- failure and recovery semantics;
- concurrency ownership, bounded queues and backpressure;
- runtime daemon ↔ TUI protocol and history virtualization;
- World/action/effect contracts;
- capability + Intent-Based Authority model;
- Cognitive MMU pages/working-set/recall semantics;
- Context Fault semantics;
- Epistemic Memory + Truth Maintenance;
- recursive-agent budget/cancellation/messaging semantics;
- transaction/fork/restore/merge safety;
- Cognitive Scheduler + Agent Economy;
- Verified Continual Improvement extension semantics;
- testing, soak and benchmark gates.

## A0 result

**PASS.**

Remaining unknowns are isolated as empirical adapter/tuning work rather than semantic gaps.

Examples:

```text
SQLite driver soak/performance validation
Git WorkspaceWorld implementation details
Unix/Windows process-tree edge cases
MMU ranking thresholds
Memory consolidation heuristics
Scheduler utility weights
TUI library selection
```

These do not block implementation because their interfaces/guarantees are already defined.

---

# G0 — Foundations ✅ COMPLETE

## Goal

Create a tiny Go codebase implementing the architecture's foundational boundaries without agent behavior.

## Delivered

- Go 1.27 module and repository layout;
- CLI/runtime daemon entry point and cancellation lifecycle;
- structured `log/slog` diagnostics;
- typed IDs/errors;
- injectable deterministic clock/ID generation;
- layered typed configuration;
- SQLite storage implementation behind internal adapter;
- content-addressed streaming Object Store;
- ordered embedded SQL migration mechanism with checksums;
- Unix-domain-socket local control on macOS/Linux;
- Windows named-pipe local control;
- bounded length-framed JSON control protocol;
- bounded control connection concurrency;
- runtime metrics snapshots and optional loopback-only pprof;
- basic unit/integration/fault test setup;
- SQLite hard-kill/reopen test foundation;
- malformed/oversized IPC frame tests;
- cross-platform GitHub Actions CI;
- race-detector baseline.

## Implemented baseline

```text
Go
modernc.org/sqlite behind internal adapter (provisional/benchmarked)
database/sql + explicit SQL
SQLite WAL + foreign_keys + reliability-first synchronous mode
SHA-256 content-addressed Object Store
Unix domain socket / Windows named pipe IPC
length-framed versioned JSON control protocol
```

## Preserved constraints

- no TUI-owned canonical state;
- no unbounded queues;
- large object APIs stream via `io.Reader`/`io.Writer`;
- storage health failures have explicit behavior;
- standard library preferred where adequate;
- no ORM in kernel storage;
- no new architecture semantics invented in code;
- no Agent Process/Event Ledger/model/World behavior leaked into G0.

## G0 result

**PASS.**

Foundation implementation commit:

```text
76a36df20ed8d0097725ddf0a8ec4807607d5c42
feat: implement G0 runtime foundations
```

GitHub Actions run `33062697653` passed on August 27, 2026:

```text
test (ubuntu-latest)  ✅
test (macos-latest)   ✅
test (windows-latest) ✅
race                   ✅
```

The platform jobs passed `go test ./...`, `go vet ./...` and `go build ./cmd/go-agent`; the race job passed `go test -race ./...`.

The previously listed G0 criterion “snapshots are rebuildable from ledger” is correctly a **G1 criterion**: G0 intentionally contains no Event Ledger. G0 provides the persistence/recovery substrate on which G1 proves deterministic snapshot + replay reconstruction.

Longer SQLite driver soak/comparison and extended long-session stress remain empirical validation work inherited from A0. An executable baseline now exists in `LONG_DURATION_BENCHMARKS.md`; representative reference runs remain pending.

See `G0_EXIT_REVIEW.md`.

---

# G1 — Durable Agent Process + Event Ledger ✅ COMPLETE

## Goal

Prove the central process model before calling any LLM.

## Deliverables

### Agent Process

- stable `AgentID`;
- parent/child relationship metadata;
- lifecycle states from accepted state machine;
- root Intent object;
- versioned status transitions;
- durable sleep/wait representation foundations.

### Event Ledger

- append-only meaningful events;
- causation/correlation IDs;
- accepted global sequence + process-version model;
- pure deterministic reducer;
- versioned snapshot format;
- reconstruct process from snapshot + tail events;
- current process projection.

### Runtime supervision foundation

- activation of runnable durable processes;
- canonical state independent from goroutine lifetime;
- clean shutdown/recovery skeleton.

### CLI inspection

```text
go-agent process create
go-agent process inspect <id>
go-agent process suspend <id>
go-agent process resume <id>
go-agent events <id>
```

## Killer tests

1. create process;
2. append state changes;
3. hard-kill executable;
4. restart;
5. reconstruct exact logical state/version/lineage;
6. verify no in-memory singleton was required;
7. create thousands of waiting processes without one permanent goroutine each.

## G1 result

**PASS.**

Validated on August 27, 2026 with:

- exact state/version/lineage reconstruction after hard kill;
- deterministic full replay and snapshot + tail reconstruction;
- durable request-id idempotency;
- optimistic process-version CAS and atomic ledger/projection/receipt commits;
- stale wake rejection and durable RUNNING recovery;
- parent/child lineage + causal references;
- 10,000 waiting/sleeping-process supervision without one permanent goroutine per process;
- reducer/event hot paths at zero allocations in benchmarks;
- cross-platform daemon/client support including Windows named-pipe dialing.

GitHub Actions run `33074043420` passed:

```text
test (ubuntu-latest)  ✅
test (macos-latest)   ✅
test (windows-latest) ✅
race                   ✅
```

See `G1_EXIT_REVIEW.md`.

---

# G2 — Minimal Agent Loop + Agent Syscalls ✅ COMPLETE

## Goal

Run the smallest useful intelligent process through kernel boundaries.

## Deliverables

### Provider interface

- provider-independent request/event types;
- streaming;
- cancellation;
- token/cost usage accounting when available;
- one frontier provider adapter;
- one OpenAI-compatible adapter for local/vLLM/Ollama-style endpoints;
- deterministic fake provider for tests.

### Initial syscalls

```text
observe
execute
checkpoint
```

### Built-in actions

- read file;
- list directory;
- run command with timeout/output limits.

### Streaming architecture

- model tokens use bounded/coalesced live stream;
- final response persisted as object/artifact;
- canonical events record invocation lifecycle, not one event per token;
- command output streams to Object Store with bounded preview/tail.

### UI

A basic attachable client/CLI is enough. Do not build final TUI yet.

## Completion criteria

Agent can inspect a small repository, run a command and answer while every meaningful action is attributable in Ledger and large output cannot grow hot memory without bound.

## G2 result

**PASS.**

Implemented on August 27, 2026 with:

- provider-neutral messages, tool schemas, tool calls, usage and streaming events;
- deterministic fake provider;
- OpenAI Responses streaming adapter;
- OpenAI-compatible streaming adapter for local endpoints;
- durable `ModelInvocationStarted/Completed/Failed` lifecycle events;
- `observe`, `execute` and `checkpoint` syscalls;
- read-file/list-directory workspace actions;
- argv-based command execution with cancellation, timeout, bounded preview and hard output quota;
- streaming model/command bodies into the content-addressed Object Store;
- bounded live presentation stream with gap-detectable offsets;
- minimal model → syscall → model agent loop;
- attachable `go-agentctl` run/attach client;
- deterministic test proving observe → execute → final-answer flow;
- explicit bounds on steps, tool calls, live bytes and G2 working context.

Implementation commit:

```text
19f3fadcf0343cc53910dd52144bf3dc35d95bcb
feat(g2): implement minimal agent loop and syscalls
```

GitHub Actions run `33083660845` passed:

```text
test (ubuntu-latest)  ✅
test (macos-latest)   ✅
test (windows-latest) ✅
race                   ✅
```

See `G2_EXIT_REVIEW.md`.

---

# G3 — Worlds + Authority + Effect System ✅ COMPLETE

## Goal

Make execution structurally controlled.

## Deliverables

### World API

- `LocalWorld`;
- action/result protocol;
- process execution abstraction;
- filesystem abstraction;
- World lifecycle/Profile;
- streamed output/cancellation contract;
- platform process-tree adapter.

### Capability system

- filesystem read/write scopes;
- process execution scopes;
- network policy foundation;
- delegation subset validation;
- capability leases.

### Effect system

```text
Pure
Read
Reversible
Compensatable
Irreversible
```

plus traits such as idempotency/retryability.

### Intent-Based Authority foundation

- immutable/versioned root Intent;
- acceptance criteria;
- allowed/forbidden effect domains;
- Purpose-Carrying Actions;
- Action Proof;
- typed authorization outcomes.

## Killer tests

- child cannot acquire missing parent capability;
- read-only agent cannot write even if model requests it;
- expired lease fails deterministically;
- denied action never reaches World;
- prompt/tool content cannot mint authority;
- model cannot downgrade Effect classification;
- every security decision is causally visible.

## G3 result

**PASS.**

Implemented on August 28, 2026 with:

- typed World action/result/Profile contracts;
- `LocalWorld` filesystem and process actions confined to a workspace root;
- Unix process-group cancellation and Windows process-tree termination;
- filesystem read/write, process execution and network capability domains;
- scoped grants, leases and delegation-subset validation;
- canonical Effect classification with idempotency/retryability traits;
- versioned Intent policy with allowed/forbidden domains;
- Purpose-Carrying Actions and kernel-generated Action Proofs;
- typed authorization decisions and a security-decision sink;
- `SecureWorld` authorization gate that refuses denied actions before World execution;
- deterministic tests proving all G3 authority killer cases.

Validation run `33203489779` covers `go test ./...`, `go vet ./...`, both binaries on Ubuntu/macOS/Windows, and `go test -race ./...`.

See `G3_EXIT_REVIEW.md`.

---

# G4 — Cognitive MMU v0 ✅ COMPLETE

## Goal

Stop treating conversation history as canonical memory.

## Deliverables

- semantic Context Pages;
- token estimates;
- persisted metadata/object content;
- deterministic tiered working-set builder;
- explicit `recall()`;
- Context Manifest;
- basic structured compaction;
- bounded eviction/cache behavior.

## Killer tests

- accumulated history larger than model context still allows useful work;
- hard token budget never exceeded;
- old relevant fact explicitly recalled;
- 100k-page corpus does not materialize all bodies;
- hot memory/context remains bounded as history grows.

## G4 result

**PASS.**

G4 implements persisted semantic Context Pages, SQLite FTS retrieval, deterministic tiered working-set construction, explicit recall leases, Context Manifests, structured compaction, supersession handling, bounded token-estimate caching and lazy body materialization.

The historical final CI/race pass deferred at G4 closure was later covered by the integrated G5/main validation. Long-duration empirical calibration now has an executable soak harness; reference 1h/8h/24h results remain pending.

See `G4_EXIT_REVIEW.md` and `LONG_DURATION_BENCHMARKS.md`.

---

# G5 — Recursive Agent Processes ✅ COMPLETE

## Goal

Bring recursive-agent benefits into the durable process/security/economy model.

## Deliverables

- durable `spawn()` validate/reserve/create protocol;
- child Task Intent and authority subset;
- result/evidence contract;
- bounded messaging/mailboxes;
- durable parent waits;
- cancellation tree;
- wait-for cycle detection;
- fan-out/depth/fairness controls;
- budget reservations/settlement.

## Killer test

Root delegates three repository investigations in parallel, survives daemon restart with child pending, and receives structured evidence without importing whole child transcripts.

## G5 result

**PASS.**

G5 adds hierarchical budget reservation/settlement, durable bounded mailboxes, restart-safe parent waits, wait-cycle rejection, cancellation propagation, fan-out/depth/parallelism limits, fair per-root admission, result/evidence contracts and explicit completed-work reuse.

The integrated G5 head passed cross-platform tests/vet/builds and the race detector.

See `G5_EXIT_REVIEW.md`.

---

# G6 — Cognitive Scheduler v0 ✅ COMPLETE

## Goal

Stop binding an Agent Process to one model.

## Deliverables

- model registry/Profile;
- Cognitive Task descriptor;
- hard eligibility filtering;
- cost/latency/quality score policy;
- runtime provider health/circuit breaker;
- durable budget reservation/settlement;
- fallback;
- privacy/locality constraints;
- fairness/global/per-root/provider slots;
- routing decision events/metrics;
- daemon/config integration;
- MMU/invocation integration;
- static-baseline evaluation.

## Initial policy

Deterministic rules first.

No learned router in v0. Shadow/advisory routing cannot weaken hard constraints.

## Evaluation

Quality-first, cost-first and latency-first routing are compared against strongest, cheapest and fastest static baselines on a controlled profile matrix. Balanced routing remains an explicit utility tradeoff.

## G6 result

**PASS.**

G6 routes each bounded MMU-built invocation independently while preserving stable Agent identity. It includes hard context/capability/privacy/policy/health/quality/reliability/budget/deadline filtering, deterministic scoring, provider telemetry/circuit breaking, bounded fallback, global/root/provider slots and durable routing events.

Root model-budget accounting is persisted in SQLite by `0006_cognitive_scheduler.sql`. Limits, spent usage and active reservations survive close/reopen; overspend remains rejected after restart and settlement stays exact-once.

GitHub Actions run `34160212580` passed cross-platform tests/vet/builds and `go test -race ./...` on the durable-budget implementation.

See `G6_EXIT_REVIEW.md` and `COGNITIVE_SCHEDULER_V0.md`.

---

# G7 — Workspace/OCI World + Agent Transactions ✅ COMPLETE

## Goal

Make speculative work isolated and reversible where guarantees permit.

## Deliverables

### WorkspaceWorld

- Git-aware isolated workspace;
- captured dirty/untracked base policy;
- target divergence detection;
- three-way promotion.

### OCI World

- controlled mounts;
- restricted network default;
- CPU/memory/time limits;
- secret binding;
- snapshot/fork Profile.

### Transaction API

```text
begin
execute
verify
prepare
commit / rollback / reconcile
```

## Killer tests

- breaking multi-file change fails verification and rolls back exact defined state;
- kill runtime at each transaction boundary;
- crash during APPLY enters reconciliation, never false commit;
- irreversible effect cannot be falsely rolled back.

## G7 result

**PASS.**

G7 implements a Git-aware `WorkspaceWorld` that captures tracked dirty/untracked base state without mutating the user's index, isolates mutations in detached worktrees, detects target divergence and performs promotion through an explicit three-way Git merge with a short target-scoped promotion lease.

The OCI adapter provides controlled bind mounts, network-off/read-only/cap-drop/no-new-privileges defaults, optional CPU/memory/PID/time limits and model-opaque secret-file binding. Its Profile deliberately reports snapshot/fork/promotion as unsupported until those guarantees can be proven rather than faking transaction semantics.

Agent Transactions persist their state, prepared promotion plans, effects, verifications and audit events in SQLite migration `0007_agent_transactions.sql`. The runtime records DISPATCH before crossing the World boundary, defers irreversible effects, sends unknown dispatched outcomes to `NEEDS_RECONCILIATION`, prevents false rollback of unresolved/externally visible effects and revalidates current policy/capability through a commit guard before PREPARE and COMMIT.

Implementation head `da1ce88159bf8f3d51606222fb64413827dd2743` passed GitHub Actions run `34203844173` across Ubuntu/macOS/Windows plus the race detector. Long-duration workflow run `34203844315` also passed on that implementation head.

See `G7_EXIT_REVIEW.md`.

---

# G8 — Cognitive Fork / Safe Execution Editing ✅ COMPLETE

## Goal

Explore alternative futures in parallel without rewriting history.

## Deliverables

- Forkable quiescent checkpoint;
- Execution Frontier;
- isolated branch Agent/World state;
- branch-local memory/context overlay;
- independent budget reservations;
- objective evaluator;
- three-way World merge;
- selective cognitive promotion;
- promotion lease;
- restore-as-new-timeline;
- cleanup/retention.

## Killer demonstration

Implement two solutions to a performance problem, benchmark both in isolated forks, explain comparison and promote only winner with no mutation leakage or history truncation.

## G8 result

**PASS.**

G8 implements quiescent forkable/committable checkpoints with explicit Execution Frontier, exact process-state integrity, required-result preservation, restore-as-new-Agent-timeline semantics, isolated Workspace forks from the checkpoint's immutable Git base, shared immutable Git objects with distinct mutable worktrees, branch-local cognitive overlays, independent pre-admission budget reservations, fork-namespaced actions, deterministic evidence-backed branch evaluation, persisted winner reason and selective provenance-aware cognitive promotion.

Historical checkpoint authority cannot resurrect revoked rights: branch capabilities and allowed Intent domains are the intersection of checkpoint and current authority, while forbidden domains/acceptance criteria accumulate conservatively. Speculative irreversible effects are denied before reaching the inner World.

Winner promotion reuses G7's three-way target/base/source merge, target-scoped promotion lease and reconciliation semantics. Cleanup is idempotent across winner/loser paths and releases owned promotion lease, worktree, synthetic Git refs and unused budget before retention-based purge.

SQLite migration `0008_cognitive_forks.sql` persists checkpoints, Execution Frontier, authority, fork groups/branches, budget state, objective evaluation and cognitive overlays.

The deterministic validation suite includes the two-solution killer demonstration plus Execution Frontier, SQLite persistence, Workspace isolation/COW, authority-intersection, speculative-effect and full cleanup tests. GitHub Actions was intentionally skipped for this closure pass; no unobserved green CI/local-test result is claimed.

See `G8_EXIT_REVIEW.md`.

---

# G9 — Epistemic Memory + Truth Maintenance 🟢 READY

## Goal

Replace flat snippets with evidence-aware knowledge.

## Deliverables

- immutable/versioned Evidence store;
- platform provenance;
- Belief lifecycle;
- scope/temporal validity;
- structured confidence metadata;
- contradiction/dependency edges;
- localized causal invalidation;
- Cognitive MMU integration;
- branch-local memory overlays.

## Killer test

Agent learns repository architecture fact; source changes; old belief is downgraded and cannot silently rank as trusted while original evidence/history remains inspectable.

---

# G10 — Context Faults + Cognitive MMU v2

## Goal

Make missing knowledge a typed runtime paging event.

## Deliverables

- stable semantic cognitive references;
- Reference/Recall/Evidence/Freshness/Dependency/Representation faults;
- context leases;
- dependency-driven page loading;
- fault budgets/storm protection;
- structured compaction ↔ evidence paging;
- improved working-set planning.

## Requirement

Faults remain provider-independent at invocation/tool boundaries and observable/replayable.

---

# G11 — Adaptive Teams + Agent Negotiation

## Goal

Move beyond static multi-agent graphs.

## Deliverables

- team proposal/decomposition;
- scheduler admission;
- temporary specialist profiles;
- bounded claim/challenge/evidence/counterexample/revision protocol;
- disagreement deadlines/round limits;
- escalation.

## Killer demonstration

Reviewer finds race condition, implementer disputes it, reviewer supplies reproducer and they converge—or escalate deterministically—without unlimited dialogue.

---

# G12 — Verified Continual Improvement

## Goal

Allow system improvement without uncontrolled self-modification.

## Deliverables

Versioned cognitive artifacts:

```text
skills
prompts
agent profiles
routing policies
context policies
memory policies
```

Lifecycle:

```text
hypothesis
→ candidate
→ evaluate
→ shadow
→ canary
→ promote/reject
→ rollback
```

## Hard invariants

- cannot expand capabilities;
- cannot rewrite root Intent/effect floor;
- historical invocations retain exact artifact versions;
- security regression rejects candidate even if quality improves.

---

# G13 — Production TUI / Interactive Agent Workspace

## Goal

Turn the durable runtime into a **delightful, fast and deeply inspectable terminal product** that is excellent for both pair-programming and supervising long-running multi-agent work.

G13 should combine the strongest interaction ideas from Claude Code, Codex, Grok Build and Pi without cloning any one product. The TUI remains a replaceable client of the daemon and never owns canonical Agent state.

See `PRODUCTION_TUI_WORKSPACE.md` for the detailed product contract.

## Product principles

- keyboard-first, mouse-capable fullscreen experience;
- progressive disclosure: simple chat by default, deep runtime inspection on demand;
- conversation remains fluid during tools, subagents and long tasks;
- every important Agent operation has headless/runtime API parity;
- no hidden authority: plans/approvals are UX, runtime capabilities/effects remain authoritative;
- UI work scales with the visible viewport and active state, not Agent age/history size;
- visual polish is a product requirement, not an afterthought.

## Inspiration to keep

### Claude Code

- low-friction conversational workflow;
- clear plan/act and permission boundaries;
- compact tool/action progress;
- strong keyboard ergonomics and project customization.

### Codex

- first-class multi-agent/task supervision;
- parallel work that remains understandable;
- goal + success-criteria oriented execution;
- annotation/review workflows around produced work.

### Grok Build

- rich mouse-interactive fullscreen TUI;
- dedicated plan viewer;
- approve/comment/rewrite individual plan steps;
- high-quality inline diff review;
- discoverable skills/plugins/hooks/MCP/subagents.

### Pi

- composable interaction primitives;
- themes/extensions/keymaps;
- model switching;
- tree-structured history;
- steer-running-agent vs queued-follow-up distinction;
- RPC/headless parity.

## Core surfaces

```text
adaptive status/header
Agent/task tree
virtualized conversation
composer + steering/follow-up queue
command palette
plan workspace
diff/change review
transaction inspector
fork comparison
Context/MMU inspector
authority/approval inspector
model/scheduler inspector
history tree/search/bookmarks
artifacts/test output viewer
```

## Required interaction modes

```text
ASK
PLAN
ACT
REVIEW
OBSERVE
```

Modes improve UX but never weaken capability, Intent, Effect or World enforcement.

## Agent cockpit

The user can inspect root/child Agents with state, task, model, World, elapsed time, budget/cost, active action and blocked reason; switch focus without stopping siblings; steer one Agent while others continue; and consume structured child evidence without importing full transcripts.

Large process trees must be virtualized/filterable.

## Plan + review workflow

- complex tasks can enter PLAN before mutation;
- hierarchical plan with dependencies/status;
- inline comments on individual steps;
- rewrite selected step without regenerating everything;
- approve all or selected steps;
- explicit PLAN → ACT transition;
- clean diff review after execution;
- comment on diff hunks and send feedback back to the active Agent.

## G7/G8-native UX

Transactions and forks are first-class UI concepts:

- visible transaction state and verification results;
- commit/rollback/reconcile controls only when semantically valid;
- prominent `NEEDS_RECONCILIATION` uncertainty rather than false success;
- speculative vs promoted changes clearly distinguished;
- branch/fork tree, side-by-side candidate comparison, tests/benchmarks/diffs and winner promotion after G8.

## Context / authority visibility

The TUI should expose:

- MMU working-set occupancy/pages/recalls/Context Faults;
- model/provider/routing objective and fallback state;
- capabilities, Effect class and World guarantees for approval requests;
- cost/budget summaries;
- causal security denials without cluttering normal conversation.

Hidden model reasoning is not displayed; runtime-visible progress/state is.

## Themes and customization

- polished dark default + light theme;
- truecolor with graceful terminal fallbacks;
- declarative themes and semantic color tokens;
- configurable keymaps;
- command/skill palette;
- customizable status-line segments;
- safe external/RPC extension points rather than unsafe kernel-side UI plugins.

## Performance gates

Initial engineering targets on a normal developer machine:

```text
keypress → visible update p95          < 50 ms
stream render target                   20–30 updates/s
idle CPU attached                      near-zero / < 1% typical
100k-block local attach                < 500 ms target
normal viewport resize p95             < 50 ms
history/render memory                  bounded by viewport/cache
```

These targets may be calibrated empirically, but G13 cannot close with full-history rendering, unbounded presentation queues or responsiveness that degrades linearly with session age.

## Killer demonstrations

1. start a long task, kill the TUI, let runtime/subagents continue, relaunch and instantly reattach to current state;
2. open/search/resize/scroll a 100,000-block synthetic Agent session with multi-GB referenced artifacts while memory stays bounded;
3. supervise at least three child Agents in parallel, steer one and queue a follow-up to another without stopping siblings;
4. review/comment/rewrite a plan, execute in WorkspaceWorld, inspect diff/tests, verify and commit through the G7 transaction UI;
5. force `NEEDS_RECONCILIATION` and prove the TUI never presents a false rollback/commit;
6. after G8, compare two fork candidates side by side and promote only the winner;
7. complete the core workflow with keyboard only.

## Completion criteria

```text
production fullscreen TUI
keyboard-first + optional mouse workflow
virtualized long conversation/history
first-class plan and diff review
multi-Agent cockpit
G7 transaction operation/inspection
G8 fork comparison when available
MMU/context inspector
authority/approval inspector
model/scheduler visibility
themes + keymaps + command palette
headless/runtime API parity
macOS/Linux/Windows terminal matrix
100k-block responsiveness benchmark
TUI crash never kills Agent work
```

---

# G14 — Distributed Worlds / Workers

## Goal

Run same durable Agent Process abstraction across local and remote compute.

Candidates:

- SSH worker;
- remote Go worker protocol;
- Kubernetes jobs/workspaces;
- GPU/local inference node routing;
- distributed object/blob storage;
- worker leases/heartbeats/reconciliation.

Agent identity remains independent from worker location.

---

# G15 — Production Observability / Time-Travel Debugger

## Goal

Make complex agent behavior understandable and replayable beyond the everyday G13 workspace.

## Deliverables

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

G15 builds deep forensic/debugging workflows on top of the production interaction primitives established by G13 rather than postponing the basic product UI until observability work.

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

A feature is not done if happy path works but it leaks memory/goroutines, blocks on slow clients or has undefined crash behavior.

---

# Product slices

A credible first coding-agent experience emerged around G5:

```text
single Go binary distribution
local durable daemon
attachable terminal
persistent Agent Processes
model streaming
filesystem/shell actions
Event Ledger
capabilities/effects/Intent
bounded context
recursive subagents
```

G6 added model/resource scheduling and restart-safe model-budget accounting. G7 added isolated speculative workspace mutation, conservative OCI execution and durable commit/rollback/reconciliation semantics. G8 adds safe execution editing, isolated alternative Agent/World futures, objective winner selection and selective promotion. G9–G10 build on this substrate with epistemic memory and typed context faults.

**G13 is the deliberate productization generation:** it turns these runtime primitives into a polished interactive terminal workspace inspired by the best modern coding-agent UX while preserving go-agent's daemon durability, inspectability and headless parity. G14 then expands execution across remote workers; G15 adds deep production observability/time-travel debugging.

---

# Testing philosophy

Most kernel semantics must be testable without an LLM.

Use deterministic fake providers and fake Worlds for:

- process recovery;
- authority subset;
- denied effects;
- transaction rollback;
- fork isolation;
- budget accounting;
- context budgets/faults;
- unknown outcomes;
- slow consumers;
- crash recovery;
- Truth Maintenance propagation;
- scheduler fairness;
- TUI projection/virtualization and reconnect behavior.

Real-model tests evaluate harness/model quality separately.

Long-duration validation uses tagged provider-free soak scenarios and explicit boundedness criteria. See `LONG_DURATION_BENCHMARKS.md`.

---

# Immediate next step

**G9 — Epistemic Memory + Truth Maintenance is READY.**

Build evidence-aware durable knowledge on top of G8's branch-local overlay and selective promotion substrate with:

- immutable/versioned Evidence objects;
- platform/source provenance;
- Belief lifecycle and confidence metadata;
- scope and temporal validity;
- contradiction/dependency edges;
- localized causal invalidation;
- Cognitive MMU integration;
- branch-local belief overlays and promotion rules.

G9 must preserve the kernel invariant that a winning fork may promote verified knowledge with provenance, but speculative branch assumptions do not become trusted global truth merely because the branch won an execution benchmark.
