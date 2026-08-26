# Foundation Technical Decisions for G0

## Status

**A0 closure document — ACCEPTED baseline unless marked PROTOTYPE-VALIDATED.**

This document turns the remaining foundational questions into concrete implementation defaults so G0 does not become an architecture discovery phase.

---

# 1. Kernel language

## Decision

**Go remains the kernel/runtime language.**

Use Go for:

```text
runtime daemon
Agent Process supervisor
Event Ledger/state reducers
Cognitive MMU
Epistemic Memory
Authority/effect engine
scheduler/economy
local IPC
storage/object layer
TUI unless a later reason separates it
```

Python is an Execution World, not the kernel language.

Rust/native helpers remain optional future implementation details only if a measured low-level need appears.

---

# 2. SQLite driver

## Decision

**Start with `modernc.org/sqlite` behind an internal storage adapter.**

Status: **PROVISIONAL but implementation-ready.**

Why:

- CGo-free;
- straightforward Go cross-compilation;
- supports the single-binary/local-first product shape;
- mature SQLite semantics through a Go port;
- DB workload is metadata/event/projection oriented rather than analytical hot-loop throughput;
- storage adapter keeps driver replaceable if soak/benchmark evidence rejects it.

Do not couple domain code to driver-specific APIs unnecessarily.

## Revisit trigger

Before claiming G1 reliability, benchmark against at least one mature CGO SQLite binding on:

```text
append transaction latency
WAL checkpoint behavior
concurrent read + serialized write workload
backup/restore
1h/8h write soak
race detector compatibility
startup/recovery after forced kill
```

A significant correctness/stability issue can override the CGo-free preference.

---

# 3. Database access style

## Decision

**Use `database/sql` + explicit SQL. No ORM in kernel storage.**

Why:

- schemas/events are correctness-critical;
- explicit transaction boundaries matter;
- predictable queries;
- easier migration/replay diagnosis;
- avoids reflection/model magic in kernel core.

Small query helpers are allowed; a generic repository framework is not required.

---

# 4. SQLite baseline pragmas

Initial reliability-oriented profile:

```text
journal_mode = WAL
foreign_keys = ON
busy_timeout = bounded non-zero
synchronous = FULL initially
```

`FULL` is chosen as conservative baseline because event/control write rate is intentionally low—we do not persist per-token events.

After crash/power-loss benchmarks, `NORMAL` may be considered as a performance profile only if durability consequences are explicitly accepted.

Do not tune away durability before measuring a real bottleneck.

---

# 5. SQLite write ownership

## Decision

Use one logical canonical write path with short DB transactions.

This does **not** require one goroutine forever owning all DB access, but state-machine commands should serialize conflicting per-process writes through expected-version checks and avoid many long concurrent writers.

Rules:

```text
read transactions short
write transactions short
no network/model/tool call while DB transaction open
large blob streaming happens outside SQLite transaction
object finalization before canonical object reference commit
```

---

# 6. Event payload encoding

## Decision

**Versioned JSON payloads for v0 canonical events.**

Why:

- inspectable/debuggable;
- simple schema evolution/upcasters;
- performance is sufficient because event granularity is coarse;
- payloads are small; large artifacts are object references.

Envelope fields remain typed relational columns where useful; payload contains event-specific data.

Do not serialize internal Go structs blindly without schema-version discipline.

---

# 7. Snapshot format

## Decision

**Versioned JSON snapshot envelope + object-backed payload when large.**

Snapshot is an optimization, not canonical history.

```yaml
snapshot_version: 1
state_schema_version: 1
agent_id: ...
through_process_version: ...
through_ledger_sequence: ...
payload_ref_or_inline: ...
integrity_hash: ...
```

If snapshot decoding/migration fails, runtime may fall back to an earlier compatible snapshot + ledger replay.

This makes snapshot format low-risk to evolve.

Compression is an object-store optimization, not snapshot semantic format.

---

# 8. Content addressing

## Decision

Use **SHA-256** content hashes for immutable Object Store identity/integrity in v0.

Why:

- standard-library implementation;
- interoperable/stable;
- collision resistance far beyond product needs;
- avoids introducing a hash dependency for marginal speed where I/O dominates.

ObjectRef identity can include algorithm prefix for future evolution:

```text
sha256:<hex>
```

---

# 9. Object finalization

## Decision

```text
stream temporary object
→ hash/size while streaming
→ flush/fsync according to durability policy
→ atomically rename/finalize immutable object
→ commit metadata/reference in SQLite
```

Canonical DB state never points at an unfinished blob.

Crash-created unreachable temp/finalized objects are cleaned by reachability/age-safe GC.

---

# 10. IDs

## Decision

IDs are opaque typed strings/bytes at domain boundaries; ordering never depends on ID lexical order.

Use a time-sortable random UUID-style ID implementation if convenient, but:

```text
Event ordering = explicit sequence/version
not ID ordering
not timestamp ordering
```

This prevents ID choice from leaking into correctness semantics.

---

# 11. Time

## Decision

Wall-clock timestamps are observational metadata, not concurrency ordering.

Kernel uses injected `Clock` interface in stateful/testable components.

