# Architecture Decisions

## Purpose

This is the initial decision log for architecture phase A0.

It is intentionally lighter than one-file-per-ADR while the design is moving quickly. Before implementation, decisions that become high-cost to reverse can be split into formal ADR files.

Statuses:

```text
ACCEPTED      current architecture baseline
PROVISIONAL   direction accepted but prototype/measurement may change details
OPEN          must be decided before relevant implementation milestone
DEFERRED      intentionally not decided yet
```

---

# D001 — Go is the kernel/runtime language

**Status:** ACCEPTED

Use Go for the durable runtime, supervisor, scheduler, policy boundaries, storage integration and local control plane. Python may exist as an Execution World/runtime when dynamic computation is useful.

Why: concurrency/process/networking model, single-binary distribution, daemon/service fit, operational simplicity, sufficient performance and explicit interfaces.

---

# D002 — Durable runtime is separate from the TUI

**Status:** ACCEPTED

The terminal is a local client attached to a durable runtime process/daemon. One distributable binary may expose both modes, but state ownership is separate.

Consequences:

- UI restart does not kill agents;
- transcript rendering cannot become canonical state;
- detached agents continue;
- history is paginated;
- slow UI has independent backpressure;
- local IPC/projection API required.

See `LOCAL_CONTROL_PROTOCOL.md` and `TUI_AND_STREAMING.md`.

---

# D003 — SQLite + filesystem object store for local-first persistence

**Status:** PROVISIONAL

Use SQLite for canonical metadata/events/projections and a streaming filesystem content-addressed object store for large immutable payloads.

Revisit if measured SQLite behavior under realistic concurrency or future multi-node requirements becomes unsuitable.

---

# D004 — Event-sourced process transitions, but not byte/token event sourcing

**Status:** ACCEPTED

Meaningful canonical transitions are append-only events. High-volume model tokens/stdout bytes use bounded streams + object storage and are represented by lifecycle/outcome events.

See `EVENT_MODEL_AND_CATALOG.md`.

---

# D005 — Context window is a bounded cache

**Status:** ACCEPTED

The LLM context is never canonical long-term Agent Process memory. Cognitive MMU constructs a bounded working set from durable pages/objects/memory.

See `COGNITIVE_MMU_V0_ALGORITHM.md`.

---

# D006 — No prompt-only security

**Status:** ACCEPTED

Capabilities, delegation, effects, budgets and critical policy are enforced outside the model. Tools/Worlds never receive state-changing model requests without kernel mediation.

---

# D007 — Provider thread/session IDs are non-canonical

**Status:** ACCEPTED

Agent identity/state cannot depend on hosted provider conversation objects. Model requests are constructed from runtime-owned context/state.

---

# D008 — No unbounded queues in runtime hot paths

**Status:** ACCEPTED

All producer/consumer boundaries define capacity or another bounded strategy such as coalescing/spill-to-disk/rejection.

See `CONCURRENCY_AND_BACKPRESSURE.md`.

---

# D009 — Large I/O is streaming by default

**Status:** ACCEPTED

Tool output, model response persistence, file/artifact transfer and object storage use streaming readers/writers rather than mandatory full-buffer APIs.

---

# D010 — Canonical waiting state does not require resident execution

**Status:** ACCEPTED

Sleeping/waiting Agent Processes are persisted state machines. They do not need one goroutine/ticker each. Central durable scheduler/timer structures are rebuilt after restart.

See `AGENT_PROCESS_STATE_MACHINE.md`.

---

# D011 — Recursive agents are durable child processes, not helper prompts

**Status:** ACCEPTED

`spawn()` creates a child Agent Process with independent state/context/authority/budget. Child creation is durable before execution scheduling.

---

# D012 — Effects are first-class metadata

**Status:** ACCEPTED

Actions carry typed effect semantics such as Pure/Read/Reversible/Compensatable/Irreversible plus traits. Effect descriptors participate in authorization before World execution.

---

# D013 — Unknown outcomes are explicit

**Status:** ACCEPTED

When an external mutation may or may not have happened, represent `OutcomeUnknown` and reconcile before retry.

---

# D014 — Speculative execution requires isolated World state

**Status:** ACCEPTED at semantic level

A Cognitive Fork is not merely two prompts. Mutating alternatives require isolated World state. Unsupported Worlds reject/degrade explicitly.

See `TRANSACTIONS_AND_COGNITIVE_FORKS.md`.

---

# D015 — Self-improvement is versioned and evaluated

**Status:** ACCEPTED at semantic level / DEFERRED implementation

Prompts, skills, routing/context policies and agent profiles cannot mutate silently. Candidate changes require versioning/evaluation/promotion/rollback.

---

# D016 — Event ordering uses process version + local ledger sequence + causal references

**Status:** ACCEPTED for local-first v0

Use:

```text
process-local monotonic version
+
local global ledger sequence
+
EventID / causation / correlation references
```

Timestamps are metadata, not correctness ordering.

Expected process version provides optimistic concurrency.

See `EVENT_MODEL_AND_CATALOG.md`.

Revisit for G13 distributed execution if required.

---

# D017 — Local control protocol uses framed versioned messages and cursor history

**Status:** PROVISIONAL at encoding/transport detail, ACCEPTED semantics

Protocol semantics:

- local Unix socket / named pipe preferred;
- explicit length framing;
- versioned envelopes;
- JSON payloads initially for debugability;
- cursor-based conversation history;
- bounded per-client queues;
- reconstructable presentation stream;
- object references/streaming for large artifacts.

See `LOCAL_CONTROL_PROTOCOL.md`.

Encoding can change after benchmark without changing message semantics.

---

