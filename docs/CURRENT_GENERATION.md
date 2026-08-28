# Current Implementation Generation

**Updated: August 28, 2026**

```text
A0  COMPLETE
G0  COMPLETE
G1  COMPLETE
G2  COMPLETE
G3  COMPLETE
G4  COMPLETE
G5  COMPLETE
G6  READY
```

## Current generation

**G5 — Recursive Agent Processes: COMPLETE.**

G5 implements durable recursive Agent Processes with delegated Task Intent, authority subsets, hierarchical budget reservation/settlement, bounded durable mailboxes, restart-safe waits, wait-cycle detection, cancellation propagation, fan-out/depth/parallelism controls, fair admission, explicit completed-work reuse and structured evidence-oriented result contracts.

The G5 killer scenario proves three delegated repository investigators survive durable-store reopen while the parent is waiting and return bounded structured results/evidence without importing whole child transcripts.

Validation run `33214615854` passed:

```text
test (ubuntu-latest)  ✅
test (macos-latest)   ✅
test (windows-latest) ✅
race                   ✅
```

See `G5_EXIT_REVIEW.md`.

## Next generation

**G6 — Cognitive Scheduler v0: READY.**

G6 can now build model/resource scheduling on top of durable processes, bounded context, controlled Worlds and recursive orchestration without changing G5 semantics.
