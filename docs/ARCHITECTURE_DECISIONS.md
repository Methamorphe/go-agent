# Architecture Decisions

## Status

**A0 decision log — semantic decisions closed.**

See `A0_EXIT_REVIEW.md` for the architecture gate result.

Statuses:

```text
ACCEPTED      semantic baseline; implementation must follow
PROVISIONAL   concrete adapter/package baseline; measurable evidence may replace it
EMPIRICAL     thresholds/mechanics validated during prototype/implementation
DEFERRED      intentionally excluded from early milestones
```

---

# D001 — Go is the kernel/runtime language

**Status:** ACCEPTED

Use Go for runtime daemon, durable process supervision, Event Ledger/reducers, MMU, memory, policy, scheduler, IPC and storage integration.

Python is an Execution World. Rust/native helpers are optional future measured optimizations only.

---

# D002 — Runtime is separate from the TUI

**Status:** ACCEPTED

TUI is a disposable local client over IPC. Closing/crashing it never owns or destroys canonical Agent Process state.

---

# D003 — Local persistence is SQLite + content-addressed Object Store

**Status:** ACCEPTED architecture / PROVISIONAL driver

Canonical metadata/events/projections use SQLite.

Large immutable payloads use a streaming filesystem Object Store.

Initial driver: `modernc.org/sqlite`, behind internal adapter.

---

# D004 — Event-source meaningful transitions, not token/byte streams

**Status:** ACCEPTED

Canonical events represent state transitions. Model tokens/stdout bytes are streamed/object-backed.

---

# D005 — Context window is a bounded cache

**Status:** ACCEPTED

Canonical knowledge/state exists outside model context. Cognitive MMU builds bounded Context Manifests per invocation.

---

# D006 — Security is enforced outside the model

**Status:** ACCEPTED

Capabilities, Intent, effects, budgets and approvals are kernel/runtime policy objects. Prompt text cannot create authority.

---

# D007 — Provider conversation/thread state is non-canonical

**Status:** ACCEPTED

Agent identity persists across model/provider switches and provider outages.

---

# D008 — No unbounded hot-path queues

**Status:** ACCEPTED

Every producer/consumer boundary has capacity, backpressure, coalescing, spill or rejection semantics.

---

# D009 — Large I/O streams by default

**Status:** ACCEPTED

Tool/model/artifact APIs do not require full-buffer `[]byte` materialization.

---

# D010 — Waiting/sleeping Agent Processes are non-resident logical state

**Status:** ACCEPTED

No one-goroutine/ticker-per-sleeper design.

---

# D011 — Recursive agents are durable child processes

**Status:** ACCEPTED

`spawn()` validates delegated task/authority/budget and durably creates process before scheduling execution.

---

# D012 — Effects are first-class typed metadata

**Status:** ACCEPTED

Initial classes:

```text
Pure
Read
Reversible
Compensatable
Irreversible
```

Model cannot downgrade trusted effect floor.

---

# D013 — Unknown external outcomes are first-class

**Status:** ACCEPTED

Dispatched mutation with ambiguous result becomes `OutcomeUnknown` and must reconcile before unsafe retry/edit.

---

# D014 — Speculative mutation requires isolated World state

**Status:** ACCEPTED

A Cognitive Fork is not two prompts sharing one mutable workspace.

---

# D015 — Self-improvement is versioned/evaluated/reversible

**Status:** ACCEPTED semantics / DEFERRED implementation

No silent in-place prompt/skill/routing/context-policy mutation.

---

# D016 — Cognitive references form a stable semantic address space

**Status:** ACCEPTED

Use stable reference families such as:

```text
ctx://
belief://
evidence://
object://
event://
checkpoint://
agent://
```

Storage implementation IDs are not the cognitive ABI.

---

# D017 — Context Faults occur at invocation/tool boundaries

**Status:** ACCEPTED

Correctness never depends on provider-specific mid-token inference suspension/resume.

Explicit recall/reference path is canonical; advanced interception is optional later optimization.

---

# D018 — Context Page granularity is semantic-first

**Status:** ACCEPTED semantics / EMPIRICAL tuning

Prefer semantic units (symbol/section/turn/cluster) then split by size. Initial tuning target roughly 800–2,000 tokens, normal ceiling ~4k.

---

# D019 — Memory preserves raw episodes/evidence

**Status:** ACCEPTED

Consolidated memory never replaces the original evidence required for provenance/re-evaluation.

---

# D020 — Provenance cannot be amplified by consolidation

**Status:** ACCEPTED

Rewriting untrusted evidence into a polished memory does not upgrade source trust/authority.

Memory never mints operational capability.

---

# D021 — Beliefs use typed provenance/dependency/contradiction semantics

**Status:** ACCEPTED

Confidence is multidimensional metadata, not a naked LLM probability treated as truth.

Truth Maintenance propagates staleness locally through typed graph edges.

---

# D022 — Capability and Intent are independent mandatory authorization dimensions

**Status:** ACCEPTED

Capability defines maximum technical authority. Intent defines current task/effect/resource boundary.

Both must pass.

---

# D023 — Actions carry structural purpose references

**Status:** ACCEPTED

Meaningful actions reference current Task Intent / plan / acceptance criteria and produce a versioned Action Proof for authorization observability.

---

# D024 — Semantic intent evaluation is non-authoritative

**Status:** ACCEPTED

A semantic evaluator can detect ambiguity/drift but cannot override hard capability/domain/resource denial or grant missing authority.

High-risk irreversible actions need explicit policy/approval.

---

# D025 — Worlds advertise concrete guarantees

**Status:** ACCEPTED

