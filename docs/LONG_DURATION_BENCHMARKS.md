# Long-Duration Benchmarks

## Status

**Executable baseline added September 7, 2026.**

The project already defined 1h/8h/24h soak goals in `TESTING_BENCHMARKS_AND_QUALITY_GATES.md`. This document turns the first important G0/G4 validation debt into repeatable commands and explicit pass/fail signals.

The initial executable suite focuses on two foundations that every later generation depends on:

1. **Cognitive MMU boundedness under a large persisted corpus**;
2. **SQLite durability under repeated reads/writes, WAL checkpoints and close/reopen cycles**.

These scenarios are intentionally provider-free and deterministic. They validate runtime architecture rather than model quality.

---

## Scenarios

### MMU corpus boundedness

Test:

```text
internal/mmu/TestSoakMMUCorpusBounded
```

The scenario:

- uses the real SQLite context-page repository and FTS index;
- uses the real content-addressed Object Store;
- seeds a configurable persisted metadata corpus;
- repeatedly builds bounded working sets against that corpus;
- verifies every build remains inside its hard context budget;
- records bounded-window p95/max build latency;
- records Go heap, heap objects, GC count, goroutines and DB connection stats;
- forces a final GC and rejects excessive retained heap/goroutine growth.

The default smoke corpus is 10,000 pages. Long profiles use 100,000 pages.

The corpus deliberately reuses one Object Store body across many metadata entries so the benchmark measures MMU metadata/retrieval behavior without creating 100,000 physical blobs.

### SQLite durability

Test:

```text
internal/storage/sqlite/TestSoakSQLiteDurability
```

The scenario:

- uses the production SQLite adapter and migration path;
- performs repeated immutable-object metadata writes and reads;
- periodically runs WAL `TRUNCATE` checkpoints;
- periodically closes and reopens the database;
- verifies data remains readable after reopen;
- verifies DB connections are not left in-use between operations;
- records bounded-window p95/max operation latency;
- records Go heap/goroutine growth and connection-pool statistics.

The object cardinality is capped so a 24-hour test can distinguish runtime leaks from expected durable database growth.

---

## Commands

### Compile-only gate

The normal CI compiles the tagged soak scenarios without running a long test:

```bash
make soak-compile
```

Equivalent command:

```bash
go test -tags=soak -run '^$' ./internal/mmu ./internal/storage/sqlite
```

This prevents the long-duration suite from silently rotting while keeping ordinary CI fast.

### 30-second smoke

```bash
make soak-smoke
```

Defaults:

```text
SOAK_DURATION=30s
SOAK_INTERVAL=250ms
SOAK_SAMPLE_INTERVAL=5s
SOAK_MMU_CORPUS_PAGES=10000
```

### One hour

```bash
make soak-1h
```

This is the first meaningful stability gate before starting to make strong long-session claims.

### Eight hours

```bash
make soak-8h
```

Use a dedicated machine or self-hosted runner. Do not rely on a standard GitHub-hosted job for an eight-hour execution window.

### Twenty-four hours

```bash
make soak-24h
```

This is intended for local/reference hardware or a self-hosted runner.

---

## Configuration

The suite is controlled through environment variables.

| Variable | Default | Meaning |
|---|---:|---|
| `SOAK_DURATION` | `30s` | duration of each tagged scenario |
| `SOAK_INTERVAL` | `250ms` | delay between operations |
| `SOAK_SAMPLE_INTERVAL` | `5s` | runtime metrics log interval |
| `SOAK_MAX_HEAP_GROWTH_MB` | `64` | maximum retained Go heap growth after final GC |
| `SOAK_MAX_GOROUTINE_GROWTH` | `16` | maximum retained goroutine growth |
| `SOAK_LATENCY_SAMPLES` | `4096` | bounded latency sample window |
| `SOAK_MMU_CORPUS_PAGES` | `10000` | persisted MMU metadata corpus size |
| `SOAK_SQLITE_MAX_OBJECTS` | `100000` | maximum durable object metadata cardinality |
| `SOAK_SQLITE_REOPEN_EVERY` | `1000` | operation interval between DB reopen cycles |
| `SOAK_SQLITE_CHECKPOINT_EVERY` | `100` | operation interval between WAL checkpoints |

Example custom run:

```bash
SOAK_DURATION=2h \
SOAK_INTERVAL=500ms \
SOAK_MMU_CORPUS_PAGES=100000 \
SOAK_MAX_HEAP_GROWTH_MB=96 \
go test -v -tags=soak -run '^TestSoak' -count=1 -timeout=150m \
  ./internal/mmu ./internal/storage/sqlite
```

---

## Pass criteria

The initial executable baseline enforces qualitative boundedness rather than machine-specific nanosecond thresholds.

A run passes only if:

```text
MMU working-set hard token budget is never exceeded
SQLite data remains readable across repeated reopen cycles
SQLite has no connection left in-use between sampled operations
retained Go heap growth stays inside configured bound after GC
retained goroutine growth stays inside configured bound
no scenario operation fails
```

Latency is recorded but is not yet a hard threshold. We first need repeatable measurements on reference hardware before setting machine-sensitive p95 gates.

---

## What this does not prove yet

This first suite deliberately does **not** claim that all long-session validation debt is closed.

Still required later:

- real daemon 1h/8h/24h end-to-end workload;
- multi-GB command-output/RSS test;
- terminal attach/detach and slow-client soak once the final TUI exists;
- recursive-agent high-cardinality lifecycle soak;
- RSS/process-resident-memory sampling on reference OS/hardware;
- stable latency baselines on reference hardware;
- randomized daemon kill/restart campaign;
- scheduler/provider outage soak after G6.

The important change is that long-duration validation is now an executable track rather than only a design note.

---

## G4 validation debt

`G4_EXIT_REVIEW.md` originally deferred the final CI/race pass and longer empirical MMU corpus/heap/latency calibration.

The later G5/main validation run closed the compile/test/vet/race portion for the integrated G4 code. The new soak suite now provides the repeatable mechanism for the remaining empirical MMU and SQLite longevity work.

Until representative 1h/8h/24h runs are recorded, the correct status is:

```text
long-duration harness: IMPLEMENTED
long-duration reference runs: PENDING
long-duration architecture claim: PARTIALLY VALIDATED
```
