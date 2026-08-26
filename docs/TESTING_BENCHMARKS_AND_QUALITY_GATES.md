# Testing, Benchmarks and Quality Gates

## Purpose

The architecture should be falsifiable.

Every important claim must eventually have a test or benchmark that can prove the implementation violates it.

Examples:

```text
"durable"          → kill/restart tests
"bounded memory"   → soak/RSS tests
"safe delegation" → capability invariant tests
"transactional"    → rollback/fault-injection tests
"fast TUI"         → long-history UI benchmarks
"replayable"       → deterministic reducer/replay tests
```

---

# 1. Test pyramid

## Pure unit tests

For:

- reducers/state machines;
- effect classification;
- capability subset logic;
- intent policies;
- budget arithmetic;
- context packing;
- scheduler rules;
- belief invalidation;
- protocol encoding.

These must not require an LLM.

## Component tests

For:

- SQLite repositories;
- object store;
- IPC framing;
- LocalWorld;
- provider stream parsers;
- TUI projections;
- timer scheduler.

## Integration tests

For:

- runtime restart;
- model + syscall loop;
- tool cancellation;
- child supervision;
- transaction rollback;
- TUI attach/detach;
- provider fallback.

## End-to-end scenario tests

For real developer-agent workflows with objective checks.

## Soak/fault tests

For long duration, memory stability, concurrency and recovery.

---

# 2. Architecture invariant suite

Maintain a named suite of invariants.

## Process

```text
INV-P01 process identity survives daemon restart
INV-P02 terminal disconnect does not cancel process
INV-P03 impossible lifecycle transitions are rejected
INV-P04 child lineage is immutable after creation
INV-P05 sleeping process requires no dedicated resident worker
```

## Authority/security

```text
INV-S01 child authority ⊆ delegable parent authority
INV-S02 expired/revoked capability cannot authorize action
INV-S03 denied action never reaches World
INV-S04 information/tool output cannot mint authority
INV-S05 secret plaintext is absent from model context by default
```

## Context/memory

```text
INV-C01 context pack never exceeds configured hard budget
INV-C02 evicted context remains recoverable if retained
INV-C03 pinned root intent cannot be silently replaced
INV-C04 invalidated belief is not ranked as fully trusted
```

## Effects/transactions

```text
INV-E01 irreversible action cannot run in speculative branch without barrier policy
INV-E02 rollback restores defined observable World state
INV-E03 unknown-outcome mutation is not blindly retried
INV-E04 fork siblings cannot mutate each other's isolated state
```

## Reliability/performance

```text
INV-R01 queue capacities are bounded/documented
INV-R02 large tool output does not scale process RSS with output size
INV-R03 TUI memory does not scale with total transcript size
INV-R04 process recovery uses snapshot + tail, not lifetime replay when snapshot exists
INV-R05 runtime survives slow/dead UI subscriber without unbounded buffering
```

---

# 3. Deterministic state-machine testing

Every lifecycle reducer should have table-driven and property/state-machine tests.

Example:

```text
CREATED → READY        valid
READY → RUNNING        valid
COMPLETED → RUNNING    invalid
CANCELLED → COMPLETED  invalid
```

Randomized command sequences can assert:

- no invalid state becomes representable;
- replay always yields same projection;
- versions/sequences remain monotonic;
- budget never becomes negative except through explicitly modeled debt semantics (prefer no debt initially).

---

# 4. Race/leak testing

Required development commands:

```bash
go test -race ./...
```

Add goroutine-leak assertions for components that start background work.

Test patterns:

- start/stop runtime 100×;
- start/cancel model stream 1000× with fake provider;
- spawn/cancel children repeatedly;
- connect/disconnect UI clients;
- command cancellation while stdout is active;
- timer registration/removal storms.

Steady-state goroutine count should return near baseline after cleanup.

---

# 5. Fake deterministic model provider

A first-class fake provider is essential.

Capabilities:

- scripted chunks;
- scripted tool calls;
- controllable latency;
- disconnect after chunk N;
- malformed event injection;
- token/cost metadata;
- cancellation observation;
- deterministic replay.

Most agent-runtime tests should use this instead of a paid external model.

---

# 6. Fake deterministic Worlds

Provide test Worlds that can:

- record requested actions;
- fail before/after a simulated side effect;
- report unknown outcome;
- support snapshot/fork in-memory;
- enforce latency;
- expose intentional race/failure cases.

This allows transaction/effect/security semantics to be proven without Docker.

---

# 7. Fault-injection matrix

At minimum inject failure at these boundaries:

```text
before ledger write
after ledger write before execution
during provider stream
after provider completion before outcome event
during blob stream
after blob finalization before DB reference
before World mutation
after World mutation before response
during transaction commit
during rollback
while child is running
while UI is disconnected
while storage is slow/busy
```

For each injection point document expected recovered state.

---

# 8. Performance benchmark suite

## Persistence

```text
BenchmarkLedgerAppend1
BenchmarkLedgerAppendBatch
BenchmarkProjectionRead
BenchmarkReplay10K
BenchmarkReplay1MWithSnapshots
BenchmarkSnapshotSerialize
```