LocalWorld is host-mediated, not a sandbox.

Snapshot/fork/promotion/network/resource guarantees are discovered and policy-matched.

---

# D026 — Process-tree ownership is platform-specific behind one adapter

**Status:** ACCEPTED semantics / EMPIRICAL adapter

Unix-like systems use process-group/session semantics; Windows uses Job Objects.

---

# D027 — First coding mutation isolation is Git-aware WorkspaceWorld

**Status:** ACCEPTED semantics / EMPIRICAL mechanics

Capture base + dirty/untracked policy, run branch in isolated workspace/worktree, verify, detect target divergence, three-way promote.

---

# D028 — Restore creates a new timeline; history is never truncated

**Status:** ACCEPTED

Checkpoint/fork/restore/merge are execution edits that preserve historical events/external effects.

---

# D029 — v0 mutation-capable forks require quiescent checkpoints

**Status:** ACCEPTED

No unresolved state-changing in-flight operation, unknown mutation outcome or commit critical section.

Advanced non-quiescent exact edit safety is DEFERRED research.

---

# D030 — Merge is three-way and cognitive promotion is selective

**Status:** ACCEPTED

World state merges relative to explicit base. Branch transcript/speculative assumptions are not automatically unioned into parent memory.

---

# D031 — Commit protocol is prepare/apply/verify/finalize with reconciliation

**Status:** ACCEPTED

Use a short promotion lease to prevent stale concurrent promotion. Crash during uncertain APPLY becomes `NeedsReconciliation`.

No generic distributed ACID claim.

---

# D032 — Recursive orchestration uses durable contracts and bounded mailboxes

**Status:** ACCEPTED

Parent waits are durable, fan-out/depth are bounded, wait-for cycles are detected, large inter-agent payloads use refs.

---

# D033 — Agent negotiation is finite/evidence-oriented

**Status:** ACCEPTED

Use claim/challenge/evidence/counterexample/revision/agreement-or-escalation with explicit round/time/budget limits.

---

# D034 — Model routing occurs per Cognitive Task, not per Agent lifetime

**Status:** ACCEPTED

Model is schedulable execution resource; Agent Process owns durable identity/state.

---

# D035 — Scheduler hard-filters before utility scoring

**Status:** ACCEPTED

Privacy/context/capability/health/risk/budget constraints remove candidates before cost/latency/quality scoring.

---

# D036 — Scheduler reserves resources before fan-out

**Status:** ACCEPTED

Budget/concurrency reservations prevent recursive oversubscription. Actual usage settles and unused reservation releases exactly once.

---

# D037 — Learned routing/MMU policies are later evaluated artifacts

**Status:** ACCEPTED semantics / DEFERRED implementation

v0 is deterministic/rule-based. Learned policies use replay/shadow/canary/promotion/rollback and cannot override hard constraints.

---

# D038 — `database/sql` + explicit SQL, no kernel ORM

**Status:** ACCEPTED

Correctness-critical transaction/query semantics remain explicit.

---

# D039 — SQLite starts reliability-first

**Status:** ACCEPTED baseline / EMPIRICAL performance

Initial profile:

```text
WAL
foreign_keys=ON
bounded busy_timeout
synchronous=FULL
```

Relax durability only from measured evidence and explicit consequence review.

---

# D040 — Canonical events and snapshots use versioned JSON v0

**Status:** ACCEPTED

Events are coarse/small; snapshots are rebuildable optimization. Large payloads stay object-backed.

---

# D041 — Object Store uses SHA-256 content addressing v0

**Status:** ACCEPTED

Stream temporary → hash/size → durable finalize → SQLite reference commit.

---

# D042 — Local IPC uses UDS/named pipe + length-framed JSON envelopes

**Status:** ACCEPTED

macOS/Linux use Unix domain socket; Windows named pipe. Loopback TCP only explicit fallback.

Large artifacts never fit one normal IPC frame.

---

# D043 — SQLite FTS is lexical retrieval baseline

**Status:** ACCEPTED baseline

Embedding/vector index is optional derived signal, never correctness dependency.

---

# D044 — TUI library is benchmark-selected later

**Status:** EMPIRICAL / DEFERRED until fixture

Architecture is independent from Bubble Tea or any other library. Candidate must pass long-history/bounded-render benchmarks.

---

# Remaining empirical validation — not semantic OPEN decisions

## E001 SQLite driver

Baseline `modernc.org/sqlite`; compare soak/crash/concurrency/performance with mature alternate binding before reliability claim.

## E002 Git WorkspaceWorld implementation

Validate exact worktree/dirty/untracked capture and promotion mechanics.

## E003 Cross-platform process-tree cancellation

Validate Unix/macOS process-group and Windows Job Object edge cases.

## E004 MMU tuning

Tune page sizes/ranking/compaction/fault budgets without changing semantic contracts.

## E005 Epistemic Memory heuristics

Tune confidence/consolidation/auto-resolution policies.

## E006 Scheduler heuristics

Tune quality priors/utility/latency/hedging.

## E007 TUI library

Select from benchmark evidence.

---

# Explicit deferred research

```text
provider-specific mid-token context paging
non-quiescent exact checkpoint/fork/restore/merge
learned MMU ranking
learned scheduler
strong automatic truth resolution
multi-tenant/remote hardened sandbox
distributed control plane/event ledger
full process-identity branch promotion
continual-improvement implementation
```

These are not G0/G1 blockers.

---

# Decision discipline after A0

A high-coupling semantic change requires:

```text
problem/invariant
alternatives
selected change
persisted-state compatibility
security/failure/performance impact
test updates
revisit trigger
```

Implementation is not allowed to silently replace the specification.