Use monotonic durations from Go runtime for elapsed deadlines within one process execution; persist absolute wake/deadline timestamps + generation/version for recovery.

---

# 12. Logging

## Decision

Use Go `log/slog` structured logging for runtime diagnostics.

Logs are not canonical Event Ledger history.

Required correlation fields where relevant:

```text
agent_id
root_agent_id
operation_id
invocation_id
correlation_id
world_id
transaction_id
```

Secret/object-content leakage policy applies to logs.

---

# 13. Local IPC transport

## Decision

Protocol semantics are fixed by `LOCAL_CONTROL_PROTOCOL.md`.

Transport baseline:

```text
macOS/Linux → Unix domain socket
Windows     → named pipe
```

Loopback TCP exists only as explicit fallback/development adapter.

Runtime does not expose remote network listening by default.

---

# 14. IPC framing/encoding

## Decision

**32-bit/varint-style bounded length prefix + versioned JSON envelope v0.**

Exact framing integer representation can be finalized in code with tests, but properties are fixed:

```text
explicit message length
hard max frame size
JSON semantic payload
large artifacts out-of-band/range streamed
request IDs/idempotency where specified
```

Do not use newline-delimited JSON as protocol correctness boundary.

---

# 15. Configuration

## Decision

Keep runtime configuration layered and typed:

```text
compiled safe defaults
→ config file
→ environment variables for deployment/runtime secrets refs only where appropriate
→ CLI overrides
```

Do not let arbitrary model/harness output mutate global runtime configuration.

Configuration versions/hashes that affect reproducibility should be recorded in run metadata where relevant.

---

# 16. Migrations

## Decision

Embed ordered SQL migrations into binary.

Migration requirements:

```text
explicit version table
forward migrations transactional where possible
migration integration tests from historical fixtures
backup/recovery requirement before destructive transform
no auto-generated ORM migrations
```

Event semantic upcasting is separate from DB schema migration.

---

# 17. FTS/retrieval baseline

## Decision

Use SQLite FTS as the local-first lexical retrieval baseline for Context Pages/Epistemic Memory where supported by chosen driver build.

Embedding/vector retrieval remains optional derived indexing and must not be needed for correctness.

If driver packaging complicates FTS support, a small internal lexical index adapter can be substituted without changing MMU semantics.

---

# 18. Compression

## Decision

Do not compress every small event/row.

Object Store MAY compress large text/log/model artifacts based on media type and threshold.

Compression metadata belongs to ObjectMeta and Open() returns decompressed stream semantics.

No component should need to know whether an object is physically compressed.

---

# 19. Encryption at rest

## Decision

Not required for G0/G1 kernel correctness, but storage interfaces must not prevent it later.

Secrets themselves should live in OS/provider secret facilities rather than plain SQLite/Object Store where possible.

User/project artifact encryption-at-rest is a later product/security feature, not silently claimed by v0.

---

# 20. Dependency discipline

Kernel preference:

```text
stdlib first
small focused dependencies
adapters behind internal interfaces
no framework owns process lifecycle/state model
```

Every dependency in a hot/durable boundary is evaluated for:

```text
maintenance
cross-platform behavior
allocation/backpressure behavior
cancellation support
API stability
```

---

# 21. TUI library

## Decision

**Still DEFERRED until a benchmark fixture exists.**

The architecture is library-independent.

A candidate TUI library must demonstrate:

```text
100k-block paginated session
bounded viewport model
incremental/coalesced streaming updates
no full-history rerender
stable memory after 1h synthetic stream
```

This is intentionally not needed for G0/G1 daemon correctness.

---

# 22. Foundation validation gates

Before leaving G0/G1:

```text
FND-001 kill -9 during event writes never yields impossible process projection
FND-002 DB reopens/replays deterministically
FND-003 object finalized before reference commit
FND-004 object temp crash garbage is recoverable/collectable
FND-005 canonical write path never holds DB tx across external I/O
FND-006 UDS/named-pipe malformed frames cannot crash daemon
FND-007 driver write/read soak shows bounded connection/heap behavior
FND-008 snapshot can be discarded and state reconstructed from ledger
FND-009 cross-platform build matrix succeeds for supported targets
FND-010 SQLite driver swap remains contained to storage adapter in test prototype
```

---

# Accepted A0 foundation decisions

1. Go kernel/runtime.
2. `modernc.org/sqlite` as provisional CGo-free v0 driver behind adapter.
3. `database/sql` + explicit SQL, no ORM.
4. WAL + foreign keys + reliability-first synchronous mode.
5. Coarse versioned JSON events.
6. Versioned JSON snapshots, rebuildable from ledger.
7. SHA-256 content-addressed Object Store.
8. Explicit stream-finalize-then-reference object protocol.
9. Ordering uses sequence/version, never UUID/timestamp.
10. Unix socket/macOS/Linux and named pipe/Windows local IPC.
11. Length-framed versioned JSON messages.
12. SQLite FTS lexical retrieval baseline; embeddings optional.
13. TUI library remains benchmark-selected and is not a kernel dependency.

> **G0 may choose concrete packages, but it may not invent new semantics.**
