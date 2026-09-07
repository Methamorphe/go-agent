# G4 Exit Review — Cognitive MMU v0

## Status

**G4 — Cognitive MMU v0: COMPLETE.**

**G5 — Recursive Agent Processes: COMPLETE. G6 — Cognitive Scheduler v0: READY.**

G4 was closed on August 28, 2026 after merging PR #1 (`feat(g4): implement Cognitive MMU v0`) into `main` as commit `25c5e6c0dfa5bc97166532d29d811d1d22ffad2c`.

At the exact G4 closure point, the project owner explicitly deferred the final multi-platform/race validation pass. That was historical validation debt, not unfinished feature scope.

A later integrated `main` validation after G5 closed that compile/test/vet/race portion of the debt: head `9bf595ad40f0b74458deab52da67d06fd6984f0d` passed GitHub Actions run `33214992611` on Ubuntu, macOS, Windows and the race detector while containing the complete G4 implementation.

The remaining G4 debt is empirical long-duration MMU corpus/heap/latency calibration. An executable soak harness is now documented in `LONG_DURATION_BENCHMARKS.md`; representative 1h/8h/24h reference runs are still pending.

---

## Delivered

- semantic durable Context Pages with stable `ctx_*` identity;
- persisted page metadata in SQLite and page bodies in the content-addressed Object Store;
- bounded lexical retrieval using SQLite FTS5 without materializing the full corpus;
- conservative token estimation with bounded estimate cache and injectable estimator support;
- hard model input budgeting with explicit `ContextBudgetImpossible` failure semantics;
- deterministic tiered working-set construction;
- Tier 0 mandatory/pinned context;
- Tier 1 active state;
- Tier 2 explicit recall leases;
- Tier 3 deterministic relevant-page ranking;
- Tier 4 recent continuity fallback;
- explicit `recall()` syscall integrated into the agent loop and durable syscall trace;
- deterministic ranking components and Context Manifest explanations;
- scope filtering before ranking;
- supersession and compaction-aware selection;
- structured compaction retaining source-page references;
- bounded one-build recall leases;
- recall-loop / anti-thrashing protection;
- source diversity limiting;
- lazy body materialization with hard byte limits;
- integration of the Cognitive MMU into the daemon's agent runner so raw conversation history is no longer treated as canonical long-session memory.

---

## G4 contract coverage

The implementation includes the G4/MMU contract tests for:

```text
MMU-001 hard budget never exceeded
MMU-002 mandatory overflow fails explicitly
MMU-003 invalidated/superseded page not selected as normal trusted page
MMU-004 explicit visible PageID outranks generic search result
MMU-005 inaccessible scope page never returned
MMU-006 100k-page corpus does not materialize all bodies
MMU-007 token cache bounded
MMU-008 repeated recall loop detected
MMU-009 summary retains source references
MMU-010 manifest deterministically explains v0 selection for fixed inputs
```

---

## Preserved architecture invariants

1. The LLM context is a bounded working set, not canonical memory.
2. Retrieval scope is enforced before ranking.
3. Explicit references dominate generic retrieval when visible and valid.
4. Superseded/compacted raw pages are not silently treated as normal current context.
5. Page bodies remain object-backed and are loaded lazily.
6. Historical corpus growth does not imply equivalent hot-heap growth.
7. Context packing fails explicitly when mandatory content cannot fit.
8. Context selection remains deterministic and explainable in v0.
9. Compaction preserves source references instead of destructively replacing evidence.
10. Recall leases are bounded rather than permanent pins.
11. Recall loops are a first-class bounded failure mode.
12. Provider-specific inference suspension is not required for correctness.

---

## Validation status

### Resolved after the original G4 exit

The integrated G5/main validation run `33214992611` passed:

```text
go test ./...                  ✅ Ubuntu/macOS/Windows
go vet ./...                   ✅ Ubuntu/macOS/Windows
go build ./cmd/go-agent
         ./cmd/go-agentctl     ✅ Ubuntu/macOS/Windows
go test -race ./...            ✅ Ubuntu
```

Because that head contains the complete G4 code, the historical deferred compile/test/vet/race pass is no longer an open debt item.

### Remaining empirical debt

The following still require representative long-duration/reference-hardware execution:

- MMU corpus/heap/latency calibration over 1h/8h/24h profiles;
- process RSS sampling in addition to Go heap metrics;
- longer SQLite/MMU mixed-workload stability;
- machine-specific p95 latency baselines.

The tagged soak scenarios now provide a repeatable mechanism for this work. Until those reference runs are recorded, long-duration boundedness is partially validated rather than fully closed empirically.

---

# G5/G6 readiness

G5 has since completed successfully on top of G4, including recursive durable orchestration and a full cross-platform/race pass.

The current next generation is:

## G6 — Cognitive Scheduler v0

Primary implementation targets:

- model registry/Profile;
- Cognitive Task descriptor;
- hard eligibility filtering;
- deterministic cost/latency/quality policy;
- provider health/circuit breaker;
- hierarchical budget reservation/settlement;
- fallback and privacy/locality constraints;
- fairness/global/per-root slots;
- routing decision events/metrics.

**G4 remains COMPLETE. G6 is READY.**
