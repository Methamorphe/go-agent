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

## Decision

Use Go for the durable runtime, supervisor, scheduler, policy boundaries, storage integration and local control plane.

Python may exist as an Execution World/runtime when dynamic computation is useful.

## Why

- strong concurrency/process/networking model;
- single-binary distribution;
- good daemon/service fit;
- operational simplicity;
- performance more than sufficient relative to LLM/network latency;
- encourages explicit interfaces instead of Python-level runtime introspection becoming the kernel.

## Consequence

Dynamic Python execution is an adapter/World, not the canonical state/runtime model.

---

# D002 — Durable runtime is separate from the TUI

**Status:** ACCEPTED

## Decision

The terminal is a local client attached to a durable runtime process/daemon.

One distributable binary may expose both modes, but state ownership is separate.

## Why

This is fundamental to long-session stability:

- UI restart does not kill agents;
- transcript rendering cannot become canonical state;
- detached agents can keep working;
- history can be paginated;
- slow UI can have independent backpressure.

## Consequence

A local IPC protocol and projection API are required.

---

# D003 — SQLite + filesystem object store for local-first persistence

**Status:** PROVISIONAL

## Decision

Use SQLite for canonical metadata/events/projections and a streaming filesystem object store for large immutable payloads.

## Why

- local-first;
- zero external service dependency;
- transactional metadata;
- excellent developer-machine fit;
- large blobs should not live in hot event rows.

## Consequence

Need object finalization, references, integrity, reachability GC and backup consistency.

## Revisit trigger

Measured SQLite limitations under realistic concurrency or future multi-node deployment.

---

# D004 — Event-sourced process transitions, but not byte/token event sourcing

**Status:** ACCEPTED

## Decision

Meaningful canonical transitions are append-only events. High-volume model tokens/stdout bytes use streams + object storage and are summarized by lifecycle events.

## Why

Per-token event sourcing would inflate storage, replay cost and write pressure without adding useful canonical semantics.

## Consequence

Need a precise event catalog and clear distinction between canonical events, presentation events and telemetry.

---

# D005 — Context window is a bounded cache

**Status:** ACCEPTED

## Decision

The LLM context is never the canonical long-term memory of the Agent Process.

Cognitive MMU constructs a bounded working set from durable objects/pages/memory.

## Why

Long-running sessions must not become impossible or increasingly expensive merely because history grows.

## Consequence

Context pages, explicit recall, context traces and structured compaction are kernel/runtime concerns.

---

# D006 — No prompt-only security

**Status:** ACCEPTED

## Decision

Capabilities, delegation, effects, budgets and critical policy are enforced outside the model.

## Why

LLM output is probabilistic and may be influenced by prompt injection/untrusted content.

## Consequence

Tools/Worlds never receive state-changing requests directly from model adapters without kernel mediation.

---

# D007 — Provider thread/session IDs are non-canonical

**Status:** ACCEPTED

## Decision

Agent identity and state cannot depend on OpenAI/Anthropic/other hosted conversation objects.

## Why

- portability;
- provider switching;
- replay;
- local models;
- resilience to provider feature changes.

## Consequence

Model invocation requests are constructed from runtime-owned context/state each turn.

---

# D008 — No unbounded queues in runtime hot paths

**Status:** ACCEPTED

## Decision

All producer/consumer boundaries define explicit capacity or another bounded strategy such as coalescing/spill-to-disk.

## Why

Unbounded queues turn transient slowness into memory leaks/OOM over long sessions.

## Consequence

Every stream contract documents overflow/backpressure behavior.

---

# D009 — Large I/O is streaming by default

**Status:** ACCEPTED

## Decision

Tool output, model response persistence, file/artifact transfer and object storage APIs use streaming readers/writers rather than mandatory full-buffer `[]byte` APIs.

## Why

A tool can emit MB/GB-scale data; hot memory should not scale with payload size.

## Consequence

Preview/tail buffers are separate from canonical full artifacts.

---

# D010 — Canonical waiting state does not require resident execution

**Status:** ACCEPTED

## Decision

Sleeping/waiting Agent Processes are persisted state machines. They do not need one goroutine/ticker each.

## Why

Durable agents may wait hours/days and scale to many inactive processes.

## Consequence

Use centralized scheduler/timer structures and reconstruct them after restart.

---

# D011 — Recursive agents are durable child processes, not helper prompts

**Status:** ACCEPTED

## Decision

`spawn()` creates a child Agent Process with its own state, context, authority and budget.

## Why

This makes recursion inspectable, restartable, budgetable and secure.

## Consequence

Child creation must persist before execution scheduling.

