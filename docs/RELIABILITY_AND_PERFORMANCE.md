# Reliability, Stability and Performance Architecture

## Purpose

Reliability and performance are not polish items for this project. They are **kernel requirements**.

A long-running autonomous agent is useless if:

- the terminal becomes sluggish after an hour;
- resident memory grows with conversation length;
- goroutines leak after cancelled tools;
- a provider stream can block the whole runtime;
- a large command output freezes the UI;
- SQLite becomes a token-by-token bottleneck;
- a crash loses an hour of work;
- reconnecting to a session requires replaying its entire history into RAM;
- one pathological child agent starves every other process.

The design therefore treats bounded resource usage, backpressure, crash recovery and observability as architectural invariants.

---

# 1. Reliability thesis

The system should behave more like a durable service/runtime than an interactive chat application.

```text
                 durable state
                      │
                      ▼
┌──────────────────────────────────────┐
│            Agent Runtime             │
│                                      │
│ bounded queues   supervised workers  │
│ durable events   isolated failures   │
│ snapshots        resource budgets    │
└───────────────────┬──────────────────┘
                    │
             local IPC / stream
                    │
                    ▼
             lightweight TUI
```

The TUI is **not** the owner of agent state.

The model stream is **not** the event ledger.

The event ledger is **not** an in-memory chat transcript.

Large tool outputs are **not** kept in hot memory.

---

# 2. Core non-functional invariants

## NFR-R01 — Bounded hot memory

For a fixed number of active agents, resident memory MUST NOT grow linearly with historical conversation length.

Historical events, transcripts and artifacts must be persisted and loaded on demand.

Expected shape:

```text
memory ≈ active processes + bounded caches + active streams

not

memory ≈ entire lifetime history
```

## NFR-R02 — Bounded active context

Every model invocation MUST have an explicit context budget.

The Cognitive MMU MUST reject/evict pages rather than silently exceed the budget.

## NFR-R03 — Bounded queues

Every asynchronous queue/channel that can receive untrusted or externally paced data MUST be bounded or have an explicit overflow policy.

Unbounded Go channels implemented through slices/queues are forbidden in kernel hot paths unless a proof exists that their input is intrinsically bounded.

## NFR-R04 — Backpressure end-to-end

A slow consumer MUST NOT cause unbounded buffering upstream.

Provider stream → runtime → persistence → UI must define backpressure/coalescing/drop policies independently.

## NFR-R05 — Failure isolation

Failure of one:

- model request;
- tool execution;
- child process;
- TUI client;
- optional World worker;

MUST NOT crash unrelated durable Agent Processes.

## NFR-R06 — Restartability

The runtime MUST be able to restart and reconstruct logical process state from durable storage.

## NFR-R07 — Deterministic reducers

Given the same snapshot + canonical events, runtime state reducers MUST produce the same logical state.

Wall clocks, random IDs and external queries must not be hidden inside reducers.

## NFR-R08 — No UI ownership

Closing, crashing or replacing the TUI MUST NOT destroy an active agent.

## NFR-R09 — Resource accountability

CPU time, wall time, model tokens/cost, storage growth, child count, tool execution and large artifacts SHOULD be attributable to AgentID/InvocationID/WorldID.

## NFR-R10 — Graceful degradation

When limits are reached, the system should degrade explicitly:

```text
pause / reject / summarize / persist / evict / throttle
```

rather than become progressively slower.

---

# 3. Three-plane architecture

Separating concerns is essential for long-session stability.

## Control plane

Low-volume, canonical state transitions:

- process lifecycle;
- intents;
- capabilities;
- budgets;
- transaction state;
- model invocation lifecycle;
- tool lifecycle;
- checkpoints;
- approvals.

This is persisted durably.

## Data plane

Potentially high-volume data:

- model token streams;
- command stdout/stderr;
- file contents;
- logs;
- search results;
- browser output;
- generated artifacts.

The data plane uses bounded streaming and blob/object storage.

It MUST NOT translate every byte/token into a canonical relational event.

## Presentation plane

The TUI consumes a derived stream of UI events and paginated historical data.

It owns:

- viewport state;
- local keybindings;
- rendering caches;
- selected process/tab;
- transient animation state.

It does not own canonical agent history.

---

# 4. Long-session memory model

A one-hour, one-day or one-week session should use roughly the same amount of hot memory if the active working set is similar.

## Hot

Allowed to remain in RAM:

- active AgentProcess projections;
- current bounded context pages;
- recent event cache;
- current model response assembly buffer;
- bounded stdout/stderr tail;
- TUI visible viewport;
- small indexes/caches with hard limits.

## Warm

Persisted, cheaply retrievable:

- recent message segments;
- structured summaries;
- process snapshots;
- recent tool results;
- context page metadata.

## Cold

Blob/object store:

- full command outputs;
- old message bodies;
- large logs;
- file snapshots;
- model artifacts;
- trace payloads;
- archived context pages.

The runtime must prefer references:

```text
object://sha256:...
```

over copying large payloads across subsystems.

---

# 5. TUI performance targets

These are initial engineering targets, not externally promised SLAs. They should become benchmark gates.

For one normal interactive session on a modern developer machine:

| Metric | Initial target |
|---|---:|
| Key input → visible response p95 | < 50 ms |
| UI render work p95 | < 16 ms when a frame is requested |
| Token rendering refresh | capped/coalesced, typically 20–30 Hz max |
| Attach to existing session | < 250 ms for metadata + initial viewport |
| Scroll old history | paginated, no full-history materialization |
| Idle TUI CPU | approximately zero / event-driven |
| Historical messages retained in TUI RAM | bounded viewport/cache, not full session |

60 FPS is not itself a goal. **Responsiveness and bounded work per frame** are.

---

# 6. Runtime performance targets

Initial targets should be validated on macOS and Linux first.

| Metric | Design target |
|---|---|
| Kernel action-routing overhead | negligible versus tool/model latency; target sub-ms to low-ms locally |
| Event append latency | batchable, non-blocking to presentation path |
| Resume from snapshot | proportional to events since snapshot, not lifetime events |
| Waiting agent resident compute | no goroutine required solely to represent durable waiting state |
| Cancel propagation | immediate in-memory signal + durable state transition |
| Active stream buffering | explicitly bounded per invocation/tool |
| Blob storage | streaming writes, no full payload copy required |

Hard numeric memory limits should be established from benchmarks after G0 prototypes. Until then the architectural requirement is **boundedness**, not an arbitrary MB number.

---

# 7. Streaming strategy

## Model tokens

Persisting one database event per token is prohibited.

Recommended semantics:

```text
ModelInvocationStarted        canonical event
        │
        ├── token/chunk stream    ephemeral bounded stream
        │      │
        │      ├── TUI coalescer
        │      └── response assembler
        │
ModelResponseArtifact         persisted content/blob
ModelInvocationCompleted      canonical event
```

Optionally persist coarse recovery chunks every N KB / N seconds if future provider-resume behavior benefits from it.

On crash during a stream:

```text
InvocationStarted
(no Completed)
```

is enough to classify the invocation as interrupted during recovery.

## Tool stdout/stderr

Do not accumulate arbitrary command output in a `bytes.Buffer`.

Use:

```text
process pipe
   │
   ├── bounded tail buffer → UI
   ├── streaming blob writer → object store
   └── output limiter / cancellation policy
```

The UI can show the last N lines while the full output remains addressable on disk.

---

# 8. Queue and backpressure policy

Every stream must document:

```text
producer
consumer
capacity
overflow behavior
cancellation behavior
persistence behavior
```

Possible overflow policies:

- block producer;
- coalesce adjacent updates;
- keep latest;
- drop non-canonical telemetry;
- spill to disk;
- fail invocation when loss would violate correctness.

Examples:

### UI token updates

Safe to coalesce.

### Security decision events

Never drop.

### stdout display chunks

Can coalesce/drop from live display because canonical full output is streamed to disk.

### ledger commands

Must use durable/acknowledged write semantics.

---

# 9. Goroutine discipline

Goroutines are cheap, not free.

Rules:

1. every goroutine has a named owner component;
2. every long-lived goroutine has an explicit stop condition;
3. every request goroutine receives cancellation;
4. channels are closed by the producer/owner according to documented ownership;
5. no goroutine is created merely to represent persisted `SLEEPING`/`WAITING` state;
6. periodic background loops should be consolidated into schedulers/timers rather than one ticker per agent;
7. integration tests must include goroutine-leak detection;
8. panic boundaries around untrusted worker tasks should prevent runtime-wide crashes where practical.

A useful review question for every `go fn()`:

> Who owns this goroutine, and what guarantees that it terminates?

---

# 10. SQLite performance and safety

SQLite remains a good local-first choice if used deliberately.

Recommended baseline:

- WAL mode;
- short explicit transactions;
- schema migrations;
- busy timeout;
- canonical event writes serialized/batched where appropriate;
- read connections separated from write path as needed;
- large payloads outside event rows;
- indexes based on observed queries, not speculative indexing;
- periodic checkpoint policy;
- corruption/recovery test coverage.

Do not write one row per streamed token.

A single logical event should typically be small metadata plus references to blobs.

---

# 11. Blob/object storage

