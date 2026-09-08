# Current Implementation Generation

**Updated: September 8, 2026**

```text
A0  COMPLETE
G0  COMPLETE
G1  COMPLETE
G2  COMPLETE
G3  COMPLETE
G4  COMPLETE
G5  COMPLETE
G6  COMPLETE
G7  COMPLETE
G8  COMPLETE
G9  COMPLETE
G10 COMPLETE
G11 READY
```

## Current generation

**G10 — Context Faults + Cognitive MMU v2: COMPLETE.**

G10 turns missing cognitive state into explicit, typed, bounded runtime paging events while preserving G4/G6/G9 compatibility and provider independence.

The implementation now includes:

- stable semantic references: `ctx://`, `belief://`, `evidence://`, `object://`, `event://`, `checkpoint://`, `agent://`;
- all six typed faults: Reference, Recall, Evidence, Freshness, Dependency and Representation;
- finite one-build context leases for faulted pages;
- bounded dependency-driven paging with resolver-independent cognitive references;
- freshness paging through durable supersession chains with cycle detection;
- structured compaction ↔ raw evidence paging through `SummaryOf`;
- historical evidence inspection without silently promoting superseded knowledge back to current truth;
- explicit `NEEDS_COMPACTION_OR_PROJECTION` handling for representations that exceed fault budgets;
- per-invocation and per-task-window fault budgets;
- progress-aware repeated-fault/page-set storm protection;
- append-only lifecycle observability from `DETECTED` through terminal resolution state;
- durable SQLite current-state journal and lifecycle event history through migration `0011`;
- crash-safe resolved-fault → next-manifest handoff;
- G4 manifest integration so resolved faults remain visible/replayable at the next invocation boundary;
- semantic `belief://` references emitted by G9 MMU projections;
- G10 contract tests plus durable manifest/lifecycle tests;
- repaired tagged soak compilation using the dedicated SQLite `CheckpointWAL` maintenance API.

The G10 killer condition is covered by the implementation contract: an invocation can fault on missing/stale/dependent knowledge, the runtime resolves only authorized bounded pages, grants finite leases, records the complete lifecycle, and makes the resolution explicit in the next durable context manifest rather than silently mutating model context.

See:

- `G10_EXIT_REVIEW.md`;
- `CONTEXT_FAULTS_AND_COGNITIVE_PAGING.md`;
- `G9_EXIT_REVIEW.md`.

## Validation note

G10 uses the repository CI matrix as its closure gate: standard tests, race tests, tagged soak compilation, `go vet` and command builds must remain green on the final implementation head before merge.

## Long-duration / scale validation

```text
G10 contract tests             IMPLEMENTED
durable lifecycle tests       IMPLEMENTED
tagged soak compilation       IMPLEMENTED
fault storm bounds            IMPLEMENTED
1h reference run              PENDING
8h reference run              PENDING
24h reference run             PENDING
```

Long-duration empirical calibration remains separate from G10 semantic completion.

## Next generation

**G11 — Adaptive Teams + Agent Negotiation: READY.**

G11 can now build bounded multi-agent negotiation on top of durable processes, forks/world transactions, epistemic memory, typed context paging and the Cognitive Scheduler.
