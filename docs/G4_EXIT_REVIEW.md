# G4 Exit Review — Cognitive MMU v0

## Status

**G4 — Cognitive MMU v0: COMPLETE.**

**G5 — Recursive Agent Processes: READY.**

G4 is closed on August 28, 2026 after merging PR #1 (`feat(g4): implement Cognitive MMU v0`) into `main` as commit `25c5e6c0dfa5bc97166532d29d811d1d22ffad2c`.

Per project-owner decision, the final multi-platform/race validation pass is **deferred** and will be run later. This does not block advancing the implementation roadmap to G5; any validation defect discovered later must be fixed without silently changing the accepted G4 semantics.

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

The final full CI/race execution of the latest G4 commit set is intentionally deferred by the project owner.

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

## Deferred validation

The following validation is deliberately postponed rather than represented as completed:

- latest `go test ./...` on all CI operating systems;
- latest `go vet ./...` on all CI operating systems;
- latest binary builds on all CI operating systems;
- latest `go test -race ./...`;
- longer empirical MMU corpus/heap/latency calibration.

These are validation debt, not unfinished G4 feature scope.

---

# G5 readiness

G5 can now build on a durable bounded cognitive substrate.

The next generation is:

## G5 — Recursive Agent Processes

Primary implementation targets:

- durable `spawn()` validate/reserve/create protocol;
- child Task Intent and authority subset;
- structured result/evidence contract;
- bounded messaging/mailboxes;
- durable parent waits;
- cancellation tree;
- wait-for cycle detection;
- fan-out/depth/fairness controls;
- budget reservations and settlement.

Killer demonstration:

> A root Agent Process delegates three repository investigations in parallel, survives daemon restart while a child is pending, and receives bounded structured evidence without importing whole child transcripts.

**G5 is READY.**
