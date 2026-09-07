# Current Implementation Generation

**Updated: September 7, 2026**

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

The integrated `main` head `9bf595ad40f0b74458deab52da67d06fd6984f0d` passed GitHub Actions run `33214992611`:

```text
test (ubuntu-latest)  ✅
test (macos-latest)   ✅
test (windows-latest) ✅
race                   ✅
```

This integrated pass also validates the compiled/tested/race state of G4 as included in G5. The remaining G4/G0 longevity debt is empirical rather than a known compile/race defect.

See:

- `G5_EXIT_REVIEW.md`;
- `G4_EXIT_REVIEW.md`;
- `LONG_DURATION_BENCHMARKS.md`.

## Long-duration validation

```text
soak harness                IMPLEMENTED
soak compile gate           ENABLED IN CI
1h reference run            PENDING
8h reference run            PENDING
24h reference run           PENDING
```

The executable baseline currently covers real SQLite reopen/checkpoint durability and Cognitive MMU boundedness against a persisted large corpus. Reference long-duration results must be recorded before claiming full 1h/8h/24h validation.

## Next generation

**G6 — Cognitive Scheduler v0: READY.**

G6 can now build model/resource scheduling on top of durable processes, bounded context, controlled Worlds and recursive orchestration without changing G5 semantics.