Large immutable content should use content-addressed storage where practical.

Example:

```text
objects/
  ab/
    cd/<sha256>
```

Metadata in SQLite:

```text
ObjectID
hash
size
media type
compression
created_at
refcount/reachability metadata
```

Benefits:

- deduplication;
- zero need to duplicate large content across events;
- stable references;
- integrity verification;
- simpler export/replay.

Garbage collection must be explicit and reachability-aware; never delete blobs only because they are old.

---

# 12. Snapshot policy

Replay cost must stay bounded.

Snapshots may be triggered by a combination of:

- canonical event count since previous snapshot;
- serialized state size;
- elapsed active time;
- lifecycle boundaries;
- transaction/checkpoint events.

Snapshot creation should not block interactive streams for noticeable periods.

Snapshot format must be versioned.

Old snapshots need either migration support or fallback replay from an earlier compatible point.

---

# 13. Cache policy

Every cache needs:

```text
maximum entries/bytes
TTL or eviction policy
owner
metrics
cache-miss fallback
```

Forbidden pattern:

```go
map[ID]HugeObject // grows forever
```

Candidate bounded caches:

- context-page metadata;
- token estimates;
- recent process projections;
- recent UI history windows;
- model metadata;
- compiled policy results.

Cache correctness must never be required for recovery.

---

# 14. CPU control

The runtime should remain mostly I/O-bound.

Potential CPU hotspots:

- terminal rendering;
- repeated Markdown parsing;
- JSON serialization;
- embedding/re-ranking;
- compression;
- large diff computation;
- context packing;
- event replay;
- runaway agent spawning.

Controls:

- incremental/virtualized rendering;
- memoized parsed message blocks with bounded cache;
- worker pools for CPU-heavy tasks;
- configurable concurrency ceilings;
- profiler hooks;
- no busy polling;
- coalesced timers/events.

---

# 15. Resource budgets beyond LLM cost

Agent Economy should eventually budget runtime resources too.

```go
type RuntimeBudget struct {
    MaxResidentBytesHint int64
    MaxArtifactBytes     int64
    MaxConcurrentTools   int
    MaxConcurrentModels  int
    MaxChildProcesses    int
    MaxCommandOutput     int64
    MaxCommandRuntime    time.Duration
    MaxContextTokens     int64
}
```

Some limits are hard kernel constraints; others are policy hints.

The scheduler should apply fairness so one recursive tree cannot monopolize all worker slots.

---

# 16. Long-running soak tests

Performance cannot be validated by five-minute demos.

Required automated scenarios should eventually include:

## 1-hour synthetic conversation

- continuous streaming;
- thousands of messages/events;
- scrolling history;
- periodic tool calls;
- RSS and goroutine counts sampled.

Pass criterion: no monotonic unbounded growth after caches reach steady state.

## 8-hour idle/active mixed session

- sleep/wake;
- reconnect TUI repeatedly;
- provider failures;
- child creation/cancellation.

## Large-output tool

Generate multi-GB stdout-like data in a controlled test.

Pass criterion: output streams to disk while hot memory stays bounded and UI remains responsive.

## Event-history stress

Millions of historical events with snapshots.

Pass criterion: attach/resume work depends primarily on current projection + tail events rather than scanning full history.

## Slow-client test

Artificially freeze the TUI consumer.

Pass criterion: runtime continues safely without accumulating unbounded presentation messages.

---

# 17. Profiling and telemetry

Development builds should make it easy to inspect:

- goroutine count;
- heap/RSS;
- allocation rate;
- GC pause/time;
- queue depth;
- dropped/coalesced UI updates;
- SQLite write latency;
- blob throughput;
- active model/tool calls;
- context packing time;
- render duration;
- event loop lag.

Go `pprof`, runtime metrics and OpenTelemetry-compatible metrics/traces should be supported without making telemetry mandatory for correctness.

---

# 18. Performance regression gates

Performance bugs should be test failures, not anecdotes.

Maintain versioned benchmarks such as:

```text
BenchmarkEventAppend
BenchmarkStateReplay10K
BenchmarkStateReplay1MWithSnapshot
BenchmarkContextPack
BenchmarkBlobStream1GB
BenchmarkUIRender100Messages
BenchmarkUITail100KHistory
BenchmarkSpawn100WaitingAgents
```

CI can initially track results without hard thresholds. Once stable baselines exist, regressions above an agreed percentage should require review.

---

# 19. Reliability design rule

When choosing between two implementations, prefer the one whose resource use and failure behavior can be **bounded and explained**.

The key product promise is not that the runtime is infinitely fast.

It is:

> **A session can become old and knowledgeable without becoming heavy and fragile.**