---

# D012 — Effects are first-class metadata

**Status:** ACCEPTED

## Decision

Actions carry typed effect semantics such as Pure/Read/Reversible/Compensatable/Irreversible plus traits.

## Why

Retry, speculation, rollback and approval cannot be safely derived from tool name alone.

## Consequence

Effect descriptors participate in policy before World execution.

---

# D013 — Unknown outcomes are explicit

**Status:** ACCEPTED

## Decision

When an external mutation may or may not have happened, represent `OutcomeUnknown` and reconcile before retry.

## Why

False certainty causes duplicate irreversible effects and data corruption.

## Consequence

Failure/result types need outcome certainty separate from error/retry class.

---

# D014 — Speculative execution requires isolated World state

**Status:** ACCEPTED at semantic level

## Decision

A Cognitive Fork is not merely two prompts. Branches exploring mutating alternatives need isolated World state when mutation is involved.

## Why

Otherwise branches contaminate each other and comparison becomes meaningless/dangerous.

## Consequence

Fork support depends on World snapshot/fork capabilities; unsupported Worlds must reject/degrade explicitly.

---

# D015 — Self-improvement is versioned and evaluated

**Status:** ACCEPTED at semantic level / DEFERRED implementation

## Decision

Prompts, skills, routing/context policies and agent profiles cannot mutate silently in place. Candidate changes require versioning/evaluation/promotion/rollback.

## Why

Long-lived self-modification without evaluation is unreproducible and unsafe.

## Consequence

Historical model invocations should eventually record exact cognitive artifact versions.

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

## O002 — Canonical event ordering/version model

**Status:** OPEN

Need exact choice for:

- global sequence vs process-local sequence;
- expected-version concurrency;
- cross-agent causal order;
- event IDs.

Must be settled before Event Ledger implementation.

---

## O003 — Snapshot serialization format

**Status:** OPEN

Need versioned, deterministic-enough format and migration strategy.

Candidates include JSON, CBOR/msgpack-like formats, protobuf-like schema, custom typed encoding.

Optimize for evolvability/debuggability before raw size.

---

## O004 — Local IPC transport and framing

**Status:** OPEN before final TUI, not necessarily before G0

Need:

- Unix socket / named pipe strategy;
- fallback transport;
- message framing;
- protocol versioning;
- authentication/ownership for local socket;
- reconnect cursors.

---

## O005 — TUI library

**Status:** DEFERRED / benchmark-driven

Bubble Tea or another library may be used only if it supports long-history virtualization and bounded incremental rendering cleanly.

Architecture must remain independent from library-specific state ownership.

---

# Open decisions before controlled execution milestones

## O006 — Capability expression grammar

Need path/network/process/secret scopes with a decidable subset/intersection relation.

Avoid arbitrary policy language in v0 if a typed model is enough.

## O007 — Dynamic effect classification

Decide how static action declarations combine with runtime-specific effects.

Example: file write inside COW World may be Reversible while same write directly on host may have different guarantees.

## O008 — World action protocol

Need exact request/result/cancellation/output-stream contracts while keeping World interface small.

## O009 — Process-tree termination semantics

Cross-platform differences for local subprocess descendants must be researched/tested on macOS/Linux/Windows.

---

# Open research decisions before Cognitive MMU milestone

## O010 — Context-page granularity

File/section/message/tool-result sizing and segmentation policy.

## O011 — Token estimation

Provider/model-specific tokenizers vs conservative estimates/caches.

## O012 — Ranking/packing baseline

Need deterministic v0 formula combining explicit refs, recency, scope, relevance, dependency and importance.

Do not begin with learned ranking.

## O013 — Structured compaction triggers

Define when raw pages become summary pages and how evidence links are preserved.

---

# Open research decisions before Transactions/Forks

## O014 — Local isolated workspace primitive

Candidates:

- Git worktrees;
- copy-on-write filesystem strategy;
- OCI overlay layers;
- combinations.

Need cross-platform behavior and promotion semantics.

## O015 — Commit protocol

Define how an isolated World promotes changes atomically/observably, especially when multiple resource types are involved.

## O016 — Forked cognitive-state model

Define which memory/context changes are branch-local overlays and how winning state merges into parent.

---

# Open research decisions before Epistemic Memory

## O017 — Belief representation

Free-text statement + metadata versus partially structured predicates/types.

Likely start text-first with typed scope/provenance/status, while preserving future structured facts.

## O018 — Confidence semantics

Avoid presenting arbitrary LLM confidence as objective probability. Define categories/signals and ranking behavior.

## O019 — Dependency-edge types

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
