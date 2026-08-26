# State, Persistence and Storage Architecture

## Purpose

Durability is one of the product's defining properties.

The runtime must be able to answer:

> What is the canonical state of this agent if every Go process disappears right now?

If the answer is “whatever was in RAM”, the architecture has failed.

This document defines the separation between canonical state, projections, ephemeral execution state and large artifacts.

---

# 1. State classes

## Canonical durable state

Must survive runtime death:

- Agent Process identity/lifecycle;
- immutable root intent;
- authority/capability leases;
- budgets/reservations;
- parent/child/fork lineage;
- transaction/checkpoint state;
- completed model/tool invocation metadata;
- memory/belief metadata;
- durable wake/sleep conditions;
- references to artifacts/context pages;
- user approvals/denials.

## Durable artifacts

Persisted but not necessarily relational:

- model response bodies;
- command output;
- file snapshots;
- context page contents;
- logs;
- generated assets;
- large structured results.

## Reconstructable projections

Optimized query views:

- current ProcessSummary;
- process tree;
- conversation block index;
- current budget usage;
- current capabilities;
- pending approvals.

These can be rebuilt from canonical data if needed.

## Ephemeral execution state

May disappear on crash:

- active goroutines;
- network sockets;
- partial UI frame;
- provider stream decoder buffers;
- bounded cache entries;
- local rendering AST cache.

Crash recovery must know how to reconcile interrupted operations whose ephemeral state vanished.

---

# 2. Event-sourced core, not event-sourced everything

The Event Ledger should capture **meaningful state transitions**.

It should not become a byte-level log of every event in the universe.

Good canonical events:

```text
AgentCreated
AgentStatusChanged
IntentBound
CapabilityGranted
BudgetReserved
ModelInvocationStarted
ModelInvocationCompleted
ToolActionAuthorized
ToolActionCompleted
TransactionCommitted
BeliefAdded
CheckpointCreated
```

Bad canonical event granularity:

```text
TokenReceived("a")
TokenReceived("b")
StdoutByteReceived(...)
SpinnerAdvanced
MouseMoved
```

High-volume content belongs in streaming/object layers.

---

# 3. Canonical transaction boundary

A logical state transition must be atomic at the storage level where practical.

Example: spawning a child should not result in a child row without the event/budget reservation that justified it.

Possible transaction:

```text
BEGIN
  append ChildCreated
  append BudgetReserved
  create/update process projection
  create resource reservation
COMMIT
```

If the database commit fails, the runtime must not act as if the child exists.

---

# 4. Command → validate → persist → execute model

For state-changing operations, prefer:

```text
Command
  ↓
validate current state/policy
  ↓
persist authoritative transition/intention
  ↓
acknowledge accepted work
  ↓
perform external/ephemeral execution
  ↓
persist outcome
```

For example model invocation:

```text
persist ModelInvocationStarted
        ↓
call provider
        ↓
persist response artifact
        ↓
persist ModelInvocationCompleted
```

A crash after `Started` but before `Completed` produces an explicit interrupted operation on recovery.

---

# 5. Event envelope

Recommended conceptual fields:

```go
type Event struct {
    ID            EventID
    Sequence      uint64       // per stream/process or global ordering strategy
    AgentID       AgentID
    RootAgentID   AgentID
    Type          EventType
    Timestamp     time.Time
    CausationID   *EventID
    CorrelationID CorrelationID
    Actor         ActorRef
    SchemaVersion uint32
    Payload       []byte
}
```

Important semantics:

- event IDs immutable;
- payload schema versioned;
- causation identifies the event/command that directly caused this transition;
- correlation groups one user/task/request chain;
- ordering guarantees are explicitly documented rather than assumed from timestamps.

---

# 6. Ordering model

Do not depend on wall-clock timestamp ordering for correctness.

A simple local-first design can use:

- SQLite row/global sequence for ledger order;
- process-local monotonic sequence for each AgentID;
- causal links for cross-agent relationships.

Distributed execution later may require stronger stream-version/optimistic-concurrency semantics.

The public process model should not depend on one database-specific sequence implementation.

---

# 7. Optimistic concurrency

Concurrent commands targeting one process should not silently overwrite each other.

A process projection can carry a version:

```text
Agent A current version = 142
Command expects version = 142
```

Storage transition succeeds only if expected version still matches.

Otherwise:

```text
Conflict
→ reload/re-evaluate command
```

This avoids global locks while preserving deterministic state-machine transitions.

---

# 8. Reducers

A reducer is pure logical state transition code:

```go
func Apply(state State, event Event) (State, error)
```

Reducer rules:

- no I/O;
- no current time lookup;
- no random IDs;
- no provider/tool calls;
- validate impossible transitions;
- deterministic output;
- backward-compatible handling strategy for event versions.

Reducer tests should be exhaustive for lifecycle state machines.

---

# 9. Snapshots

Snapshots optimize recovery but are not the only source of history.

Snapshot envelope:

```text
snapshot_id
agent_id
through_sequence
state_schema_version
created_at
serialized projection
integrity hash
```

Recovery:

```text
latest compatible snapshot
      +
ledger events after sequence
      =
current logical state
```

Snapshot frequency is driven by replay cost, not arbitrary time alone.

---

# 10. Projection strategy

For frequently queried screens/state, storing projections alongside the event ledger is acceptable.

Examples:

```text
agents_current
pending_approvals
active_capabilities
conversation_index
budget_usage
```

Rules:

- projection updates happen atomically with canonical events when they participate in correctness;
- disposable/materialized projections can be rebuilt;
- schema clearly distinguishes canonical records from caches/indexes.