# D018 — Capability model is typed and subset-decidable

**Status:** ACCEPTED for v0 direction

Initial capability families:

```text
FilesystemCapability
ProcessCapability
NetworkCapability
SecretCapability
AgentCapability
WorldCapability
```

Delegation must support deterministic subset/intersection checks. Do not build a general arbitrary policy language in v0.

See `CAPABILITY_AND_INTENT_MODEL.md`.

---

# D019 — World actions are immutable authorized descriptors

**Status:** ACCEPTED semantics

The model does not send arbitrary execution JSON directly to a World. Kernel parses/validates, derives effects, authorizes and produces an immutable `AuthorizedAction` with explicit AgentID/WorldID/effects/output policy.

Large outputs stream via object sink + bounded preview.

See `WORLD_ACTION_AND_EFFECT_PROTOCOL.md`.

---

# D020 — Cognitive MMU v0 is deterministic and explicit-recall-first

**Status:** ACCEPTED for G4 baseline

v0 uses:

- hard token budget;
- tiered mandatory/active/recalled/relevant/recent context;
- deterministic explainable ranking/packing;
- explicit `recall()`;
- context manifests;
- structured compaction links.

No learned ranking or magical mid-stream Context Fault required initially.

See `COGNITIVE_MMU_V0_ALGORITHM.md`.

---

# D021 — Transactions do not claim generic distributed ACID

**Status:** ACCEPTED semantic boundary

Agent Transactions guarantee only what the participating World/effect mechanisms can actually provide. Irreversible effects are deferred/barriered when possible, and commit conflicts/unknown outcomes require reconciliation.

First concrete target: isolated developer workspace using Git worktree/OCI-like semantics with objective verification.

See `TRANSACTIONS_AND_COGNITIVE_FORKS.md`.

---

# Open decisions required before G0/G1

## O001 — SQLite driver

**Status:** OPEN

Candidates:

- pure-Go SQLite implementation;
- CGO SQLite binding.

Criteria:

- reliability;
- platform distribution;
- WAL/concurrency behavior;
- backup API/features;
- performance;
- build complexity.

Do not choose solely to avoid CGO without measuring tradeoffs.

---

## O002 — Snapshot serialization format

**Status:** OPEN

Need versioned, deterministic-enough format and migration strategy.

Candidates include JSON, CBOR/msgpack-like formats, protobuf-like schema or another typed encoding.

Optimize for evolvability/debuggability before raw size.

---

## O003 — Exact G1 activation lease representation

**Status:** OPEN, narrow

Single-daemon v0 can likely rely on optimistic process version + runtime execution lease metadata. Need exact persisted fields/recovery behavior before G1 code.

Distributed heartbeat/lease semantics remain G13.

---

# Decisions before final TUI implementation

## O004 — Exact local IPC transport implementation

**Status:** PROVISIONAL semantics, implementation OPEN

Semantics/framing are specified. Need choose actual libraries/platform implementation for Unix sockets/named pipes and local ownership security.

## O005 — TUI library

**Status:** DEFERRED / benchmark-driven

Bubble Tea or another library is acceptable only if it supports long-history virtualization and bounded incremental rendering cleanly.

---

# Open decisions before controlled execution milestones

## O006 — Filesystem selector/path grammar details

**Status:** OPEN, bounded by accepted typed-capability design

Need exact path canonicalization/glob/symlink semantics while preserving decidable subset checks across Worlds/platforms.

## O007 — Dynamic effect classification details

**Status:** OPEN, semantics constrained

Static action definition sets minimum safe effect. Runtime/World can strengthen/refine, never weaken. Need exact descriptor merging rules.

## O008 — Local process-tree termination contract

**Status:** OPEN

Research/test macOS/Linux/Windows descendant process termination, job/process-group semantics and timeout guarantees.

---

# Open research decisions before Cognitive MMU milestone

## O009 — Context-page granularity

File/section/message/tool-result sizing and segmentation policy.

## O010 — Token estimator implementations

Provider/model-specific tokenizers vs conservative estimators/caches.

## O011 — Structured compaction triggers

When raw pages become summaries and how aggressively old raw pages leave default candidate sets.

Ranking/packing baseline itself is now defined in `COGNITIVE_MMU_V0_ALGORITHM.md`.

---

# Open research decisions before Transactions/Forks

## O012 — Local isolated workspace implementation

**Status:** OPEN

Candidates:

- Git worktrees;
- copy-on-write filesystem strategy;
- OCI overlay layers;
- combinations.

Semantics/promotion requirements are defined; implementation must be benchmarked cross-platform.

## O013 — Exact commit/promotion mechanism

**Status:** OPEN

Need concrete workspace promotion protocol, base-divergence handling and crash reconciliation for selected first World.

## O014 — Cognitive overlay merge details

**Status:** OPEN

Current preferred semantics: branch context/memory are overlays and promotion is selective. Need exact merge rules before G8.

---

# Open research decisions before Epistemic Memory

## O015 — Belief representation

Free-text statement + metadata versus partially structured predicates/types.

Likely start text-first with typed scope/provenance/status.

## O016 — Confidence semantics

Avoid representing arbitrary LLM confidence as objective probability. Define evidence categories/signals/ranking behavior.

## O017 — Dependency-edge types

Differentiate derivation, support, contradiction, supersession and weak association.

---

# Decision discipline

Before coding a high-cost open decision:

1. state the question;
2. list 2–4 realistic options;
3. define evaluation criteria;
4. prototype/benchmark if uncertainty is empirical;
5. record selected decision and consequences;
6. define what evidence would make us revisit it.

The goal is not to eliminate change. It is to make change intentional rather than accidental.
