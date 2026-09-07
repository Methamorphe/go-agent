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
G6  COMPLETE
G7  READY
```

## Current generation

**G6 — Cognitive Scheduler v0: COMPLETE.**

G6 implements deterministic per-invocation model scheduling over bounded Cognitive MMU context with model Profiles, hard eligibility constraints, cost/latency/quality/load scoring, provider health/circuit breaking, fallback, privacy/locality policy, bounded global/per-root/provider concurrency, durable routing events/metrics and exact resource reservation/settlement.

The daemon is config-driven and opt-in. Legacy G4 behavior remains unchanged when the scheduler is disabled.

Scheduler root-budget state is durable in SQLite via migration `0006_cognitive_scheduler.sql`: limits, spent usage and active reservations survive close/reopen, preventing budget reset or overspend after daemon restart.

GitHub Actions run `34160212580` on head `ba26110afe7cadb96832d8ab2df357e3bcd6d949` passed:

```text
test (ubuntu-latest)  ✅
test (macos-latest)   ✅
test (windows-latest) ✅
race                   ✅
```

The platform jobs passed `go test ./...`, `go vet ./...`, soak-scenario compilation where applicable and both binary builds. The race job passed `go test -race ./...`.

See:

- `G6_EXIT_REVIEW.md`;
- `COGNITIVE_SCHEDULER_V0.md`;
- `LONG_DURATION_BENCHMARKS.md`.

## Long-duration validation

```text
soak harness                IMPLEMENTED
soak compile gate           ENABLED IN CI
1h reference run            PENDING
8h reference run            PENDING
24h reference run           PENDING
```

The executable baseline covers real SQLite reopen/checkpoint durability, Cognitive MMU boundedness against persisted large corpora, and scheduler budget restart safety. Reference 1h/8h/24h runs remain empirical longevity work and are not a known G6 correctness defect.

## Next generation

**G7 — Workspace/OCI World + Agent Transactions: READY.**

G7 can now build isolated reversible/speculative execution and transaction semantics on top of durable processes, authority/effects, bounded context, recursive orchestration and model/resource scheduling.