## Object store

```text
BenchmarkBlobWrite1MB
BenchmarkBlobWrite1GBStreaming
BenchmarkBlobReadRange
BenchmarkDeduplicatedPut
```

## Cognitive runtime

```text
BenchmarkContextPack100Pages
BenchmarkContextPack10KMetadata
BenchmarkRecallIndex
BenchmarkBeliefInvalidationGraph
```

## Scheduler

```text
BenchmarkDispatch1KWaitingAgents
BenchmarkBudgetReservation
BenchmarkFairQueue
```

## TUI

```text
BenchmarkRenderVisible50Blocks
BenchmarkStreamCoalescing
BenchmarkAttach100KBlockSession
BenchmarkScroll100KBlockSession
BenchmarkResizeLongSession
```

---

# 9. Soak suite

## SOAK-1 — One-hour interactive

Workload:

- continuous fake-model streaming;
- messages every few seconds;
- tool previews;
- periodic history scroll;
- child processes.

Record every minute:

```text
RSS
heap in-use
total allocations
goroutines
GC CPU
queue depths
DB size
object-store size
render latency
```

Pass criterion:

After warm-up/cache stabilization, heap/RSS must not show growth proportional to message count.

## SOAK-2 — Eight-hour lifecycle

Mixed activity + sleeping + reconnect.

Validate timer/recovery/leak behavior.

## SOAK-3 — 24-hour daemon idle

Thousands of durable sleeping/completed agents.

Daemon should consume negligible CPU and stable memory.

## SOAK-4 — Large output

Stream a controlled multi-GB command output.

Validate bounded RSS, responsive TUI and correct persisted object hash/size.

---

# 10. Crash matrix

Automate process `SIGKILL`/hard termination at random checkpoints.

Scenarios:

- model invocation;
- tool action;
- child creation;
- event append sequence;
- snapshot;
- transaction;
- object finalization.

After restart assert invariants and reconciliation state.

A useful future harness can run hundreds of randomized kill/restart cycles.

---

# 11. TUI anti-lag gate

Before calling the interactive client production-ready:

Seed:

```text
100k conversation blocks
5 GB referenced tool artifacts
100 child-process cards over history
```

Open latest viewport and stream a new answer.

Required behavior:

- no full-history read;
- client RSS bounded by viewport/cache limits;
- p95 input/render remains within target;
- history fetch happens asynchronously;
- model stream does not stutter because old transcript exists.

This benchmark directly protects the user-visible stability objective.

---

# 12. Context longevity evaluation

Synthetic long-running task where required facts were learned far earlier than active context budget allows.

Measure:

- retrieval precision;
- context faults/recall count;
- forgotten critical constraints;
- token cost;
- active context size.

Success means lifetime knowledge can grow while per-call context remains bounded.

---

# 13. Security adversarial suite

Corpus of inputs attempting to:

- instruct agent to reveal secret;
- expand own authority;
- tell child to ignore parent constraints;
- execute outside filesystem scope;
- contact forbidden network domain;
- reclassify irreversible action as read;
- rewrite root intent;
- bypass approval through alternate syscall/tool.

The expected security decision should be deterministic even if model output is adversarial.

---

# 14. Model-quality evaluations

Kernel correctness must not be confused with LLM quality.

Separate evals:

```text
runtime correctness
vs
agent task success
```

Task success scenarios can include:

- fix seeded bug;
- find race;
- implement small feature;
- compare two optimizations;
- research/summarize with evidence.

These evaluate harness/model/scheduler behavior, not storage invariants.

---

# 15. Scheduler evaluations

Dataset of task classes with several models.

Record:

```text
success
verification score
latency
cost
tokens
context size
fallback count
```

Then compare routing policy against baselines:

- always strongest model;
- always cheapest model;
- fixed default;
- scheduler policy.

A scheduler is only useful if it improves an explicit cost/quality/latency frontier.

---

# 16. Self-improvement gate

A candidate cognitive artifact must not be promoted because one session “felt better”.

Required metadata:

```text
hypothesis
baseline version
candidate version
evaluation set
metric(s)
minimum sample/evidence policy
security regression checks
promotion decision
rollback reference
```

No capability/security-policy changes are eligible for autonomous promotion.

---

# 17. Performance regression process

Initially:

- record benchmark history;
- inspect large regressions manually.

Later:

- define stable reference hardware/CI environment;
- gate percentage regressions for key microbenchmarks;
- always gate qualitative boundedness tests regardless of machine.

Example qualitative hard gate:

> streaming 1 GB output must not increase heap by hundreds of MB.

This is more portable than a strict nanosecond threshold.

---

# 18. Architecture gate before implementation

No milestone should begin until its core semantics answer:

1. what state is canonical?
2. what are the lifecycle states?
3. what are the invariants?
4. what can fail?
5. what survives restart?
6. what is bounded?
7. what authority is required?
8. what events are recorded?
9. how is it tested without an LLM?
10. what benchmark proves its non-functional claims?

If these cannot be answered, the concept is not architected enough to implement.

---

# Core rule

> **Every architectural promise must eventually have a failing test with its name on it.**