Do not force every query to replay events at runtime.

---

# 11. SQLite schema domains

Tentative table groups:

```text
ledger_events
agent_processes
agent_snapshots
intents
capability_leases
budget_accounts
resource_reservations
invocations
worlds
transactions
checkpoints
context_pages
beliefs
belief_edges
objects
conversation_blocks
wake_conditions
approvals
schema_migrations
```

This is a conceptual domain list, not a commitment to one table per noun.

Normalize around correctness and query patterns; avoid building a generic entity-attribute-value store.

---

# 12. Object store contract

Objects should preferably be immutable after finalization.

Conceptual API:

```go
type ObjectStore interface {
    Put(ctx context.Context, r io.Reader, meta ObjectMeta) (ObjectRef, error)
    Open(ctx context.Context, ref ObjectRef) (io.ReadCloser, ObjectMeta, error)
    Stat(ctx context.Context, ref ObjectRef) (ObjectMeta, error)
}
```

Important: APIs stream via `io.Reader`/`io.Writer`; they should not force full `[]byte` materialization.

Object references can be content-addressed to verify integrity/deduplicate.

---

# 13. Two-phase object finalization

Large content may be produced while an action is running.

Potential flow:

```text
create temporary object
stream bytes
compute hash/size
fsync/finalize as immutable object
persist canonical reference in SQLite transaction
```

If the runtime crashes before the DB reference is committed, the temporary/unreachable object can later be garbage-collected.

Avoid canonical events pointing to a blob that has not been durably finalized.

---

# 14. Artifact reachability and GC

Object-store GC requires a mark/reachability model.

Roots may include:

- ledger event references;
- snapshots;
- conversation blocks;
- context pages;
- beliefs/evidence;
- retained fork history;
- user-pinned artifacts.

GC:

```text
mark referenced objects
      ↓
apply retention grace period
      ↓
sweep unreachable finalized objects
```

Never use naive “delete files older than N days”.

---

# 15. Data integrity

Use integrity checks where cheap and useful:

- object hash verification;
- schema foreign keys where appropriate;
- event sequence uniqueness;
- version checks;
- transactional writes;
- startup integrity diagnostics;
- backup/export tests.

The application should distinguish:

```text
logical task failure
storage I/O failure
storage corruption/invariant violation
```

The latter may require fail-safe read-only mode rather than continuing mutations.

---

# 16. Database migrations

Migrations must be:

- versioned;
- transactional where SQLite permits;
- tested on realistic prior schemas;
- backed up or recoverable for destructive transformations;
- separate from runtime state reducers/event migrations.

Two version dimensions exist:

```text
storage schema version
event/payload semantic version
```

They should not be conflated.

---

# 17. Event evolution

Never rewrite old history casually because a Go struct changed.

Strategies:

- event payload version field;
- upcasters from old payload → current in-memory representation;
- snapshot migration;
- rare explicit offline history migration if unavoidable.

Prefer additive event evolution.

---

# 18. Conversation persistence

Conversation should use typed blocks indexed separately from raw runtime events.

Example:

```text
conversation_blocks
  block_id
  agent_id
  sequence
  type
  object_ref
  preview
  created_at
  invocation_id/tool_id optional
```

This gives the TUI efficient paginated queries without reading the full Event Ledger or model blobs.

---

# 19. Context page storage

Context-page metadata belongs in relational/indexed storage.

Content may live inline only when small.

```text
context_pages
  id
  type
  scope
  object_ref / inline content
  token_estimate
  importance
  confidence
  timestamps
  pin state
```

Dependencies/edges should be queryable without loading all page bodies.

---

# 20. Storage pressure

Long-lived agents can accumulate large histories.

The runtime needs explicit policies for:

- compression;
- duplicate artifacts;
- retention of losing speculative branches;
- old telemetry;
- provider raw payloads;
- raw logs after structured extraction;
- user-configured storage ceilings.

Canonical/audit data should not be discarded merely to stay under an invisible limit.

When a hard storage limit is reached, fail explicitly or request policy rather than deleting evidence silently.

---

# 21. Export and portability

A durable agent should eventually be exportable as a self-contained bundle:

```text
manifest
process/event streams
snapshots
objects
memory graph
configuration/policy metadata
```

This enables:

- backup;
- bug reports;
- reproducible replay;
- migration to another machine;
- future distributed workers.

Secrets must not be included by default.

---

# 22. Backup model

Local-first storage still needs backup semantics.

Potential online backup strategy:

- SQLite backup API or consistent checkpoint/copy procedure;
- immutable object store copied incrementally;
- manifest ties DB backup to object reachability point.

Backup/restore must become an integration test before claiming durable production behavior.

---

# 23. Storage failure behavior

If canonical persistence fails, the runtime must avoid continuing unrecorded mutations.

Example:

```text
SQLite disk full
    ↓
canonical commit fails
    ↓
runtime marks storage unhealthy
    ↓
stop accepting new mutating work
    ↓
allow safe inspection/export if possible
```

The runtime should not “keep working and catch up later” unless a formally durable alternative log exists.

---

# 24. Initial storage implementation

Pragmatic v0:

```text
SQLite WAL
+
filesystem content-addressed object directory
```

No Redis, Kafka, PostgreSQL, vector database or distributed object store is required for the local-first kernel.

Embeddings/indexes can be optional derived indexes later.

---

# Core invariant

> **If correctness requires a fact after a crash, that fact must have crossed a durable storage boundary before the runtime relies on it.**
